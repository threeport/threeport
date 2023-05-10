/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/cli"
	configInternal "github.com/threeport/threeport/internal/config"
)

var (
	cfgFile           string
	providerConfigDir string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tptctl",
	Short: "Manage Threeport",
	Long: `Threeport is a global control plane for your software.  The tptctl
CLI installs and manages instances of the Threeport control plane as well as
applications that are deployed into the Threeport compute space.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&cfgFile, "threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml).",
	)
	rootCmd.PersistentFlags().StringVar(
		&providerConfigDir, "provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	)
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	cobra.OnInitialize(func() {
		configInternal.InitConfig(cfgFile, providerConfigDir)
	})
	cobra.OnInitialize(providerConfig)
}

func providerConfig() {
	// determine user home dir
	home, err := homedir.Dir()
	if err != nil {
		cli.Error("failed to determine user home directory", err)
		os.Exit(1)
	}

	if providerConfigDir == "" {
		if err := os.MkdirAll(configInternal.DefaultThreeportConfigPath(home), os.ModePerm); err != nil {
			cli.Error("failed to write create config directory", err)
			os.Exit(1)

		}
		providerConfigDir = configInternal.DefaultThreeportConfigPath(home)
	}
}
