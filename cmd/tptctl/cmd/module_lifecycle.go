package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/version"
	api "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const controlPlaneControllerDeploy = "threeport-" + installer.ThreeportControlPlaneControllerName

var moduleUpgradeControlPlane string
var moduleUpgradeName string
var moduleUpgradeImageNamespace string
var moduleUpgradeImageTag string
var moduleNewControlPlaneName string

// UpgradeModuleCmd updates a module through the control plane that owns it.
var UpgradeModuleCmd = &cobra.Command{
	Use:   "module",
	Short: "Update a module through its control plane",
	Long:  `Update a module through its control plane.`,
	Run: func(cmd *cobra.Command, args []string) {
		missingErr("upgrade module")
		os.Exit(1)
	},
}

// UpgradeModuleInPlaceCmd records a new image on the module.
var UpgradeModuleInPlaceCmd = &cobra.Command{
	Use:          "image",
	Short:        "Record a new image on the module",
	Long:         `Record a new image on the module.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run:          runModuleImageUpgrade,
}

// UpgradeModuleBlueGreenCmd copies the control plane that runs the module.
var UpgradeModuleBlueGreenCmd = &cobra.Command{
	Use:          "blue-green",
	Short:        "Copy the control plane that runs the module",
	Long:         `Copy the control plane that runs the module.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run:          runModuleBlueGreenUpgrade,
}

// runModuleImageUpgrade records the module image from the shared flags.
func runModuleImageUpgrade(cmd *cobra.Command, args []string) {
	// record the image the module runs
	apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
	encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
	if err != nil {
		cli.Error("failed to get encryption key", err)
		os.Exit(1)
	}
	if err := cli.UpgradeModule(
		apiClient,
		apiEndpoint,
		encryptionKey,
		moduleUpgradeControlPlane,
		moduleUpgradeName,
		moduleUpgradeImageNamespace,
		moduleUpgradeImageTag,
		true,
	); err != nil {
		cli.Error("failed to upgrade module", err)
		os.Exit(1)
	}
	cli.Complete(fmt.Sprintf("module %s upgrade started", moduleUpgradeName))
}

// recordModuleThenInstall writes the module image before the backup that blue-green takes.
func recordModuleThenInstall(record func() error, install func() error) error {
	// the restored database contains the module image only if the write happens first
	if err := record(); err != nil {
		return err
	}
	return install()
}

// runModuleBlueGreenUpgrade records the module image, then requests a new control plane.
func runModuleBlueGreenUpgrade(cmd *cobra.Command, args []string) {
	// record the module image before requesting the new control plane
	apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
	encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
	if err != nil {
		cli.Error("failed to get encryption key", err)
		os.Exit(1)
	}
	controlPlane, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		cli.Error("failed to get control plane config", err)
		os.Exit(1)
	}
	err = recordModuleThenInstall(
		func() error {
			return cli.UpgradeModule(
				apiClient,
				apiEndpoint,
				encryptionKey,
				moduleUpgradeControlPlane,
				moduleUpgradeName,
				moduleUpgradeImageNamespace,
				moduleUpgradeImageTag,
				false,
			)
		},
		func() error {
			childTag := ""
			if moduleUpgradeControlPlane != "" {
				_, childTag, err = parentControlPlaneImage(apiClient, apiEndpoint, encryptionKey, true)
				if err != nil {
					return err
				}
			}

			// keep the version the replaced control plane runs for a flag left empty
			running, err := runningControlPlane(apiClient, apiEndpoint, moduleUpgradeControlPlane)
			if err != nil {
				return err
			}
			imageNamespace, imageTag := moduleBlueGreenImage(upgradeImageNamespace, upgradeImageTag, running)
			return cli.UpgradeBlueGreen(
				threeportConfig,
				controlPlane,
				apiClient,
				apiEndpoint,
				encryptionKey,
				moduleUpgradeControlPlane,
				moduleNewControlPlaneName,
				childTag,
				imageNamespace,
				imageTag,
			)
		},
	)
	if err != nil {
		cli.Error("failed to upgrade module", err)
		os.Exit(1)
	}
	cli.Complete(fmt.Sprintf("module %s upgrade started", moduleUpgradeName))
}

// runningControlPlane returns the control plane a module blue-green replaces: the
// named child, or the connected control plane when the name is empty.
func runningControlPlane(
	apiClient *http.Client,
	apiEndpoint string,
	name string,
) (*api.ControlPlaneInstance, error) {
	// the connected control plane is the one replaced when no child is named
	if name == "" {
		self, err := client.GetSelfControlPlaneInstance(apiClient, apiEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to get control plane instance: %w", err)
		}
		return self, nil
	}

	child, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane %s: %w", name, err)
	}
	return child, nil
}

// moduleBlueGreenImage returns the image namespace and tag of the new control
// plane. A value from its flag wins. A flag left empty takes the value the
// replaced control plane runs, so a module upgrade leaves the control plane
// version alone unless the caller names one. A control plane that records no
// image leaves the value empty for the install's own default.
func moduleBlueGreenImage(
	flagNamespace string,
	flagTag string,
	running *api.ControlPlaneInstance,
) (string, string) {
	imageNamespace, imageTag := flagNamespace, flagTag
	if running == nil {
		return imageNamespace, imageTag
	}
	if imageNamespace == "" {
		imageNamespace = util.DerefString(running.ImageNamespace)
	}
	if imageTag == "" {
		imageTag = util.DerefString(running.ImageTag)
	}
	return imageNamespace, imageTag
}

// parentControlPlaneImage returns the image namespace and tag for the new control plane.
// An empty child name uses this tptctl build. A child name uses the parent's deployment.
func parentControlPlaneImage(
	apiClient *http.Client,
	apiEndpoint string,
	encryptionKey string,
	namedChild bool,
) (string, string, error) {
	// load the connected control plane for its namespace and image
	self, err := client.GetSelfControlPlaneInstance(apiClient, apiEndpoint)
	if err != nil {
		return "", "", fmt.Errorf("failed to get control plane instance: %w", err)
	}
	imageNamespace := util.DerefString(self.ImageNamespace)
	if !namedChild {
		return imageNamespace, version.GetVersion(), nil
	}

	// read the running control-plane-controller deployment on the parent
	tag := ""
	if self.KubernetesRuntimeInstanceID != nil {
		runtime, runtimeErr := client.GetKubernetesRuntimeInstanceByID(
			apiClient,
			apiEndpoint,
			*self.KubernetesRuntimeInstanceID,
		)
		if runtimeErr == nil {
			kubeClient, mapper, kubeErr := kube.GetClient(
				runtime,
				false,
				apiClient,
				apiEndpoint,
				encryptionKey,
			)
			if kubeErr == nil && mapper != nil {
				deploy, deployErr := kube.GetResource(
					"apps",
					"v1",
					"Deployment",
					util.DerefString(self.Namespace),
					controlPlaneControllerDeploy,
					kubeClient,
					*mapper,
				)
				if deployErr == nil {
					tag = deploymentImageTag(deploy)
				}
			}
		}
	}

	// use the parent control plane instance image tag when the deployment is missing
	if tag == "" {
		tag = util.DerefString(self.ImageTag)
	}
	return imageNamespace, tag, nil
}

// deploymentImageTag returns the tag of the first container image.
func deploymentImageTag(deploy *unstructured.Unstructured) string {
	// read the first container image
	containers, found, err := unstructured.NestedSlice(deploy.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return ""
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return ""
	}
	image, _, _ := unstructured.NestedString(container, "image")
	if slash := strings.LastIndex(image, "/"); slash >= 0 {
		image = image[slash+1:]
	}
	colon := strings.LastIndex(image, ":")
	if colon < 0 || colon == len(image)-1 {
		return ""
	}
	return image[colon+1:]
}

// init registers the module upgrade commands and their flags.
func init() {
	// register the upgrade commands and their flags
	UpgradeCmd.AddCommand(UpgradeModuleCmd)
	UpgradeModuleCmd.AddCommand(UpgradeModuleInPlaceCmd)
	UpgradeModuleCmd.AddCommand(UpgradeModuleBlueGreenCmd)

	UpgradeModuleCmd.PersistentFlags().StringVar(
		&moduleUpgradeName,
		"module-name", "", "Name of the module",
	)
	UpgradeModuleCmd.PersistentFlags().StringVar(
		&moduleUpgradeImageNamespace,
		"module-image-namespace", "", "Image namespace of the module",
	)
	UpgradeModuleCmd.PersistentFlags().StringVar(
		&moduleUpgradeImageTag,
		"module-image-tag", "", "Image tag of the module",
	)
	UpgradeModuleCmd.PersistentFlags().StringVar(
		&moduleUpgradeControlPlane,
		"control-plane-name", "", "Name of a child control plane. Empty uses the connected control plane.",
	)
	UpgradeModuleCmd.MarkPersistentFlagRequired("module-name")
	UpgradeModuleCmd.MarkPersistentFlagRequired("module-image-tag")
	UpgradeModuleBlueGreenCmd.Flags().StringVar(
		&moduleNewControlPlaneName,
		"new-control-plane-name", "", "Name of the new control plane",
	)
	UpgradeModuleBlueGreenCmd.MarkFlagRequired("new-control-plane-name")
	UpgradeModuleBlueGreenCmd.Flags().StringVar(
		&upgradeImageNamespace,
		"control-plane-image-namespace", "", "Image namespace of the new control plane. Empty keeps the namespace the replaced control plane runs.",
	)
	UpgradeModuleBlueGreenCmd.Flags().StringVar(
		&upgradeImageTag,
		"control-plane-image-tag", "", "Image tag of the new control plane. Empty keeps the tag the replaced control plane runs.",
	)
}
