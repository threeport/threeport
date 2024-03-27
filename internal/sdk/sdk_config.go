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

// SdkConfig contains all the configuration options available to a user
// of the SDK.
type SdkConfig struct {
	ApiObjectConfig `yaml:",inline"`
}

// ApiObjectGroups contains the config for all API object groups.
type ApiObjectConfig struct {
	ApiObjectGroups []*ApiObjectGroup `yaml:"ApiObjectGroups"`
}

// ApiObjectGroup is a collection of API objects and the attributes used
// for code generation.  When a group includes objects that are reconciled
// by a controller, it also represents a controller domain, i.e. a single controller
// manages reconciliation for all objects in an ApiObjectGroup.
type ApiObjectGroup struct {
	// Name of the api object group
	Name *string `yaml:"Name"`

	// List of api objects under the object group
	Objects []*ApiObject `yaml:"Objects"`
}

// ApiObject contains the attributes needed to manage a threeport api object.
type ApiObject struct {
	// Name of the api object to manage with threeport
	Name *string `yaml:"Name"`

	// Name of the api object to manage with threeport
	Versions []*string `yaml:"Versions"`

	// Indicate whether the object will need a controller
	// that is registered with the rest-api for reconciliation
	Reconcilable *bool `yaml:"Reconcilable"`

	// Indicate the message will be persisted by NATS
	DisableNotificationPersistence *bool `yaml:"DisableNotificationPersistence"`

	// Indicates whether the route should be exposed on the rest-api for the object
	// and whether the api model for this object needs to be generated
	ExcludeRoute *bool `yaml:"ExcludeRoute"`

	// Indicates whether the object needs to be maintained in a database
	ExcludeFromDb *bool `yaml:"ExcludeFromDb"`

	// AllowCustomMiddleware indicates whether the api model for this object needs custom middleware enabled
	AllowCustomMiddleware *bool `yaml:"AllowCustomMiddleware"`

	// AllowDuplicateModelNames indicates whether the api handler for this object accepts duplicate names objects
	AllowDuplicateModelNames *bool `yaml:"AllowDuplicateModelNames"`

	// LoadAssociationsFromDb indicates whether the response returned for an object contains associated object data
	LoadAssociationsFromDb *bool `yaml:"LoadAssociationsFromDb"`

	// Tptctl contains sdk configurations related to tptctl
	Tptctl *Tptctl `yaml:"Tptctl"`
}

// Tptctl contains attributes used by the SDK to generate tptctl
// command source code.
type Tptctl struct {
	Enabled    *bool `yaml:"Enabled"`
	ConfigPath *bool `yaml:"ConfigPath"`
}

// GetSDKConfig retrieves the sdk config
func GetSDKConfig() (*ApiObjectConfig, error) {
	sdkConfig := &ApiObjectConfig{}

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
