package v0

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	ThreeportConfigName = "config"
	ThreeportConfigType = "yaml"
)

// ThreeportConfig is the client's configuration for connecting to Threeport instances
type ThreeportConfig struct {
	// All the threeport instances a user has available to use.
	ControlPlanes []ControlPlane `yaml:"ControlPlanes"`

	// The name of the control plane currently in use.
	CurrentControlPlane string `yaml:"CurrentControlPlane"`
}

// Control plane is an instance of Threeport control plane the client can use.
type ControlPlane struct {
	// The unique name of the threeport control plane.
	Name string `yaml:"Name"`

	// If true client certificate authentication is used.
	AuthEnabled bool `yaml:"AuthEnabled"`

	// True used to indicate that the control plane was the first in the control plane group
	Genesis bool `yaml:"Genesis"`

	// The address for the threeport API.
	APIServer string `yaml:"APIServer"`

	// The threeport API's CA certificate.
	CACert string `yaml:"CACert"`

	// Kubernetes API and connection info.
	KubeAPI KubeAPI `yaml:"KubeAPI"`

	// The infra provider hosting the threeport control plane.
	Provider string `yaml:"Provider"`

	// Provider configuration for EKS-hosted threeport control planes.
	EKSProviderConfig EKSProviderConfig `yaml:"EKSProviderConfig"`

	// Client authentication credentials to threeport API.
	Credentials []Credential `yaml:"Credentials"`

	// The encryption key used to encrypt secrets.
	EncryptionKey string `yaml:"EncryptionKey"`
}

// KubeAPI is the information and credentials needed to connect to the
// Kubernetes API hosting the threeport control plane.
type KubeAPI struct {
	APIEndpoint   string `yaml:"APIEndpoint"`
	CACertificate string `yaml:"CACertificate"`
	Certificate   string `yaml:"Certificate"`
	Key           string `yaml:"Key"`
	EKSToken      string `yaml:"EKSToken"`
}

// EKSProviderConfig is the set of provider config information needed to manage
// EKS clusters on AWs.
type EKSProviderConfig struct {
	AwsConfigProfile string `yaml:"AWSConfigProfile"`
	AwsRegion        string `yaml:"AWSRegion"`
	AwsAccountID     string `yaml:"AWSAccountID"`
}

// Credential is a client certificate and key pair for authenticating to a Threeport instance.
type Credential struct {
	Name       string `yaml:"Name"`
	ClientCert string `yaml:"ClientCert"`
	ClientKey  string `yaml:"ClientKey"`
	Token      string ``
}

// GetAllControlPlaneNames returns all control plane names in a threeport config.
func (cfg *ThreeportConfig) GetAllControlPlaneNames() []string {
	var allControlPlanes []string
	for _, controlPlane := range cfg.ControlPlanes {
		allControlPlanes = append(allControlPlanes, controlPlane.Name)
	}

	return allControlPlanes
}

// CheckThreeportControlPlaneExists checks if a Threeport control plane within a config already contains
// control plane information
func (cfg *ThreeportConfig) CheckThreeportConfigEmpty() bool {
	return len(cfg.ControlPlanes) == 0
}

// CheckThreeportControlPlaneExists checks if a Threeport control plane within a config exists.
func (cfg *ThreeportConfig) CheckThreeportControlPlaneExists(createThreeportControlPlaneName string) bool {
	threeportControlPlaneExists := false
	for _, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == createThreeportControlPlaneName {
			threeportControlPlaneExists = true
			break
		}
	}

	return threeportControlPlaneExists
}

// GetThreeportAPIEndpoint returns the threeport API endpoint from threeport
// config.
func (cfg *ThreeportConfig) GetThreeportAPIEndpoint(requestedControlPlane string) (string, error) {
	for i, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == requestedControlPlane {
			return cfg.ControlPlanes[i].APIServer, nil
		}
	}

	return "", errors.New("current control plane not found when retrieving threeport API endpoint")
}

// GetThreeportAuthEnabled returns a boolean that indicates whether current
// control plane has auth enabled.
func (cfg *ThreeportConfig) GetThreeportAuthEnabled(requestedControlPlane string) (bool, error) {
	for i, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == requestedControlPlane {
			return cfg.ControlPlanes[i].AuthEnabled, nil
		}
	}

	return false, errors.New("current control plane not found when retrieving threeport API endpoint")
}

// CheckThreeportGenesisControlPlane returns a boolean that indicates whether current
// control plane is the genesis control plane.
func (cfg *ThreeportConfig) CheckThreeportGenesisControlPlane(requestedControlPlane string) (bool, error) {
	for i, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == requestedControlPlane {
			return cfg.ControlPlanes[i].Genesis, nil
		}
	}

	return false, errors.New("current control plane not found when checking for genesis info")
}

// GetEncryptionKey returns the encryption key from the threeport
// config.
func (cfg *ThreeportConfig) GetEncryptionKey(requestedControlPlane string) (string, error) {
	for i, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == requestedControlPlane {
			return cfg.ControlPlanes[i].EncryptionKey, nil
		}
	}

	return "", errors.New("current instance not found when retrieving encryption key")
}

// GetThreeportCertificatesForControlPlane returns the CA certificate, client
// certificate, and client private key for a named threeport control plane.
func (cfg *ThreeportConfig) GetThreeportCertificatesForControlPlane(controlPlaneName string) (string, string, string, error) {
	// find controlPlane
	var controlPlane ControlPlane
	controlPlaneFound := false
	for _, inst := range cfg.ControlPlanes {
		if inst.Name == controlPlaneName {
			controlPlane = inst
			controlPlaneFound = true
			break
		}
	}
	if !controlPlaneFound {
		return "", "", "", errors.New(
			fmt.Sprintf("could not find threeport control plane name %s in threeport config", controlPlaneName),
		)
	}

	// fetch certs from instance config
	caCert, err := util.Base64Decode(controlPlane.CACert)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decode CA certificate: %w", err)
	}
	var clientCert string
	var clientPrivateKey string
	credsFound := false
	for _, credential := range controlPlane.Credentials {
		if credential.Name == controlPlaneName {
			cert, err := util.Base64Decode(credential.ClientCert)
			if err != nil {
				return "", "", "", fmt.Errorf("failed to decode client certificate: %w", err)
			}
			key, err := util.Base64Decode(credential.ClientKey)
			if err != nil {
				return "", "", "", fmt.Errorf("failed to decode client private key: %w", err)
			}
			clientCert = cert
			clientPrivateKey = key
			credsFound = true
			break
		}
	}
	if !credsFound {
		// for clusters with auth disabled return empty values
		return "", "", "", nil
	}

	return caCert, clientCert, clientPrivateKey, nil
}

// SetCurrentControlPlane updates the threeport config to set CurrentControlPlane as the
// provided control plane name.
func (cfg *ThreeportConfig) SetCurrentControlPlane(controlPlaneName string) {
	viper.Set("CurrentControlPlane", controlPlaneName)
	viper.WriteConfig()
}

// GetThreeportConfig retrieves the threeport config and name of the
// requested control plane.
func GetThreeportConfig(requestedControlPlane string) (*ThreeportConfig, string, error) {
	threeportConfig := &ThreeportConfig{}
	if err := viper.Unmarshal(threeportConfig); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal config: %w", err)
	}

	controlPlaneName := threeportConfig.CurrentControlPlane
	if requestedControlPlane != "" {
		controlPlaneName = requestedControlPlane
	}

	return threeportConfig, controlPlaneName, nil
}

// UpdateThreeportConfig updates a threeport config to add or update a config
// for a threeport control plane and set it as the current control plane.
func UpdateThreeportConfig(
	threeportConfig *ThreeportConfig,
	threeportControlPlaneConfig *ControlPlane,
) {
	if threeportConfig.CheckThreeportControlPlaneExists(threeportControlPlaneConfig.Name) {
		for n, instance := range threeportConfig.ControlPlanes {
			if instance.Name == threeportControlPlaneConfig.Name {
				threeportConfig.ControlPlanes[n] = *threeportControlPlaneConfig
			}
		}
	} else {
		threeportConfig.ControlPlanes = append(threeportConfig.ControlPlanes, *threeportControlPlaneConfig)
	}
	viper.Set("ControlPlanes", threeportConfig.ControlPlanes)
	viper.Set("CurrentControlPlane", threeportControlPlaneConfig.Name)
	viper.WriteConfig()
}

// DeleteThreeportConfigControlPlane updates a threeport config to remove a deleted
// threeport control plane and the current control plane.
func DeleteThreeportConfigControlPlane(threeportConfig *ThreeportConfig, deleteThreeportControlPlaneName string) {
	updatedControlPlanes := []ControlPlane{}
	for _, controlPlane := range threeportConfig.ControlPlanes {
		if controlPlane.Name == deleteThreeportControlPlaneName {
			continue
		} else {
			updatedControlPlanes = append(updatedControlPlanes, controlPlane)
		}
	}

	viper.Set("ControlPlanes", updatedControlPlanes)
	viper.Set("CurrentControlPlane", "")
	viper.WriteConfig()
}

// DefaultThreeportConfigPath returns the default path to the threeport config
// file on the user's filesystem.
func DefaultThreeportConfigPath(homedir string) string {
	return filepath.Join(homedir, ".config", "threeport")
}

// DefaultProviderConfigDir returns the default path to the directory for storing
// infra provider inventory and config if not set which is ~/.config/threeport.
func DefaultProviderConfigDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home directory: %w", err)
	}

	if err := os.MkdirAll(DefaultThreeportConfigPath(home), os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to write create config directory: %w", err)
	}

	return DefaultThreeportConfigPath(home), nil
}
