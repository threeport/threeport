package v0

import "errors"

// ThreeportConfig is the client's configuration for connecting to Threeport instances
type ThreeportConfig struct {
	Instances       []Instance `yaml:"Instances"`
	CurrentInstance string     `yaml:"CurrentInstance"`
}

// ThreeportInstance is an instance of Threeport the client can use
type Instance struct {
	Name        string       `yaml:"Name"`
	Provider    string       `yaml:"Provider"`
	APIServer   string       `yaml:"APIServer"`
	CACert      string       `yaml:"CACert"`
	Kubeconfig  string       `yaml: "Kubeconfig"`
	Credentials []Credential `yaml:"Credentials"`
}

type Credential struct {
	Name       string `yaml:"Name"`
	ClientCert string `yaml:"ClientCert"`
	ClientKey  string `yaml:"ClientKey"`
}

func (cfg *ThreeportConfig) GetThreeportAPIEndpoint() (string, error) {
	for i, instance := range cfg.Instances {
		if instance.Name == cfg.CurrentInstance {
			return cfg.Instances[i].APIServer, nil
		}
	}

	return "", errors.New("current instance not found when retrieving threeport API endpoint")
}
