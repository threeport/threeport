package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/cli"
	configInternal "github.com/threeport/threeport/internal/config"
)

// GetHTTPClient returns an HTTP client with TLS configuration
// when authEnabled is true, and an HTTP client without TLS
// when authEnabled is false.
func GetHTTPClient(authEnabled bool) (*http.Client, error) {

	if !authEnabled {
		return &http.Client{}, nil
	}

	homeDir, _ := os.UserHomeDir()
	configDir := "/etc/threeport"
	var tlsConfig *tls.Config

	_, errConfigDirectory := os.Stat(filepath.Join(homeDir, ".config/threeport"))
	_, errThreeportCert := os.Stat(filepath.Join(configDir, "cert"))
	_, errThreeportCA := os.Stat(filepath.Join(configDir, "ca"))

	var rootCA string
	var cert tls.Certificate

	// load certificates from ~/.threeport or /etc/threeport
	if errConfigDirectory == nil {

		// load certificates from ~/.threeport
		threeportConfig := configInternal.GetThreeportConfig()
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load client certificate and private key
		cert, err = tls.X509KeyPair([]byte(clientCertificate), []byte(clientPrivateKey))
		if err != nil {
			return nil, err
		}

		// load root certificate authority
		if err != nil {
			return nil, err
		}

		rootCA = ca

	} else if errThreeportCert == nil && errThreeportCA == nil {

		// Load from /etc/threeport directory
		certFile := filepath.Join(configDir, "cert/tls.crt")
		keyFile := filepath.Join(configDir, "cert/tls.key")
		caFilePath := filepath.Join(configDir, "ca/tls.crt")

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

		rootCA = string(caCertBytes)
	} else {
		return nil, errors.New("could not find certificate files")
	}

	// create certificate pool and add certificate authority
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(rootCA))

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
