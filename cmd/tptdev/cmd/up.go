/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		err = cli.CreateGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVarP(
		&cliArgs.KubeconfigPath,
		"kubeconfig", "k", "", "Path to kubeconfig (default is ~/.kube/config).",
	)
	buildCmd.Flags().BoolVar(
		&cliArgs.ForceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config. Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	buildCmd.Flags().BoolVar(
		&cliArgs.AuthEnabled,
		"auth-enabled", false, "Enable client certificate authentication (default is false).",
	)
	buildCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	)
	buildCmd.Flags().StringVarP(
		&cliArgs.ThreeportPath,
		"threeport-path", "t", "", "Path to threeport repository root (default is './').",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.CfgFile,
		"threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml).",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.ProviderConfigDir,
		"provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	)
	buildCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	)
	cobra.OnInitialize(func() {
		cli.InitConfig(cliArgs.CfgFile)
	})
}
