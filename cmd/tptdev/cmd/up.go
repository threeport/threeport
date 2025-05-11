/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
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
		// update cli args based on env vars
		cliArgs.GetControlPlaneEnvVars()

		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}
		cpi.Opts.Debug = cliArgs.Debug

		err = cli.CreateGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			if errors.Is(cli.ErrThreeportConfigAlreadyExists, err) {
				cli.Info("if you wish to overwrite the existing config use --force-overwrite-config flag")
				cli.Warning("you will lose the ability to connect to the existing threeport control planes if they are still running")
			}
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(upCmd)

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
		&cliArgs.ControlPlaneName,
		"name", "n", tptdev.DefaultInstanceName, "Name of dev genesis control plane.",
	)
	upCmd.Flags().StringVarP(
		&cliArgs.ThreeportPath,
		"threeport-path", "p", "", "Path to threeport repository root (default is './').",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.CfgFile,
		"threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml). Can also be set with environment variable THREEPORT_CONFIG",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.ProviderConfigDir,
		"provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	)
	upCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	)
	upCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "r", "", "Alternate image repo to pull threeport control plane images from.",
	)
	upCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag to pull threeport control plane images from.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.ControlPlaneOnly,
		"control-plane-only", false, "Deploy the control plane on an existing runtime. Defaults to false.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.InfraOnly,
		"infra-only", false, "Deploy only the infrastructure without the control plane. Defaults to false.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.Debug,
		"debug", false, "Debug threeport control plane components.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.Verbose,
		"verbose", false, "Enable verbose logging in control plane components, delve, and cli logs.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.SkipTeardown,
		"skip-teardown", false, "Skip the teardown of control plane components if an error is encountered.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.LocalRegistry,
		"local-registry", false, "Connects a local container registry to Threeport control plane cluster.  Only applicable with provider 'kind'.",
	)
	cobra.OnInitialize(func() {
		cli.InitConfig(cliArgs.CfgFile)
	})
}
