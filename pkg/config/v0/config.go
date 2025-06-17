package v0

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/mitchellh/go-homedir"
	builder_config "github.com/nukleros/aws-builder/pkg/config"
	"github.com/spf13/viper"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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

	// Provider configuration for OKE-hosted threeport control planes.
	OKEProviderConfig OKEProviderConfig `yaml:"OKEProviderConfig"`

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
	Token         string `yaml:"Token"`
}

// EKSProviderConfig is the set of provider config information needed to manage
// EKS clusters on AWs.
type EKSProviderConfig struct {
	AwsConfigProfile string `yaml:"AWSConfigProfile"`
	AwsRegion        string `yaml:"AWSRegion"`
	AwsAccountID     string `yaml:"AWSAccountID"`
}

// OKEProviderConfig is the set of provider config information needed to manage
// OKE clusters on OCI.
type OKEProviderConfig struct {
	OciRegion          string `yaml:"OciRegion"`
	OciConfigProfile   string `yaml:"OciConfigProfile"`
	OciCompartmentOcid string `yaml:"OciCompartmentOcid"`
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
	_, err := cfg.GetControlPlaneConfig(createThreeportControlPlaneName)
	return err == nil
}

// GetThreeportAPIEndpoint returns the threeport API endpoint from threeport
// config.
func (cfg *ThreeportConfig) GetThreeportAPIEndpoint(requestedControlPlane string) (string, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return "", errors.New("current control plane not found when retrieving threeport API endpoint")
	}
	return controlPlane.APIServer, nil
}

// GetThreeportAuthEnabled returns a boolean that indicates whether current
// control plane has auth enabled.
func (cfg *ThreeportConfig) GetThreeportAuthEnabled(requestedControlPlane string) (bool, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return false, errors.New("current control plane not found when retrieving threeport API endpoint")
	}

	return controlPlane.AuthEnabled, nil
}

// GetThreeportEncryptionKey returns the encryption key that is used encrypt
// sensitive values in the Threeport database.
func (cfg *ThreeportConfig) GetThreeportEncryptionKey(requestedControlPlane string) (string, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return "", errors.New("current control plane not found when retrieving threeport API endpoint")
	}

	return controlPlane.EncryptionKey, nil
}

// GetThreeportInfraProvider returns the infra provider from
// the threeport config.
func (cfg *ThreeportConfig) GetThreeportInfraProvider(requestedControlPlane string) (string, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return "", errors.New("current control plane not found when retrieving threeport infra provider")
	}
	return controlPlane.Provider, nil
}

// CheckThreeportGenesisControlPlane returns a boolean that indicates whether current
// control plane is the genesis control plane.
func (cfg *ThreeportConfig) CheckThreeportGenesisControlPlane(requestedControlPlane string) (bool, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return false, errors.New("current control plane not found when checking for genesis info")
	}
	return controlPlane.Genesis, nil
}

// GetEncryptionKey returns the encryption key from the threeport
// config.
func (cfg *ThreeportConfig) GetEncryptionKey(requestedControlPlane string) (string, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return "", errors.New("current instance not found when retrieving encryption key")
	}
	return controlPlane.EncryptionKey, nil
}

// GetThreeportCertificatesForControlPlane returns the CA certificate, client
// certificate, and client private key for a named threeport control plane.
func (cfg *ThreeportConfig) GetThreeportCertificatesForControlPlane(requestedControlPlane string) (string, string, string, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return "", "", "", errors.New("current control plane not found when retrieving threeport certificates")
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
		if credential.Name == requestedControlPlane {
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

// GetThreeportHTTPClient returns an HTTP client for a named threeport instance.
func (cfg *ThreeportConfig) GetHTTPClient(requestedControlPlane string) (*http.Client, error) {
	authEnabled, err := cfg.GetThreeportAuthEnabled(requestedControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth enabled: %w", err)
	}

	ca, clientCertificate, clientPrivateKey, err := cfg.GetThreeportCertificatesForControlPlane(requestedControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to get threeport certificates: %w", err)
	}

	apiClient, err := client_lib.GetHTTPClient(authEnabled, ca, clientCertificate, clientPrivateKey, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get http client: %w", err)
	}

	return apiClient, nil
}

// GetControlPlaneInstance returns the current control plane instance.
func (cfg *ThreeportConfig) GetControlPlaneInstance(requestedControlPlane string) (*v0.ControlPlaneInstance, error) {

	// get threeport API endpoint
	apiEndpoint, err := cfg.GetThreeportAPIEndpoint(requestedControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to get threeport API endpoint from config: %w", err)
	}

	// get threeport API client
	apiClient, err := cfg.GetHTTPClient(requestedControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to get threeport API client: %w", err)
	}

	for _, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == requestedControlPlane {
			if controlPlaneInstance, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, controlPlane.Name); err != nil {
				return nil, fmt.Errorf("failed to retrieve current control plane instance: %w", err)
			} else {
				return controlPlaneInstance, nil
			}
		}
	}
	return nil, fmt.Errorf("failed to retrieve current control plane instance")
}

// GetControlPlaneConfig returns the requested control plane config.
func (cfg *ThreeportConfig) GetControlPlaneConfig(name string) (*ControlPlane, error) {
	for _, controlPlane := range cfg.ControlPlanes {
		if controlPlane.Name == name {
			return &controlPlane, nil
		}
	}
	return nil, fmt.Errorf("control plane %s not found", name)
}

// GetAwsConfigs returns AWS configs for the user and resource manager.
func (cfg *ThreeportConfig) GetAwsConfigs(requestedControlPlane string) (*aws.Config, *aws.Config, string, error) {
	controlPlane, err := cfg.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get control plane config: %w", err)
	}
	awsConfigUser, err := builder_config.LoadAWSConfig(
		false,
		controlPlane.EKSProviderConfig.AwsConfigProfile,
		controlPlane.EKSProviderConfig.AwsRegion,
		"",
		"",
		"",
	)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to load AWS configuration with local config: %w", err)
	}

	// test credentials for awsConfigUser
	var callerIdentity *sts.GetCallerIdentityOutput
	if callerIdentity, err = provider.GetCallerIdentity(awsConfigUser); err != nil {
		return nil, nil, "", fmt.Errorf("failed to get caller identity: %w", err)
	}
	fmt.Printf("Successfully authenticated to account %s as %s\n", *callerIdentity.Account, *callerIdentity.Arn)

	// assume role for AWS resource manager for infra teardown
	awsConfigResourceManager, err := builder_config.AssumeRole(
		provider.GetResourceManagerRoleArn(
			controlPlane.Name,
			controlPlane.EKSProviderConfig.AwsAccountID,
		),
		"",
		"",
		3600,
		*awsConfigUser,
		[]func(*aws_config.LoadOptions) error{
			aws_config.WithRegion(controlPlane.EKSProviderConfig.AwsRegion),
		},
	)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to assume role for AWS resource manager: %w", err)
	}

	// test credentials for awsConfigResourceManager
	if callerIdentity, err = provider.GetCallerIdentity(awsConfigResourceManager); err != nil {
		return nil, nil, "", fmt.Errorf("failed to get caller identity: %w", err)
	}
	fmt.Printf("Successfully authenticated to account %s as %s\n", *callerIdentity.Account, *callerIdentity.Arn)

	return awsConfigUser, awsConfigResourceManager, *callerIdentity.Account, nil
}

// SetCurrentInstance updates the threeport config to set CurrentInstance as the
// provided instance name.
func (cfg *ThreeportConfig) SetCurrentInstance(instanceName string) {
	viper.Set("CurrentInstance", instanceName)
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

// UpdateThreeportConfigInstance updates a threeport instance config
// and returns the updated threeport config.
func (c *ControlPlane) UpdateThreeportConfigInstance(f func(*ControlPlane)) (*ThreeportConfig, error) {

	// make requested changes to threeport instance config
	f(c)

	// pull latest threeport config from disk
	threeportConfig, _, err := GetThreeportConfig("")
	if err != nil {
		return nil, fmt.Errorf("failed to get threeport config: %w", err)
	}

	// sync threeport instance config changes to disk
	UpdateThreeportConfig(threeportConfig, c)

	return threeportConfig, nil
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
	return filepath.Join(homedir, ".threeport")
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

// DefaultPluginDir returns the default directory for tptctl plugin installation.
func DefaultPluginDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home directory: %w", err)
	}

	return filepath.Join(home, ".threeport", "plugins"), nil
}
