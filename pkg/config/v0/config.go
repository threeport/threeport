package v0

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

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

type AuthConfig struct {
	CAConfig                  *x509.Certificate
	CAPrivateKey              rsa.PrivateKey
	CA                        []byte
	CAPemEncoded              string
	CABase64Encoded           string
	CAPrivateKeyPemEncoded    string
	CAPrivateKeyBase64Encoded string
}

func GetAuthConfig() *AuthConfig {
	// generate certificate authority for the threeport API
	caConfig, ca, caPrivateKey, err := GenerateCACertificate()
	if err != nil {
		cli.Error("failed to generate certificate authority and private key", err)
		os.Exit(1)
	}

	// get PEM-encoded keypairs as strings to pass into deployment manifests
	caEncoded := GetPEMEncoding(ca, "CERTIFICATE")
	caPrivateKeyEncoded := GetPEMEncoding(x509.MarshalPKCS1PrivateKey(caPrivateKey), "RSA PRIVATE KEY")

	return &AuthConfig{
		CAConfig:                  caConfig,
		CAPrivateKey:              *caPrivateKey,
		CA:                        ca,
		CAPemEncoded:              caEncoded,
		CABase64Encoded:           util.Base64Encode(caEncoded),
		CAPrivateKeyPemEncoded:    caPrivateKeyEncoded,
		CAPrivateKeyBase64Encoded: util.Base64Encode(caPrivateKeyEncoded),
	}

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

func GenerateCACertificate() (caConfig *x509.Certificate, ca []byte, caPrivateKey *rsa.PrivateKey, err error) {

	// generate a random identifier for use as a serial number
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	randomNumber, err := rand.Int(rand.Reader, max)
	if err != nil {
		fmt.Errorf("failed to generate random serial number: %w", err)
		return nil, nil, nil, err
	}

	// set config options for a new CA certificate
	caConfig = &x509.Certificate{
		SerialNumber: randomNumber,
		URIs:         []*url.URL{{Scheme: "https", Host: "localhost"}},
		DNSNames: []string{
			"localhost",
			"threeport-api-server",
			"threeport-api-server.threeport-control-plane",
			"threeport-api-server.threeport-control-plane.svc",
			"threeport-api-server.threeport-control-plane.svc.cluster",
			"threeport-api-server.threeport-control-plane.svc.cluster.local",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Threeport"},
			Country:      []string{"US"},
			Locality:     []string{"Tampa"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// generate private and public keys for the CA
	caPrivateKey, err = rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		fmt.Errorf("failed to generate CA private key: %w", err)
		return nil, nil, nil, err
	}

	// generate a certificate authority
	ca, err = x509.CreateCertificate(rand.Reader, caConfig, caConfig, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		fmt.Errorf("failed to create CA certificate: %w", err)
		return nil, nil, nil, err
	}

	return caConfig, ca, caPrivateKey, nil

}

func GenerateCertificate(caConfig *x509.Certificate, caPrivateKey *rsa.PrivateKey) (certificate string, privateKey string, err error) {

	// generate a random identifier for use as a serial number
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	randomNumber, err := rand.Int(rand.Reader, max)
	if err != nil {
		fmt.Errorf("failed to generate random serial number: %w", err)
		return "", "", err
	}

	// set config options for a new CA certificate
	cert := &x509.Certificate{
		SerialNumber: randomNumber,
		URIs:         []*url.URL{{Scheme: "https", Host: "localhost"}},
		DNSNames: []string{
			"localhost",
			"threeport-api-server",
			"threeport-api-server.threeport-control-plane",
			"threeport-api-server.threeport-control-plane.svc",
			"threeport-api-server.threeport-control-plane.svc.cluster",
			"threeport-api-server.threeport-control-plane.svc.cluster.local",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Threeport"},
			Country:      []string{"US"},
			Locality:     []string{"Tampa"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  false,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// generate private and public keys for the CA
	serverPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		fmt.Errorf("failed to generate CA private key: %w", err)
		return "", "", err
	}

	// generate a certificate authority
	serverCert, err := x509.CreateCertificate(rand.Reader, cert, caConfig, &serverPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		fmt.Errorf("failed to create CA certificate: %w", err)
		return "", "", err
	}

	serverCertificateEncoded := GetPEMEncoding(serverCert, "CERTIFICATE")
	serverPrivateKeyEncoded := GetPEMEncoding(x509.MarshalPKCS1PrivateKey(serverPrivateKey), "RSA PRIVATE KEY")
	return serverCertificateEncoded, serverPrivateKeyEncoded, nil
}

func GetPEMEncoding(cert []byte, encodingType string) (pemEncodingString string) {
	pemEncoding := new(bytes.Buffer)
	pem.Encode(pemEncoding, &pem.Block{
		Type:  encodingType,
		Bytes: cert,
	})

	return pemEncoding.String()
}
