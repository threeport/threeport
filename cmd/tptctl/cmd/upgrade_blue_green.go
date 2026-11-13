package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var blueGreenName string
var blueGreenNamespace string
var blueGreenSource string
var blueGreenImageNamespace string
var blueGreenImageTag string

// UpgradeBlueGreenCmd creates a new control plane from a backup of an existing one.
var UpgradeBlueGreenCmd = &cobra.Command{
	Use:   "blue-green",
	Short: "Create a new control plane beside an existing one",
	Long: `Back up the source control plane and create a new one that restores from it.

--image-namespace and --image-tag set the image the new control plane installs.
The source stays stopped unless the new control plane fails to come up.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		if blueGreenName == "" || blueGreenNamespace == "" {
			cli.Error("name and namespace are required", fmt.Errorf("name %q namespace %q", blueGreenName, blueGreenNamespace))
			os.Exit(1)
		}

		apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
		controlPlane, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get control plane config", err)
			os.Exit(1)
		}
		encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get encryption key", err)
			os.Exit(1)
		}
		if err := cli.UpgradeBlueGreen(
			threeportConfig,
			controlPlane,
			apiClient,
			apiEndpoint,
			encryptionKey,
			blueGreenSource,
			blueGreenName,
			blueGreenNamespace,
			blueGreenImageNamespace,
			blueGreenImageTag,
		); err != nil {
			cli.Error("failed to create control plane", err)
			os.Exit(1)
		}
		cli.Complete(fmt.Sprintf("control plane %s created", blueGreenName))
	},
}

// init registers the blue-green upgrade command and its flags.
func init() {
	// register the command and its flags
	UpgradeCmd.AddCommand(UpgradeBlueGreenCmd)
	UpgradeBlueGreenCmd.Flags().StringVar(
		&blueGreenName,
		"name", "", "Name of the new control plane",
	)
	UpgradeBlueGreenCmd.Flags().StringVar(
		&blueGreenNamespace,
		"namespace", "", "Namespace of the new control plane",
	)
	UpgradeBlueGreenCmd.Flags().StringVar(
		&blueGreenSource,
		"control-plane", "", "Name of the control plane to copy. Defaults to the current control plane. A child cannot copy itself.",
	)
	UpgradeBlueGreenCmd.Flags().StringVar(
		&blueGreenImageNamespace,
		"image-namespace", "", "Image namespace of the new control plane",
	)
	UpgradeBlueGreenCmd.Flags().StringVar(
		&blueGreenImageTag,
		"image-tag", "", "Image tag of the new control plane",
	)
	UpgradeBlueGreenCmd.MarkFlagRequired("name")
	UpgradeBlueGreenCmd.MarkFlagRequired("namespace")
	UpgradeBlueGreenCmd.MarkFlagRequired("image-namespace")
	UpgradeBlueGreenCmd.MarkFlagRequired("image-tag")
}
