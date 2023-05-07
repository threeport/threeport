package config

import (
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

func InitConfig(cfgFile, providerConfigDir string) error {
	// determine user home dir
	home, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("failed to determine user home directory: %w", err)
	}
	viper.AddConfigPath(configPath(home))
	viper.SetConfigName(configName)
	viper.SetConfigType(configType)
	configFilePath := filepath.Join(configPath(home), fmt.Sprintf("%s.%s", configName, configType))

	// read config file if provided, else go to default
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {

		// create config if not present
		if err := viper.SafeWriteConfigAs(configFilePath); err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(configPath(home), os.ModePerm); err != nil {
					return fmt.Errorf("failed to create config directory: %w", err)
				}
				if err := viper.WriteConfigAs(configFilePath); err != nil {
					return fmt.Errorf("failed to write config to disk: %w", err)
				}
			}
		}
	}

	if providerConfigDir == "" {
		if err := os.MkdirAll(configPath(home), os.ModePerm); err != nil {
			return fmt.Errorf("failed to write create config directory: %w", err)
		}
		providerConfigDir = configPath(home)
	}

	// ensure config permissions are read/write for user only
	if err := os.Chmod(configFilePath, 0600); err != nil {
		cli.Error("failed to set permissions to read/write only", err)
		os.Exit(1)
	}

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}
	return nil
}

func GetThreeportConfig() (*config.ThreeportConfig, error) {
	// get threeport config
	threeportConfig := &config.ThreeportConfig{}
	if err := viper.Unmarshal(threeportConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return threeportConfig, nil
}

func UpdateThreeportConfig(threeportInstanceConfigExists bool, threeportConfig *config.ThreeportConfig, createThreeportInstanceName string, newThreeportInstance *config.Instance) {

	// update threeport config to add the new instance and set as current instance
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

func DeleteThreeportConfigInstance(threeportConfig *config.ThreeportConfig, deleteThreeportInstanceName string) {

	// update threeport config to remove the deleted threeport instance and
	// current instance
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

func configPath(homedir string) string {
	//return fmt.Sprintf("%s/.config/threeport", homedir)
	return filepath.Join(homedir, ".config", "threeport")
}
