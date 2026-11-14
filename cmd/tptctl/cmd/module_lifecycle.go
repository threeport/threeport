package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/version"
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
			return cli.UpgradeBlueGreen(
				threeportConfig,
				controlPlane,
				apiClient,
				apiEndpoint,
				encryptionKey,
				moduleUpgradeControlPlane,
				moduleNewControlPlaneName,
				childTag,
			)
		},
	)
	if err != nil {
		cli.Error("failed to upgrade module", err)
		os.Exit(1)
	}
	cli.Complete(fmt.Sprintf("module %s upgrade started", moduleUpgradeName))
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
}
