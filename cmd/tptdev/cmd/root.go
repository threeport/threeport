/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/cli"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tptdev",
	Short: "Manage threeport development environments",
	Long:  `Manage threeport development environments.`,
}

var cliArgs = &cli.ControlPlaneCLIArgs{}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tptdev.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	cobra.OnInitialize(func() {
		cli.InitConfig(cliArgs.CfgFile, cliArgs.ProviderConfigDir)
		cli.InitArgs(cliArgs)

		cliArgs.InfraProvider = "kind"
		cliArgs.DevEnvironment = true
	})
}
