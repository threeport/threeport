package sdk

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	SDKConfigName = "sdk-config"
	SDKConfigType = "yaml"
)

// SDKConfig contains the config for the threeport sdk to use
// It is a map of controller domains and the api objects under them
type SDKConfig struct {
	APIObjects map[string][]*APIObject `yaml:"APIObjects"`
}

// APIObjectValues contains the attributes needed to manage a threeport api object.
type APIObject struct {
	Name         *string `yaml:"Name"`
	Reconcilable *bool   `yaml:"Reconcilable"`
	RouteExclude *bool   `yaml:"RouteExclude"`
}

type APIObjectConfig struct {
	SDKConfig `yaml:",inline"`
}

// GetSDKConfig retrieves the sdk config
func GetSDKConfig() (*SDKConfig, error) {
	sdkConfig := &SDKConfig{}
	if err := viper.Unmarshal(sdkConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return sdkConfig, nil
}

// InitConfig sets up the sdk config for the threeport sdk.
func InitConfig() error {
	// determine current working directory
	path, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine current working directory %w", err)
	}

	viper.AddConfigPath(DefaultSDKConfigPath(path))
	viper.SetConfigName(SDKConfigName)
	viper.SetConfigType(SDKConfigType)
	cfgFile := filepath.Join(
		DefaultSDKConfigPath(path),
		fmt.Sprintf("%s.%s", SDKConfigName, SDKConfigType),
	)

	viper.SetConfigFile(cfgFile)

	// ensure config permissions are read/write for user only
	if err := os.Chmod(cfgFile, 0600); err != nil {
		return fmt.Errorf("failed to set permissions to read/write only: %w", err)
	}

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	return nil
}

// DefaultSDKConfigPath returns the default path to the sdk config
// file on the user's filesystem.
func DefaultSDKConfigPath(path string) string {
	return filepath.Join(path, ".threeport")
}
