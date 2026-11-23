package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/version"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

var blueGreenNewName string

// upgradeDryRun stops a blue-green upgrade after its preflight checks.
var upgradeDryRun bool

// UpgradeBlueGreenCmd creates a new control plane from a backup of an existing one.
var UpgradeBlueGreenCmd = &cobra.Command{
	Use:   "blue-green",
	Short: "Create a new control plane beside an existing one",
	Long: `Back up the source control plane and create a new one that restores from it.

The new control plane runs this tptctl version in a namespace of the same name.
--control-plane-image-namespace and --control-plane-image-tag override that image. Omit them to keep the default.
--control-plane-name selects the control plane the command connects to.
--old-control-plane-name selects a child. The controller installs the image recorded on that child.
The source stays stopped unless the new control plane fails to come up.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		// refuse a connection to a child before selecting a source
		self, err := client.GetSelfControlPlaneInstance(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to get control plane instance", err)
			os.Exit(1)
		}
		if err := api_v0.ChildControlPlaneRequest(self); err != nil {
			cli.Error("failed to create control plane", fmt.Errorf("control plane %s: %w", requestedControlPlane, err))
			os.Exit(1)
		}

		// a name is a descendant, and the controller installs it
		sourceName := ""
		if upgradeOldControlPlaneName != "" {
			if _, err := upgradeRollTarget(
				self,
				upgradeOldControlPlaneName,
				func(name string) (*api_v0.ControlPlaneInstance, error) {
					return client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, name)
				},
				func(id uint) (*api_v0.ControlPlaneInstance, error) {
					return client.GetControlPlaneInstanceByID(apiClient, apiEndpoint, id)
				},
			); err != nil {
				cli.Error("failed to create control plane", err)
				os.Exit(1)
			}
			sourceName = upgradeOldControlPlaneName
		}

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
		childTag := ""
		if sourceName != "" {
			_, childTag, err = parentControlPlaneImage(apiClient, apiEndpoint, encryptionKey, true)
			if err != nil {
				cli.Error("failed to read control plane controller image", err)
				os.Exit(1)
			}
		}
		// check the upgrade before anything stops, using the tag the run would use
		checkTag := upgradeImageTag
		if checkTag == "" {
			checkTag = childTag
		}
		if checkTag == "" {
			checkTag = version.GetVersion()
		}
		if err := cli.PreflightBlueGreen(apiClient, apiEndpoint, cli.BlueGreenPreflight{
			SourceName:     sourceName,
			NewName:        blueGreenNewName,
			ImageNamespace: upgradeImageNamespace,
			ImageTag:       checkTag,
		}); err != nil {
			cli.Error("preflight failed", err)
			os.Exit(1)
		}
		if upgradeDryRun {
			cli.Complete("preflight passed, nothing changed")
			return
		}
		if err := cli.UpgradeBlueGreen(
			threeportConfig,
			controlPlane,
			apiClient,
			apiEndpoint,
			encryptionKey,
			sourceName,
			blueGreenNewName,
			childTag,
			upgradeImageNamespace,
			upgradeImageTag,
			"",
		); err != nil {
			cli.Error("failed to create control plane", err)
			os.Exit(1)
		}
		cli.Complete(fmt.Sprintf("control plane %s created", blueGreenNewName))
	},
}

// init registers the blue-green upgrade command and its flags.
func init() {
	// register the new control plane name
	UpgradeBlueGreenCmd.Flags().StringVar(
		&blueGreenNewName,
		"new-control-plane-name", "", "Name of the new control plane",
	)
	UpgradeBlueGreenCmd.MarkFlagRequired("new-control-plane-name")
	UpgradeBlueGreenCmd.Flags().StringVar(
		&upgradeImageNamespace,
		"control-plane-image-namespace", "", "Image namespace of the new control plane. Empty keeps the source namespace.",
	)
	UpgradeBlueGreenCmd.Flags().StringVar(
		&upgradeImageTag,
		"control-plane-image-tag", "", "Image tag of the new control plane. Empty uses this tptctl version, or the parent controller tag for a child.",
	)
	UpgradeBlueGreenCmd.Flags().BoolVar(
		&upgradeDryRun,
		"dry-run", false, "Run the preflight checks and stop before anything changes.",
	)
}
