/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

// upApis holds the --apis flag value: a comma-separated list
// of sdk-config ApiObjectGroup names whose controllers to install.
var upApis string

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

		// narrow the controller list when --apis is set so the install
		// brings up only the requested apis' controllers alongside the
		// rest-api and agent.
		if upApis != "" {
			selected, err := installer.SelectControllersByGroup(
				parseApis(upApis),
				cpi.Opts.ControllerList,
			)
			if err != nil {
				cli.Error("failed to select controllers", err)
				os.Exit(1)
			}
			selectedNames := make([]string, 0, len(selected))
			for _, controller := range selected {
				selectedNames = append(selectedNames, controller.Name)
			}
			cli.Info(fmt.Sprintf(
				"limiting install to %d controller(s): %s",
				len(selected), strings.Join(selectedNames, ", "),
			))
			cpi.Opts.ControllerList = selected
		}

		err = cli.CreateGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			if errors.Is(err, cli.ErrThreeportConfigAlreadyExists) {
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
		"kubeconfig", "k", "", "Path to kubeconfig (default is $KUBECONFIG, then ~/.kube/config).",
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
		"threeport-config", "", "Path to config file (default is $HOME/.threeport/config.yaml). Can also be set with environment variable THREEPORT_CONFIG",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.ProviderConfigDir,
		"provider-config", "", "Path to infra provider config directory (default is $HOME/.threeport/).",
	)
	upCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	)
	upCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-namespace", "r", "", "Alternate image namespace to pull threeport control plane images from.",
	)
	upCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag for threeport control plane images.",
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
		&cliArgs.TeardownOnFailure,
		"teardown-on-failure", false, "Automatically tear down control plane resources if an error is encountered.",
	)
	upCmd.Flags().BoolVar(
		&cliArgs.LocalRegistry,
		"local-registry", false, "Connects a local container registry to Threeport control plane cluster.  Only applicable with provider 'kind'.",
	)
	upCmd.Flags().StringVar(
		&upApis,
		"apis", "",
		"Comma-separated sdk-config api names (e.g. kubernetes_workload,gateway) "+
			"whose controllers to install. NOT component names. When omitted, installs the "+
			"full default controller list.",
	)
	cobra.OnInitialize(func() {
		cli.InitConfig(upCmd, cliArgs.CfgFile)
	})
}
