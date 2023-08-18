package v0

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"github.com/threeport/threeport/internal/util"
)

const (
	ThreeportConfigName = "config"
	ThreeportConfigType = "yaml"
)

// ThreeportConfig is the client's configuration for connecting to Threeport instances
type ThreeportConfig struct {
	// All the threeport instances a user has available to use.
	Instances []Instance `yaml:"Instances"`

	// The name of the threeport instance currently in use.
	CurrentInstance string `yaml:"CurrentInstance"`
}

// ThreeportInstance is an instance of Threeport the client can use.
type Instance struct {
	// The unique name of the threeport instance.
	Name string `yaml:"Name"`

	// If true client certificate authentication is used.
	AuthEnabled bool `yaml:"AuthEnabled"`

	// The address for the threeport API.
	APIServer string `yaml:"APIServer"`

	// The threeport API's CA certificate.
	CACert string `yaml:"CACert"`

	// Kubernetes API and connection info.
	KubeAPI KubeAPI `yaml:"KubeAPI"`

	// The infra provider hosting the threeport instance.
	Provider string `yaml:"Provider"`

	// Provider configuration for EKS-hosted threeport instances.
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
	AwsConfigEnv     bool   `yaml:"AWSConfigEnv"`
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

// GetAllInstanceNames returns all instance names in a threeport config.
func (cfg *ThreeportConfig) GetAllInstanceNames() []string {
	var allInstances []string
	for _, instance := range cfg.Instances {
		allInstances = append(allInstances, instance.Name)
	}

	return allInstances
}

// CheckThreeportConfigExists checks if a Threeport instance config exists.
func (cfg *ThreeportConfig) CheckThreeportConfigExists(createThreeportInstanceName string) bool {
	threeportInstanceConfigExists := false
	for _, instance := range cfg.Instances {
		if instance.Name == createThreeportInstanceName {
			threeportInstanceConfigExists = true
			break
		}
	}

	return threeportInstanceConfigExists
}

// GetThreeportAPIEndpoint returns the threeport API endpoint from threeport
// config.
func (cfg *ThreeportConfig) GetThreeportAPIEndpoint() (string, error) {
	for i, instance := range cfg.Instances {
		if instance.Name == cfg.CurrentInstance {
			return cfg.Instances[i].APIServer, nil
		}
	}

	return "", errors.New("current instance not found when retrieving threeport API endpoint")
}

// GetThreeportAuthEnabled returns a boolean that indicates whether current
// instance has auth enabled.
func (cfg *ThreeportConfig) GetThreeportAuthEnabled() (bool, error) {
	for i, instance := range cfg.Instances {
		if instance.Name == cfg.CurrentInstance {
			return cfg.Instances[i].AuthEnabled, nil
		}
	}

	return false, errors.New("current instance not found when retrieving threeport API endpoint")
}

// GetEncryptionKey returns the encryption key from the threeport
// config.
func (cfg *ThreeportConfig) GetEncryptionKey() (string, error) {
	for i, instance := range cfg.Instances {
		if instance.Name == cfg.CurrentInstance {
			return cfg.Instances[i].EncryptionKey, nil
		}
	}

	return "", errors.New("current instance not found when retrieving threeport API endpoint")
}

// GetThreeportCertificates returns the CA certificate, client certificate, and
// client private key for a named threeport instance.
func (cfg *ThreeportConfig) GetThreeportCertificatesForInstance(instanceName string) (string, string, string, error) {
	// find instance
	var instance Instance
	instanceFound := false
	for _, inst := range cfg.Instances {
		if inst.Name == instanceName {
			instance = inst
			instanceFound = true
			break
		}
	}
	if !instanceFound {
		return "", "", "", errors.New(
			fmt.Sprintf("could not find threeport instance name %s in threeport config", instanceName),
		)
	}

	// fetch certs from instance config
	caCert, err := util.Base64Decode(instance.CACert)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decode CA certificate: %w", err)
	}
	var clientCert string
	var clientPrivateKey string
	credsFound := false
	for _, credential := range instance.Credentials {
		if credential.Name == instanceName {
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

// GetThreeportCertificates returns the CA certificate, client certificate, and
// client private key for the current instance.
func (cfg *ThreeportConfig) GetThreeportCertificates() (caCert, clientCert, clientPrivateKey string, err error) {
	if cfg.CurrentInstance == "" {
		return "", "", "", errors.New("current instance not set - set it with 'tptctl config current-instance -n [instance name]'")
	}
	return cfg.GetThreeportCertificatesForInstance(cfg.CurrentInstance)
}

// SetCurrentInstance updates the threeport config to set CurrentInstance as the
// provided instance name.
func (cfg *ThreeportConfig) SetCurrentInstance(instanceName string) {
	viper.Set("CurrentInstance", instanceName)
	viper.WriteConfig()
}

// GetThreeportConfig retrieves the threeport config
func GetThreeportConfig() (*ThreeportConfig, error) {
	threeportConfig := &ThreeportConfig{}
	if err := viper.Unmarshal(threeportConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return threeportConfig, nil
}

// UpdateThreeportConfig updates a threeport config to add or update a config
// for a threeport instance and set it as the current instance.
func UpdateThreeportConfig(
	threeportConfig *ThreeportConfig,
	threeportInstanceConfig *Instance,
) {
	if threeportConfig.CheckThreeportConfigExists(threeportInstanceConfig.Name) {
		for n, instance := range threeportConfig.Instances {
			if instance.Name == threeportInstanceConfig.Name {
				threeportConfig.Instances[n] = *threeportInstanceConfig
			}
		}
	} else {
		threeportConfig.Instances = append(threeportConfig.Instances, *threeportInstanceConfig)
	}
	viper.Set("Instances", threeportConfig.Instances)
	viper.Set("CurrentInstance", threeportInstanceConfig.Name)
	viper.WriteConfig()
}

// DeleteThreeportConfigInstance updates a threeport config to remove a deleted
// threeport instance and the current instance.
func DeleteThreeportConfigInstance(threeportConfig *ThreeportConfig, deleteThreeportInstanceName string) {
	updatedInstances := []Instance{}
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
