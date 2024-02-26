package sdk

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
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
	// Name of the api object to manage with threeport
	Name *string `yaml:"Name"`

	// Name of the api object to manage with threeport
	Versions []*string `yaml:"Versions"`

	// Indicate whether the object will need a controller
	// that is registered with the rest-api for reconciliation
	Reconcilable *bool `yaml:"Reconcilable"`

	// Indicates whether the route should be exposed on the rest-api for the object
	RouteExclude *bool `yaml:"RouteExclude"`

	// Indicates whether the object needs to be maintained in a database
	DatabaseExclude *bool `yaml:"DatabaseExclude"`

	// DisableApiModel indicates whether the api model for this object needs to be generated
	DisableApiModel *bool `yaml:"DisableApiModel"`

	// AllowCustomMiddleware indicates whether the api model for this object needs custom middleware enabled
	AllowCustomMiddleware *bool `yaml:"AllowCustomMiddleware"`

	// AllowDuplicateModelNames indicates whether the api handler for this object accepts duplicate names objects
	AllowDuplicateModelNames *bool `yaml:"AllowDuplicateModelNames"`

	// LoadAssociationsFromDatabase indicates whether the response returned for an object contains associated object data
	LoadAssociationsFromDatabase *bool `yaml:"LoadAssociationsFromDatabase"`
}

type APIObjectConfig struct {
	SDKConfig `yaml:",inline"`
}

// GetSDKConfig retrieves the sdk config
func GetSDKConfig() (*SDKConfig, error) {
	sdkConfig := &SDKConfig{}

	path, err := DefaultSDKConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine sdk config path %w", err)
	}

	configContent, err := ioutil.ReadFile(path) //read the content of file
	if err != nil {
		return nil, fmt.Errorf("could not read sdk config from location %s: %w", path, err)
	}

	if err := yaml.UnmarshalStrict(configContent, &sdkConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sdk config file yaml content: %w", err)
	}

	return sdkConfig, nil
}

// DefaultSDKConfigPath returns the default path to the sdk config
// file on the user's filesystem.
func DefaultSDKConfigPath() (string, error) {
	// determine current working directory
	path, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine current working directory %w", err)
	}
	return filepath.Join(path, ".threeport", fmt.Sprintf("%s.%s", SDKConfigName, SDKConfigType)), nil
}
