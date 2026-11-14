/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

// upgradeImageTag is the image tag the command writes.
var upgradeImageTag string

// upgradeImageNamespace is the image repo to record when the flag is set.
var upgradeImageNamespace string

// upgradeControlPlaneCmd is the parent of the image and blue-green commands.
var upgradeControlPlaneCmd = &cobra.Command{
	Use:   "control-plane",
	Short: "Upgrade a control plane",
	Run: func(cmd *cobra.Command, args []string) {
		missingErr("upgrade control-plane")
		os.Exit(1)
	},
}

// UpgradeControlPlaneCmd updates the image on a control plane.
var UpgradeControlPlaneCmd = &cobra.Command{
	Use:     "image",
	Example: "tptctl upgrade control-plane image",
	Short:   "Upgrades the version of the Threeport control plane",
	Long: `Update the image tag on existing control plane Deployments.

The one other thing it changes is what the database migrator needs to back
the database up before migrating: the backup location, on a control plane
with scheduled backups, and the mounts for it. Otherwise it does not
recreate manifests, RBAC, or configmaps. Use tptctl reinstall for those.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, config, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		encyptionKey, err := config.GetThreeportEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to retrieve encryption key for control plane:", err)
			os.Exit(1)
		}

		// get the kubernetes runtime instance object
		controlPlaneInstance, err := client.GetSelfControlPlaneInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve self control plane instance from threeport API:", err)
			os.Exit(1)
		}

		// reject a child connection, then select the named descendant
		controlPlaneInstance, err = upgradeRollTarget(
			controlPlaneInstance,
			cliArgs.ControlPlaneName,
			func(name string) (*api_v0.ControlPlaneInstance, error) {
				return client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, name)
			},
			func(id uint) (*api_v0.ControlPlaneInstance, error) {
				return client.GetControlPlaneInstanceByID(apiClient, apiEndpoint, id)
			},
		)
		if err != nil {
			cli.Error(err.Error(), nil)
			os.Exit(1)
		}

		// record the child and let its controller roll the deployments
		if cliArgs.ControlPlaneName != "" {
			if err := recordControlPlaneImage(apiClient, apiEndpoint, controlPlaneInstance.ID); err != nil {
				cli.Error("failed to record control plane image", err)
				os.Exit(1)
			}
			cli.Complete(fmt.Sprintf("recorded image %s on %s", upgradeImageTag, cliArgs.ControlPlaneName))
			return
		}

		// the cluster the connected control plane runs on
		var kubernetesRuntimeInstance *api_v0.KubernetesRuntimeInstance
		if controlPlaneInstance.KubernetesRuntimeInstanceID != nil {
			kubernetesRuntimeInstance, err = client.GetKubernetesRuntimeInstanceByID(
				apiClient,
				apiEndpoint,
				*controlPlaneInstance.KubernetesRuntimeInstanceID,
			)
		} else {
			kubernetesRuntimeInstance, err = client.GetThreeportControlPlaneKubernetesRuntimeInstance(
				apiClient,
				apiEndpoint,
			)
		}
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instance from threeport API:", err)
			os.Exit(1)
		}

		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			apiEndpoint,
			encyptionKey,
		)
		if err != nil {
			cli.Error("failed to get kube client:", err)
			os.Exit(1)
		}

		// The upgrade moves every default component to one tag. Say so before
		// it happens when a component was pinned to another. The list endpoint
		// leaves the recorded components out, so the instance is fetched by ID.
		// The notice is advisory, so a failure to read the record is reported
		// and the upgrade goes on.
		recordedInstance, err := client.GetControlPlaneInstanceByID(
			apiClient,
			apiEndpoint,
			*controlPlaneInstance.ID,
		)
		if err != nil {
			cli.Warning(fmt.Sprintf(
				"could not read the recorded components to check for pinned images: %s", err,
			))
		} else if warning := pinnedComponentsWarning(recordedInstance, upgradeImageTag); warning != "" {
			cli.Warning(warning)
		}

		// the bucket scheduled backups go to, which the database migrator backs
		// up to before migrating. A child uses its own config entry when one exists.
		backupName := requestedControlPlane
		if controlPlaneInstance.Name != nil && *controlPlaneInstance.Name != "" {
			backupName = *controlPlaneInstance.Name
		}
		controlPlaneConfig, err := config.GetControlPlaneConfig(backupName)
		if err != nil && backupName != requestedControlPlane {
			controlPlaneConfig = &cli.ControlPlane{}
			err = nil
		}
		if err != nil {
			cli.Error("failed to get control plane config:", err)
			os.Exit(1)
		}

		controllers, err := controllersForRecordedApis(recordedInstance)
		if err != nil {
			cli.Error("failed to select controllers from the recorded apis list", err)
			os.Exit(1)
		}
		// roll the connected deployments; the repo is recorded with the tag
		err = installer.UpdateDeploymentImageTag(
			upgradeImageTag,
			upgradeImageNamespace,
			*controlPlaneInstance.Namespace,
			cli.ScheduledBackupBucket(controlPlaneConfig),
			dynamicKubeClient,
			mapper,
			controllers,
			true,
		)
		if err != nil {
			cli.Error("failed to update threeport deployment image tag", err)
			os.Exit(1)
		}

		// Children created from here on default to the images this control plane
		// runs, so record the tag the deployments were just moved to.
		//
		// This is the instance-wide tag, not a rewrite of the recorded
		// components: the API cannot change a component once it is recorded, an
		// update to an instance leaves them as they are. The instance-wide tag
		// is applied over the recorded components when a child is created, so
		// the child gets the tag the deployments are now on, which is what the
		// recorded tags of the components named in the warning above no longer
		// describe. What a component records beyond its tag, its image name and
		// registry, is untouched by the upgrade and is still inherited.
		// retry the tag write until the replaced API answers
		if err := util.Retry(5, 2, func() error {
			return recordControlPlaneImage(apiClient, apiEndpoint, controlPlaneInstance.ID)
		}); err != nil {
			cli.Error("deployments were updated but the new image tag could not be recorded for child control planes:", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Succesfully updates all threeport deployments to image: %s", upgradeImageTag))
	},
}

// recordControlPlaneImage writes the image tag, and the repo when the flag is set.
// UpdateDeploymentImageTag takes only a tag, so the repo is stored on the instance.
func recordControlPlaneImage(apiClient *http.Client, apiEndpoint string, id *uint) error {
	// write the tag, and the repo when the flag is set
	instance := &api_v0.ControlPlaneInstance{
		Common:   api_v0.Common{ID: id},
		ImageTag: &upgradeImageTag,
	}
	if upgradeImageNamespace != "" {
		instance.ImageNamespace = &upgradeImageNamespace
	}
	_, err := client.UpdateControlPlaneInstance(apiClient, apiEndpoint, instance)
	return err
}

// pinnedComponentsWarning says which components of a control plane were pinned
// to an image tag other than the rest api's, and that the upgrade is about to
// move them to imageTag with everything else. It returns "" when nothing was
// pinned.
//
// A control plane that records an instance-wide tag has been upgraded before.
// That upgrade moved every default deployment to the tag, so the components
// all run it and nothing is pinned any more, whatever tags the recorded
// components still carry: the API cannot update them, so they keep the tag
// from before the upgrade. Reading them again would warn about a pin that
// the earlier upgrade already replaced.
// controllersForRecordedApis returns the controllers named by the recorded apis list.
// A missing record uses the full default set.
func controllersForRecordedApis(instance *api_v0.ControlPlaneInstance) ([]*api_v0.ControlPlaneComponent, error) {
	// an unread record keeps the full default set
	if instance == nil {
		return nil, nil
	}
	return installer.SelectControllersByGroup(
		installer.ParseApis(util.DerefString(instance.Apis)),
		installer.ThreeportControllerList,
	)
}

func pinnedComponentsWarning(instance *api_v0.ControlPlaneInstance, imageTag string) string {
	if instance == nil || util.DerefString(instance.ImageTag) != "" {
		return ""
	}
	pinned := installer.DivergentComponents(instance.CustomComponentInfo)
	if len(pinned) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"%s run a different image tag than the rest of the control plane. "+
			"The upgrade moves every default component to %s, so those are replaced too, "+
			"and control planes created from here on get %s for them. "+
			"Rebuild or re-pin them after the upgrade if they must stay on their own tag.",
		strings.Join(pinned, ", "), imageTag, imageTag,
	)
}

// init registers the control plane image upgrade command.
func init() {
	// register the command and its flags
	upgradeControlPlaneCmd.AddCommand(UpgradeControlPlaneCmd)
	upgradeControlPlaneCmd.AddCommand(UpgradeBlueGreenCmd)
	UpgradeCmd.AddCommand(upgradeControlPlaneCmd)
	upgradeControlPlaneCmd.PersistentFlags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)

	UpgradeControlPlaneCmd.Flags().StringVar(
		&upgradeImageNamespace,
		"image-namespace",
		"",
		"Image namespace to record. Omit to leave the recorded repo unchanged.",
	)
	UpgradeControlPlaneCmd.Flags().StringVar(
		&upgradeImageTag,
		"image-tag",
		"",
		"Image tag to roll the control plane to.",
	)
	UpgradeControlPlaneCmd.MarkFlagRequired("image-tag")
}
