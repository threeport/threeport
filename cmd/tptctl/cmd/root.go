/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	configName = "config"
	configType = "yaml"
)

func configPath(homedir string) string {
	//return fmt.Sprintf("%s/.config/threeport", homedir)
	return filepath.Join(homedir, ".config", "threeport")
}

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
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "threeport-config", "",
		"path to config file - default is $HOME/.config/threeport/config.yaml")
	rootCmd.PersistentFlags().StringVar(&providerConfigDir, "provider-config", "",
		"path to infra provider config directory - default is $HOME/.config/threeport/")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func initConfig() {
	// determine user home dir
	home, err := homedir.Dir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	viper.AddConfigPath(configPath(home))
	viper.SetConfigName(configName)
	viper.SetConfigType(configType)
	//configFilePath := fmt.Sprintf("%s/%s.%s", configPath(home), configName, configType)
	configFilePath := filepath.Join(configPath(home), fmt.Sprintf("%s.%s", configName, configType))

	// read config file if provided, else go to default
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		//viper.AddConfigPath(configPath(home))
		//viper.SetConfigName(configName)
		//viper.SetConfigType(configType)

		// create config if not present
		//configFilePath := fmt.Sprintf("%s/%s.%s", configPath(home), configName, configType)
		if err := viper.SafeWriteConfigAs(configFilePath); err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(configPath(home), os.ModePerm); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := viper.WriteConfigAs(configFilePath); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			}
		}
	}

	if providerConfigDir == "" {
		if err := os.MkdirAll(configPath(home), os.ModePerm); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		providerConfigDir = configPath(home)
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Can't read config:", err)
		os.Exit(1)
	}
}
