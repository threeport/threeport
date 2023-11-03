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

var imageName string

// buildCmd represents the up command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		tptdev.BuildImage(
			cpi.Opts.ThreeportPath,
			cliArgs.ControlPlaneImageRepo,
			cliArgs.ControlPlaneImageTag,
			imageName)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVar(
		&imageName,
		"image-name", "", "Image name",
	)
	buildCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "", "Alternate image repo to pull threeport control plane images from.",
	)
	buildCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "", "Alternate image tag to pull threeport control plane images from.",
	)
	// buildCmd.Flags().BoolVar(
	// 	&cliArgs.AuthEnabled,
	// 	"auth-enabled", false, "Enable client certificate authentication (default is false).",
	// )
	// buildCmd.Flags().StringVarP(
	// 	&cliArgs.ControlPlaneName,
	// 	"name", "n", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	// )
	// buildCmd.Flags().StringVarP(
	// 	&cliArgs.ThreeportPath,
	// 	"threeport-path", "t", "", "Path to threeport repository root (default is './').",
	// )
	// rootCmd.PersistentFlags().StringVar(
	// 	&cliArgs.CfgFile,
	// 	"threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml).",
	// )
	// rootCmd.PersistentFlags().StringVar(
	// 	&cliArgs.ProviderConfigDir,
	// 	"provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	// )
	// buildCmd.Flags().IntVar(
	// 	&cliArgs.NumWorkerNodes,
	// 	"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	// )
	// cobra.OnInitialize(func() {
	// 	cli.InitConfig(cliArgs.CfgFile)
	// })
}
