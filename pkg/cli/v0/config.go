package v0

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	config "github.com/threeport/threeport/pkg/config/v0"
)

// InitConfig sets up the threeport config for a CLI.
func InitConfig(cmd *cobra.Command, cfgFile string) {
	cfgFile = config.DetermineThreeportConfigPath(cfgFile)

	// ensure config file exists (except for up command which creates it)
	if cmd.Use != "up" {
		if _, err := os.Stat(cfgFile); errors.Is(err, os.ErrNotExist) {
			Error(fmt.Sprintf("config file %s does not exist", cfgFile), err)
			os.Exit(1)
		}
	}

	viper.SetConfigFile(cfgFile)

	if err := viper.ReadInConfig(); err != nil {
		Error("failed to read config", err)
		os.Exit(1)
	}
}
