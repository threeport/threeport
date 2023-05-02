package v0

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/util"
)

const (
	configName = "config"
	configType = "yaml"
)

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

func (cfg *ThreeportConfig) GetThreeportCertificates() (caCert, clientCert, clientPrivateKey string, err error) {
	for i, instance := range cfg.Instances {
		if instance.Name == cfg.CurrentInstance {
			caCert = cfg.Instances[i].CACert
		}
		for j, credential := range instance.Credentials {
			if credential.Name == cfg.CurrentInstance {
				clientCert = cfg.Instances[i].Credentials[j].ClientCert
				clientPrivateKey = cfg.Instances[i].Credentials[j].ClientKey
			}
		}
		return util.Base64Decode(caCert), util.Base64Decode(clientCert), util.Base64Decode(clientPrivateKey), nil
	}

	return "", "", "", errors.New("could not load credentials")
}

func InitConfig(cfgFile, providerConfigDir string) {
	// determine user home dir
	home, err := homedir.Dir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	viper.AddConfigPath(configPath(home))
	viper.SetConfigName(configName)
	viper.SetConfigType(configType)
	//configFilePath := fmt.Sprintf("%s/%s.%s", configPath(home), configName, configType)
	configFilePath := filepath.Join(configPath(home), fmt.Sprintf("%s.%s", configName, configType))

	// read config file if provided, else go to default
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		//viper.AddConfigPath(configPath(home))
		//viper.SetConfigName(configName)
		//viper.SetConfigType(configType)

		// create config if not present
		//configFilePath := fmt.Sprintf("%s/%s.%s", configPath(home), configName, configType)
		if err := viper.SafeWriteConfigAs(configFilePath); err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(configPath(home), os.ModePerm); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := viper.WriteConfigAs(configFilePath); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			}
		}
	}

	if providerConfigDir == "" {
		if err := os.MkdirAll(configPath(home), os.ModePerm); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		providerConfigDir = configPath(home)
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Can't read config:", err)
		os.Exit(1)
	}
}

func configPath(homedir string) string {
	//return fmt.Sprintf("%s/.config/threeport", homedir)
	return filepath.Join(homedir, ".config", "threeport")
}

func GetThreeportConfig() *ThreeportConfig {
	// get threeport config
	threeportConfig := &ThreeportConfig{}
	if err := viper.Unmarshal(threeportConfig); err != nil {
		cli.Error("failed to get threeport config", err)
		os.Exit(1)
	}

	return threeportConfig
}

func CheckThreeportConfigExists(threeportConfig *ThreeportConfig, createThreeportInstanceName string, forceOverwriteConfig bool) bool {
	// check threeport config for exisiting instance
	threeportInstanceConfigExists := false
	for _, instance := range threeportConfig.Instances {
		if instance.Name == createThreeportInstanceName {
			threeportInstanceConfigExists = true
			if !forceOverwriteConfig {
				cli.Error(
					"interupted creation of threeport instance",
					errors.New(fmt.Sprintf("instance of threeport with name %s already exists", instance.Name)),
				)
				cli.Info("if you wish to overwrite the existing config use --force-overwrite-config flag")
				cli.Warning("you will lose the ability to connect to the existing threeport instance if it still exists")
				os.Exit(1)
			}
		}
	}

	return threeportInstanceConfigExists
}

func UpdateThreeportConfig(threeportInstanceConfigExists bool, threeportConfig *ThreeportConfig, createThreeportInstanceName string, newThreeportInstance *Instance) {

	// update threeport config to add the new instance and set as current instance
	if threeportInstanceConfigExists {
		for n, instance := range threeportConfig.Instances {
			if instance.Name == createThreeportInstanceName {
				threeportConfig.Instances[n] = *newThreeportInstance
			}
		}
	} else {
		threeportConfig.Instances = append(threeportConfig.Instances, *newThreeportInstance)
	}
	viper.Set("Instances", threeportConfig.Instances)
	viper.Set("CurrentInstance", createThreeportInstanceName)
	viper.WriteConfig()
}

// loads certificates from ~/.threeport or /etc/threeport
func GetHTTPClient(authEnabled bool) (*http.Client, error) {

	if !authEnabled {
		return &http.Client{}, nil
	}

	homeDir, _ := os.UserHomeDir()
	var tlsConfig *tls.Config

	_, errConfigDirectory := os.Stat(filepath.Join(homeDir, ".config/threeport"))
	_, errThreeportCert := os.Stat("/etc/threeport/cert")
	_, errThreeportCA := os.Stat("/etc/threeport/ca")
	// var caFile []byte
	var caCert string
	var cert tls.Certificate

	if errConfigDirectory == nil {

		threeportConfig := GetThreeportConfig()
		// config.InitConfig("", "")
		var err error
		var clientCertificate string
		var clientPrivateKey string
		caCert, clientCertificate, clientPrivateKey, err = threeportConfig.GetThreeportCertificates()
		certFile := []byte(clientCertificate)
		keyFile := []byte(clientPrivateKey)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load client certificate and private key
		cert, err = tls.X509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}

		// load root certificate authority
		// caCert, err = ioutil.ReadFile(caFile)
		if err != nil {
			return nil, err
		}

		// // create certificate pool and add certificate authority
		// caCertPool := x509.NewCertPool()
		// caCertPool.AppendCertsFromPEM(caFile)

		// // create tls config required by http client
		// tlsConfig = &tls.Config{
		// 	Certificates: []tls.Certificate{cert},
		// 	RootCAs:      caCertPool,
		// }

	} else if errThreeportCert == nil && errThreeportCA == nil {
		// Use certificates from /etc/threeport directory
		certFile := "/etc/threeport/cert/tls.crt"
		keyFile := "/etc/threeport/cert/tls.key"
		caFilePath := "/etc/threeport/ca/tls.crt"

		// load client certificate and private key
		var err error
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}

		// load root certificate authority
		caCertBytes, err := ioutil.ReadFile(caFilePath)
		if err != nil {
			return nil, err
		}

		caCert = string(caCertBytes)
	} else {
		return nil, errors.New("could not find certificate files")
	}

	// create certificate pool and add certificate authority
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(caCert))

	// create tls config required by http client
	tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	apiClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return apiClient, nil
}
