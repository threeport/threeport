package sdk

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/sdk/util"
)

// SdkConfig contains all the configuration options available to a user
// of the SDK.
type SdkConfig struct {
	// ApiNamespace is the globally unique namespace for objects managed by this
	// API.  It prevents naming collisions between extension objects using in a
	// single Threeport control plane.  We recommend using a domain name you own
	// to make it globally unique.
	ApiNamespace string `yaml:"ApiNamespace"`

	// The image repository that will be used for builds of extensions
	// components.
	ImageRepo string `yaml:"ImageRepo"`

	// Details to be displayed on the API documentation page by API server.
	ApiDocs ApiDocs `yaml:"ApiDocs"`

	// The configuration of API objects used in extension.
	ApiObjectConfig `yaml:",inline"`
}

// ApiDocs contains the information displayed on the documentation page served
// by the API server.
type ApiDocs struct {
	// The title for the API documentation
	Title string `yaml:"Title"`

	// Description of the API.
	Description string `yaml:"Description"`

	// TosLink is a URL to the terms of service for the API.
	TosLink string `yaml:"TosLink"`

	// ContactName is the name of the primary contact for support.
	ContactName string `yaml:"ContactName"`

	// ContactUrl is a link to a support page online.
	ContactUrl string `yaml:"ContactUrl"`

	// ContactEmail is the email address to contact for support.
	ContactEmail string `yaml:"ContactEmail"`
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
	// Name of the api object group.
	Name *string `yaml:"Name"`

	// List of api objects under the object group.
	Objects []*ApiObject `yaml:"Objects"`
}

// ApiObject contains the attributes needed to manage a threeport api object.
type ApiObject struct {
	// Name of the api object to manage with threeport.
	Name *string `yaml:"Name"`

	// Name of the api object to manage with threeport.
	Versions []*string `yaml:"Versions"`

	// If false, acts as an override for API objects that have a "Definition" or
	// "Instance" suffix that do NOT want a connection established beteen them
	// for a DefinedInstance abstraction, e.g. WorkloadResourceDefinition and
	// WorkloadResourceInstance.  If this field is not included, the default
	// behavior is to create the DefinedInstance fields.
	//
	// If false on an API object with a "Definition" suffix:
	// * `threeport-sdk create` will not add a `Definition` field to the API
	//   object definition.
	// * `threeport-sdk create` will not add a slice of corresponding instances
	//   to the API object definition.
	// * `threeport-sdk gen` will not include a check in the API handler that
	//   deletes a definition to see if there are corresponding instances of the
	//   definition.
	//
	// If false on an API object with a "Instance" suffix:
	// * `threeport-sdk create` will not add an `Instance` field to the API
	//   object definition.
	// * `threeport-sdk create` will not add a foreign key reference back to the
	//   definition.
	DefinedInstance *bool `yaml:"DefinedInstance"`

	// Indicate whether the object will need a controller
	// that is registered with the rest-api for reconciliation.
	Reconcilable *bool `yaml:"Reconcilable"`

	// Indicate the message will be persisted by NATS
	DisableNotificationPersistence *bool `yaml:"DisableNotificationPersistence"`

	// Indicates whether the route should be exposed on the rest-api for the object
	// and whether the api model for this object needs to be generated.
	ExcludeRoute *bool `yaml:"ExcludeRoute"`

	// Indicates whether the object needs to be maintained in a database.
	ExcludeFromDb *bool `yaml:"ExcludeFromDb"`

	// AllowCustomMiddleware indicates whether the api model for this object
	// needs custom middleware enabled.
	AllowCustomMiddleware *bool `yaml:"AllowCustomMiddleware"`

	// AllowDuplicateModelNames indicates whether the api handler for this
	// object accepts duplicate names objects.
	AllowDuplicateModelNames *bool `yaml:"AllowDuplicateModelNames"`

	// LoadAssociationsFromDb indicates whether the response returned for an
	// object contains associated object data.
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

// GetSdkConfig reads, unmarshalls and returns the SDK config from the specified
// path.
func GetSdkConfig(configPath string) (*SdkConfig, error) {
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var sdkConfig SdkConfig
	if err := yaml.UnmarshalStrict(configContent, &sdkConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file yaml content: %w", err)
	}

	if err := ValidateSdkConfig(&sdkConfig); err != nil {
		return nil, fmt.Errorf("SDK config validation failed: %w", err)
	}

	return &sdkConfig, nil
}

// ValidateSdkConfig validates an SDK config.
func ValidateSdkConfig(sdkConfig *SdkConfig) error {
	// determine if repo is an extension
	extension, _, err := util.IsExtension()
	if err != nil {
		fmt.Errorf("failed to determine if generating code for an extension: %w", err)
	}

	if extension && sdkConfig.ApiNamespace == "" {
		return fmt.Errorf("ApiNamespace is a required field")
	}

	// check to make sure that defined instance objects have matching values for
	// the `DefinedInstance` field.  If they don't the API object definitions
	// will be incompatible.
	for _, objectGroup := range sdkConfig.ApiObjectConfig.ApiObjectGroups {
		for _, object := range objectGroup.Objects {
			definedInstance, definitionName, instanceName := IsOfDefinedInstance(
				*object.Name,
				objectGroup.Objects,
			)
			if !definedInstance {
				continue
			}
			switch {
			case strings.HasSuffix(*object.Name, "Definition"):
				for _, obj := range objectGroup.Objects {
					if *obj.Name == instanceName {
						if obj.DefinedInstance != nil && !*obj.DefinedInstance {
							return fmt.Errorf(
								"%s has 'DefinedInstance: false' but %s has 'DefinedInstance: true' (or is not set).  This will result in invalid API objects.  Both definition and instance must have the same value for 'DefinedInstance'.",
								instanceName,
								*object.Name,
							)
						}
					}
				}
			case strings.HasSuffix(*object.Name, "Instance"):
				for _, obj := range objectGroup.Objects {
					if *obj.Name == definitionName {
						if obj.DefinedInstance != nil && !*obj.DefinedInstance {
							return fmt.Errorf(
								"%s has 'DefinedInstance: false' but %s has 'DefinedInstance: true' (or is not set).  This will result in invalid API objects.  Both definition and instance must have the same value for 'DefinedInstance'.",
								definitionName,
								*object.Name,
							)
						}
					}
				}
			}
		}
	}

	return nil
}

// IsOfDefinedInstance takes the name of any API object and the API object
// group it belongs to and returns whether it is a part of a DefinedInstance
// abstraction.  If it is, it returns the definition object name and the
// instance object name.
func IsOfDefinedInstance(
	objectName string,
	objGroupObjects []*ApiObject,
) (bool, string, string) {
	// check if SDK config declares is not of a DefinedInstance abstraction
	for _, obj := range objGroupObjects {
		if objectName == *obj.Name {
			if obj.DefinedInstance != nil && !*obj.DefinedInstance {
				return false, "", ""
			}
		}
	}

	switch {
	case strings.HasSuffix(objectName, "Definition"):
		// check for corresponding instance object
		rootObjectName := strings.TrimSuffix(objectName, "Definition")
		instanceName := fmt.Sprintf("%sInstance", rootObjectName)
		instanceFound := false
		for _, obj := range objGroupObjects {
			if *obj.Name == instanceName {
				instanceFound = true
				break
			}
		}
		if instanceFound {
			return true, objectName, instanceName
		}
	case strings.HasSuffix(objectName, "Instance"):
		// check for corresponding definition object
		rootObjectName := strings.TrimSuffix(objectName, "Instance")
		definitionName := fmt.Sprintf("%sDefinition", rootObjectName)
		definitionFound := false
		for _, obj := range objGroupObjects {
			if *obj.Name == definitionName {
				definitionFound = true
				break
			}
		}
		if definitionFound {
			return true, definitionName, objectName
		}
	}

	return false, "", ""
}
