package v0

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/util"
)

// ThreeportConfig is the client's configuration for connecting to Threeport instances
type ThreeportConfig struct {
	Instances       []Instance `yaml:"Instances"`
	CurrentInstance string     `yaml:"CurrentInstance"`
}

// ThreeportInstance is an instance of Threeport the client can use.
type Instance struct {
	Name        string       `yaml:"Name"`
	Provider    string       `yaml:"Provider"`
	APIServer   string       `yaml:"APIServer"`
	CACert      string       `yaml:"CACert"`
	Kubeconfig  string       `yaml: "Kubeconfig"`
	Credentials []Credential `yaml:"Credentials"`
}

// Credential is a client certificate and key pair for authenticating to a Threeport instance.
type Credential struct {
	Name       string `yaml:"Name"`
	ClientCert string `yaml:"ClientCert"`
	ClientKey  string `yaml:"ClientKey"`
}

// AuthConfig contains root CA and private key for generating client certificates.
type AuthConfig struct {
	CAConfig                  *x509.Certificate
	CAPrivateKey              rsa.PrivateKey
	CA                        []byte
	CAPemEncoded              string
	CABase64Encoded           string
	CAPrivateKeyPemEncoded    string
	CAPrivateKeyBase64Encoded string
}

// GetAuthConfig populates an AuthConfig object and returns a pointer to it.
func GetAuthConfig() *AuthConfig {
	// generate certificate authority for the threeport API
	caConfig, ca, caPrivateKey, err := GenerateCACertificate()
	if err != nil {
		cli.Error("failed to generate certificate authority and private key", err)
		os.Exit(1)
	}

	// get PEM-encoded keypairs as strings to pass into deployment manifests
	caEncoded := GetCertificatePEMEncoding(ca)
	caPrivateKeyEncoded := GetPrivateKeyPEMEncoding(caPrivateKey)

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

// GetThreeportAPIEndpoint returns the API endpoint for the current instance.
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

func (cfg *ThreeportConfig) CheckThreeportConfigExists(createThreeportInstanceName string, forceOverwriteConfig bool) bool {
	// check threeport config for exisiting instance
	threeportInstanceConfigExists := false
	for _, instance := range cfg.Instances {
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

// GenerateCertificate generates a certificate and private key for the current Threeport instance.
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

	serverCertificateEncoded := GetCertificatePEMEncoding(serverCert)
	serverPrivateKeyEncoded := GetPrivateKeyPEMEncoding(serverPrivateKey)
	return serverCertificateEncoded, serverPrivateKeyEncoded, nil
}

// GetCertificatePEMEncoding returns a PEM encoded string for a given certificate.
func GetCertificatePEMEncoding(cert []byte) string {
	return GetPEMEncoding(cert, "CERTIFICATE")
}

// GetPrivateKeyPEMEncoding returns a PEM encoded string for a given private key.
func GetPrivateKeyPEMEncoding(privateKey *rsa.PrivateKey) string {
	return GetPEMEncoding(x509.MarshalPKCS1PrivateKey(privateKey), "RSA PRIVATE KEY")
}

// GetPEMEncoding returns a PEM encoded string for a given certificate or private key.
func GetPEMEncoding(cert []byte, encodingType string) (pemEncodingString string) {
	pemEncoding := new(bytes.Buffer)
	pem.Encode(pemEncoding, &pem.Block{
		Type:  encodingType,
		Bytes: cert,
	})

	return pemEncoding.String()
}
