package v0

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"

	config "github.com/threeport/threeport/pkg/config/v0"
)

// InitConfig sets up the threeport config for a CLI.
func InitConfig(cfgFile, providerConfigDir string) {
	// determine user home dir
	home, err := homedir.Dir()
	if err != nil {
		Error("failed to determine user home directory", err)
		os.Exit(1)
	}

	// set default threeport config path if not set by user
	if cfgFile == "" {
		viper.AddConfigPath(config.DefaultThreeportConfigPath(home))
		viper.SetConfigName(config.ThreeportConfigName)
		viper.SetConfigType(config.ThreeportConfigType)
		cfgFile = filepath.Join(
			config.DefaultThreeportConfigPath(home),
			fmt.Sprintf("%s.%s", config.ThreeportConfigName, config.ThreeportConfigType),
		)
	}

	// create file if it doesn't exit
	if _, err := os.Stat(cfgFile); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(config.DefaultThreeportConfigPath(home), os.ModePerm); err != nil {
			Error("failed to create config directory", err)
			os.Exit(1)
		}
		if err := viper.WriteConfigAs(cfgFile); err != nil {
			Error("failed to write config to disk", err)
			os.Exit(1)
		}
	}

	viper.SetConfigFile(cfgFile)

	// ensure config permissions are read/write for user only
	if err := os.Chmod(cfgFile, 0600); err != nil {
		Error("failed to set permissions to read/write only", err)
		os.Exit(1)
	}

	if err := viper.ReadInConfig(); err != nil {
		Error("failed to read config", err)
		os.Exit(1)
	}
}
