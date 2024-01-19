package v0

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// GetHTTPClient returns an HTTP client with TLS configuration when authEnabled
// is true, and an HTTP client without TLS when authEnabled is false.  If used
// by a workload in a runtime environment, the values for the TLS assets should
// be empty strings.  In that case they will be read from disk (from a mounted
// secret).  If used by a command line tool, the TLS assets should be obtained
// from the threeport config prior to calling this function and then provied.
func GetHTTPClient(
	authEnabled bool,
	ca string,
	clientCertificate string,
	clientPrivateKey string,
	sessionToken string,
) (*http.Client, error) {
	if !authEnabled {
		return &http.Client{
			Transport: &CustomTransport{
				CustomRoundTripper: Chain(nil),
				IsTlsEnabled:       false,
			},
		}, nil
	}

	configDir := "/etc/threeport"
	var tlsConfig *tls.Config

	_, errThreeportCert := os.Stat(filepath.Join(configDir, "cert"))
	_, errThreeportCA := os.Stat(filepath.Join(configDir, "ca"))

	var rootCA string
	var cert tls.Certificate

	// get TLS asset values
	// first check to see if they were provided and use those values if they were
	// (for command line usage)
	// then check the filesystem at the expected location (for workload usage)
	if ca != "" && clientCertificate != "" && clientPrivateKey != "" {
		// load client certificate and private key
		loadedCert, err := tls.X509KeyPair([]byte(clientCertificate), []byte(clientPrivateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate and key: %w", err)
		}
		cert = loadedCert

		// load root certificate authority
		rootCA = ca

	} else if errThreeportCert == nil && errThreeportCA == nil {
		// load from /etc/threeport directory
		certFile := filepath.Join(configDir, "cert/tls.crt")
		keyFile := filepath.Join(configDir, "cert/tls.key")
		caFilePath := filepath.Join(configDir, "ca/tls.crt")

		// load client certificate and private key
		var err error
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate and key: %w", err)
		}

		// load root certificate authority
		caCertBytes, err := os.ReadFile(caFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load root CA: %w", err)
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

	tlsTransport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	var apiClient *http.Client
	var customTransport *CustomTransport

	if sessionToken != "" {
		customTransport = &CustomTransport{
			CustomRoundTripper: Chain(tlsTransport),
			IsTlsEnabled:       true,
		}
	} else {
		customTransport = &CustomTransport{
			CustomRoundTripper: Chain(
				tlsTransport,
				AddHeader("Authorization", fmt.Sprintf("Bearer %s", sessionToken))),
			IsTlsEnabled: true,
		}
	}

	apiClient = &http.Client{
		Transport: customTransport,
	}

	return apiClient, nil
}

// GetKubeDynamicClientAndMapper returns a dynamic client and rest mapper for a given kubeconfig path.
func GetKubeDynamicClientAndMapper(kubeconfigPath string) (*dynamic.DynamicClient, meta.RESTMapper, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, err
	}

	// Create a dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	// Create a discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	// the rest mapper allows us to determine resource types
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, nil, err
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return dynamicClient, mapper, nil
}
