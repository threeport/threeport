package v0

import (
	"errors"
	"fmt"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/util"
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

	// The address for the threeport API.
	APIServer string `yaml:"APIServer"`

	// The threeport API's CA certificate.
	CACert string `yaml:"CACert"`

	// Kubernetes API and connection info.
	KubeAPI kube.KubeConnectionInfo `yaml:"KubeAPI"`

	// The infra provider hosting the threeport instance.
	Provider string `yaml:"Provider"`

	// Provider configuration for EKS-hosted threeport instances.
	EKSProviderConfig provider.ControlPlaneInfraEKS `yaml:"EKSProviderConfig"`

	// Client authentication credentials to threeport API.
	Credentials []Credential `yaml:"Credentials"`
}

// Credential is a client certificate and key pair for authenticating to a Threeport instance.
type Credential struct {
	Name       string `yaml:"Name"`
	ClientCert string `yaml:"ClientCert"`
	ClientKey  string `yaml:"ClientKey"`
}

// CheckThreeportConfigExists checks if a Threeport instance config exists.
func (cfg *ThreeportConfig) CheckThreeportConfigExists(createThreeportInstanceName string, forceOverwriteConfig bool) (bool, error) {
	// check threeport config for exisiting instance
	threeportInstanceConfigExists := false
	for _, instance := range cfg.Instances {
		if instance.Name == createThreeportInstanceName {
			threeportInstanceConfigExists = true
			if !forceOverwriteConfig {
				return threeportInstanceConfigExists, errors.New(fmt.Sprintf("instance of threeport with name %s already exists", instance.Name))
			}
		}
	}

	return threeportInstanceConfigExists, nil
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

// GetThreeportCertificates returns the CA certificate, client certificate, and client private key for the current instance.
func (cfg *ThreeportConfig) GetThreeportCertificates() (caCert, clientCert, clientPrivateKey string, err error) {
	for i, instance := range cfg.Instances {
		if instance.Name == cfg.CurrentInstance {
			caCert = cfg.Instances[i].CACert
		}
		for j, credential := range instance.Credentials {
			if credential.Name == cfg.CurrentInstance {
				clientCert = cfg.Instances[i].Credentials[j].ClientCert
				clientPrivateKey = cfg.Instances[i].Credentials[j].ClientKey

				caCert, err := util.Base64Decode(caCert)
				if err != nil {
					return "", "", "", fmt.Errorf("failed to decode CA certificate: %w", err)
				}

				clientCert, err := util.Base64Decode(clientCert)
				if err != nil {
					return "", "", "", fmt.Errorf("failed to decode client certificate: %w", err)
				}

				clientPrivateKey, err := util.Base64Decode(clientPrivateKey)
				if err != nil {
					return "", "", "", fmt.Errorf("failed to decode client private key: %w", err)
				}

				return caCert, clientCert, clientPrivateKey, nil
			}
		}
	}

	return "", "", "", errors.New("could not load credentials")
}
