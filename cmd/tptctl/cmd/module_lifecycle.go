package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var moduleControlPlane string
var moduleInstallImageNamespace string
var moduleInstallImageTag string
var moduleUpgradeImageNamespace string
var moduleUpgradeImageTag string

// ModuleCmd installs, upgrades, and uninstalls a module.
var ModuleCmd = &cobra.Command{
	Use:   "module",
	Short: "Install, upgrade, and uninstall a module",
}

// ModuleInstallCmd creates the module install job on a control plane.
var ModuleInstallCmd = &cobra.Command{
	Use:          "install NAME",
	Short:        "Install a module",
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Args:         cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
		encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get encryption key", err)
			os.Exit(1)
		}
		if err := cli.InstallModule(
			apiClient,
			apiEndpoint,
			encryptionKey,
			moduleControlPlane,
			args[0],
			moduleInstallImageNamespace,
			moduleInstallImageTag,
		); err != nil {
			cli.Error("failed to install module", err)
			os.Exit(1)
		}
		cli.Complete(fmt.Sprintf("module %s install started", args[0]))
	},
}

// ModuleUpgradeCmd records a module image and wakes the owning control plane.
var ModuleUpgradeCmd = &cobra.Command{
	Use:          "upgrade NAME",
	Short:        "Upgrade a module",
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Args:         cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
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
			moduleControlPlane,
			args[0],
			moduleUpgradeImageNamespace,
			moduleUpgradeImageTag,
		); err != nil {
			cli.Error("failed to upgrade module", err)
			os.Exit(1)
		}
		cli.Complete(fmt.Sprintf("module %s upgrade started", args[0]))
	},
}

// ModuleUninstallCmd removes a module from a control plane.
var ModuleUninstallCmd = &cobra.Command{
	Use:          "uninstall NAME",
	Short:        "Uninstall a module",
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Args:         cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
		encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get encryption key", err)
			os.Exit(1)
		}
		if err := cli.UninstallModule(
			apiClient,
			apiEndpoint,
			encryptionKey,
			moduleControlPlane,
			args[0],
		); err != nil {
			cli.Error("failed to uninstall module", err)
			os.Exit(1)
		}
		cli.Complete(fmt.Sprintf("module %s uninstalled", args[0]))
	},
}

// init registers the module commands and their flags.
func init() {
	// register the commands and their flags
	rootCmd.AddCommand(ModuleCmd)
	ModuleCmd.AddCommand(ModuleInstallCmd)
	ModuleCmd.AddCommand(ModuleUpgradeCmd)
	ModuleCmd.AddCommand(ModuleUninstallCmd)

	ModuleInstallCmd.Flags().StringVar(
		&moduleInstallImageNamespace,
		"image-namespace", "", "Image namespace of the module",
	)
	ModuleInstallCmd.Flags().StringVar(
		&moduleInstallImageTag,
		"image-tag", "", "Image tag of the module",
	)
	ModuleInstallCmd.Flags().StringVar(
		&moduleControlPlane,
		"control-plane", "", "Name of a control plane instance to install into. A child cannot install into itself.",
	)
	ModuleInstallCmd.MarkFlagRequired("image-namespace")
	ModuleInstallCmd.MarkFlagRequired("image-tag")

	ModuleUpgradeCmd.Flags().StringVar(
		&moduleUpgradeImageNamespace,
		"image-namespace", "", "Image namespace of the module",
	)
	ModuleUpgradeCmd.Flags().StringVar(
		&moduleUpgradeImageTag,
		"image-tag", "", "Image tag of the module",
	)
	ModuleUpgradeCmd.Flags().StringVar(
		&moduleControlPlane,
		"control-plane", "", "Name of a control plane instance to upgrade. Defaults to the current control plane.",
	)
	ModuleUpgradeCmd.MarkFlagRequired("image-namespace")
	ModuleUpgradeCmd.MarkFlagRequired("image-tag")

	ModuleUninstallCmd.Flags().StringVar(
		&moduleControlPlane,
		"control-plane", "", "Name of a control plane instance to uninstall from. A child cannot uninstall from itself.",
	)
}
