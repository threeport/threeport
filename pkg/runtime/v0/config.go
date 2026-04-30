package v0

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/threeport/threeport/pkg/msg"
)

// LoadRuntimeConfig loads the config file with runtime parameters for the API
// server and controllers.
func LoadRuntimeConfig(configFile string) error {
	// if configFile is an empty string, we assume there is no runtime config
	// and return without error
	if configFile == "" {
		return nil
	}

	// parse filepath
	configPath, filename := filepath.Split(configFile)

	lastDotIndex := strings.LastIndex(filename, ".")
	configName := filename[:lastDotIndex]
	configType := filename[lastDotIndex+1:]

	viper.AddConfigPath(configPath)
	viper.SetConfigName(configName)
	viper.SetConfigType(configType)

	cfgFile := filepath.Join(
		configPath,
		fmt.Sprintf("%s.%s", configName, configType),
	)

	viper.SetConfigFile(cfgFile)

	viper.OnConfigChange(func(e fsnotify.Event) {
		msg.Info(fmt.Sprintf("server config file changed: %s", e.Name))
	})
	viper.WatchConfig()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read api server config: %w", err)
	}

	return nil
}
