/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	internalCmd "github.com/threeport/threeport/internal/cmd"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/tptdev"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var cliArgs *config.CLIArgs

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := internalCmd.CreateControlPlane(cliArgs)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
	cliArgs = &config.CLIArgs{}
	cliArgs.InfraProvider = "kind"
	cliArgs.DevEnvironment = true

	upCmd.Flags().StringVarP(
		&cliArgs.KubeconfigPath,
		"kubeconfig", "k", "", "Path to kubeconfig (default is ~/.kube/config).",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.ForceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config. Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.AuthEnabled,
		"auth-enabled", false, "Enable client certificate authentication (default is false).",
	)
	upCmd.Flags().StringVarP(
		&cliArgs.InstanceName,
		"name", "n", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	)
	upCmd.Flags().StringVarP(
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
	upCmd.Flags().IntVar(
		&cliArgs.ThreeportLocalAPIPort,
		"threeport-api-port", 443, "Local port to bind threeport APIServer to (default is 443).",
	)
	upCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	)
	cobra.OnInitialize(func() {
		config.InitConfig(cliArgs.CfgFile, cliArgs.ProviderConfigDir)
	})

	// get kubeconfig to use for kind cluster
	if cliArgs.KindKubeconfigPath == "" {
		k, err := kube.DefaultKubeconfig()
		if err != nil {
			cli.Error("failed to get default kubeconfig path", err)
			os.Exit(1)
		}
		cliArgs.KindKubeconfigPath = k
	}

	// set default threeport repo path if not provided
	// this is needed to map the container path to the host path for live
	// reloads of the code
	if cliArgs.ThreeportPath == "" {
		tp, err := os.Getwd()
		if err != nil {
			cli.Error("failed to get current working directory", err)
			os.Exit(1)
		}
		cliArgs.ThreeportPath = tp
	}

}
