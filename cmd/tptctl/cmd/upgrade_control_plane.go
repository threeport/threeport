/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
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

var updateImageTag string

// UpCmd represents the create threeport command
var UpgradeControlPlaneCmd = &cobra.Command{
	Use:     "control-plane",
	Example: "tptctl upgrade control-plane --version=v0.5.0",
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
		kubernetesRuntimeInstance, err := client.GetThreeportControlPlaneKubernetesRuntimeInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instance from threeport API:", err)
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

		// reject a child before any deployment changes
		if err := api_v0.ChildControlPlaneRequest(controlPlaneInstance); err != nil {
			cli.Error(err.Error(), nil)
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
		} else if warning := pinnedComponentsWarning(recordedInstance, updateImageTag); warning != "" {
			cli.Warning(warning)
		}

		// the bucket scheduled backups go to, which the database migrator backs
		// up to before migrating
		controlPlaneConfig, err := config.GetControlPlaneConfig(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get control plane config:", err)
			os.Exit(1)
		}

		controllers, err := controllersForRecordedApis(recordedInstance)
		if err != nil {
			cli.Error("failed to select controllers from the recorded apis list", err)
			os.Exit(1)
		}
		err = installer.UpdateDeploymentImageTag(
			updateImageTag,
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
			_, err := client.UpdateControlPlaneInstance(
				apiClient,
				apiEndpoint,
				&api_v0.ControlPlaneInstance{
					Common:   api_v0.Common{ID: controlPlaneInstance.ID},
					ImageTag: &updateImageTag,
				},
			)
			return err
		}); err != nil {
			cli.Error("deployments were updated but the new image tag could not be recorded for child control planes:", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Succesfully updates all threeport deployments to image: %s", updateImageTag))
	},
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
	UpgradeCmd.AddCommand(UpgradeControlPlaneCmd)

	UpgradeControlPlaneCmd.Flags().StringVarP(
		&updateImageTag,
		"version", "t", "", "version to update Threeport Control plane.",
	)

	UpgradeControlPlaneCmd.MarkFlagRequired("version")
}
