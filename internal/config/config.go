package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"github.com/threeport/threeport/internal/cli"
	config "github.com/threeport/threeport/pkg/config/v0"
)

const (
	configName = "config"
	configType = "yaml"
)

func InitConfig(cfgFile, providerConfigDir string) {
	// determine user home dir
	home, err := homedir.Dir()
	if err != nil {
		cli.Error("failed to determine user home directory", err)
		os.Exit(1)
	}

	// set default threeport config path if not set by user
	if cfgFile == "" {
		viper.AddConfigPath(DefaultThreeportConfigPath(home))
		viper.SetConfigName(configName)
		viper.SetConfigType(configType)
		cfgFile = filepath.Join(DefaultThreeportConfigPath(home), fmt.Sprintf("%s.%s", configName, configType))
	}

	// create file if it doesn't exit
	if _, err := os.Stat(cfgFile); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(DefaultThreeportConfigPath(home), os.ModePerm); err != nil {
			cli.Error("failed to create config directory", err)
			os.Exit(1)
		}
		if err := viper.WriteConfigAs(cfgFile); err != nil {
			cli.Error("failed to write config to disk", err)
			os.Exit(1)
		}
	}

	viper.SetConfigFile(cfgFile)

	// ensure config permissions are read/write for user only
	if err := os.Chmod(cfgFile, 0600); err != nil {
		cli.Error("failed to set permissions to read/write only", err)
		os.Exit(1)
	}

	if err := viper.ReadInConfig(); err != nil {
		cli.Error("failed to read config", err)
		os.Exit(1)

	}
}

// GetThreeportConfig retrieves the threeport config
func GetThreeportConfig() (*config.ThreeportConfig, error) {
	threeportConfig := &config.ThreeportConfig{}
	if err := viper.Unmarshal(threeportConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return threeportConfig, nil
}

// UpdateThreeportConfig updates a threeport config to add a new instance and
// set it as the current instance.
func UpdateThreeportConfig(threeportInstanceConfigExists bool, threeportConfig *config.ThreeportConfig, createThreeportInstanceName string, newThreeportInstance *config.Instance) {
	if threeportInstanceConfigExists {
		for n, instance := range threeportConfig.Instances {
			if instance.Name == createThreeportInstanceName {
				threeportConfig.Instances[n] = *newThreeportInstance
			}
		}
	} else {
		threeportConfig.Instances = append(threeportConfig.Instances, *newThreeportInstance)
	}
	viper.Set("Instances", threeportConfig.Instances)
	viper.Set("CurrentInstance", createThreeportInstanceName)
	viper.WriteConfig()
}

// DeleteThreeportConfigInstance updates a threeport config to remove a deleted
// threeport instance and the current instance.
func DeleteThreeportConfigInstance(threeportConfig *config.ThreeportConfig, deleteThreeportInstanceName string) {
	updatedInstances := []config.Instance{}
	for _, instance := range threeportConfig.Instances {
		if instance.Name == deleteThreeportInstanceName {
			continue
		} else {
			updatedInstances = append(updatedInstances, instance)
		}
	}

	viper.Set("Instances", updatedInstances)
	viper.Set("CurrentInstance", "")
	viper.WriteConfig()
}

// DefaultThreeportConfigPath returns the default path to the threeport config
// file on the user's filesystem.
func DefaultThreeportConfigPath(homedir string) string {
	return filepath.Join(homedir, ".config", "threeport")
}
