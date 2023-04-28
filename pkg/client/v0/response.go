package v0

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

var ErrorObjectNotFound = errors.New("object not found")

// GetResponse calls the threeport API and returns a response.
func GetResponse(
	client *http.Client,
	url string,
	httpMethod string,
	reqBody *bytes.Buffer,
	expectedStatusCode int,
) (*v0.Response, error) {

	urlScheme := "http://"

	// check if TLS is configured
	tlsConfigured := false
	if transport, ok := client.Transport.(*http.Transport); ok {
		if transport.TLSClientConfig != nil {
			tlsConfigured = true
		}
	}

	// update url if TLS is configured
	if tlsConfigured {
		urlScheme = "https://"
	}
	url = urlScheme + url

	req, err := http.NewRequest(httpMethod, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to build request to threeport API: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed execute call to threeport API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from threeport API: %w", err)
	}

	var response v0.Response
	if resp.StatusCode != expectedStatusCode {
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response body from threeport API: %w", err)
		}
		status, err := json.MarshalIndent(response.Status, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response status from threeport API: %w", err)
		}
		// return specific errors that need to be identified with `errors.As`
		// elsewhere
		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrorObjectNotFound
		}
		return nil, errors.New(fmt.Sprintf("API returned status: %d, %s\n%s\nexpected: %d", response.Status.Code, response.Status.Message, string(status), expectedStatusCode))
	}

	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response body from threeport API: %w", err)
	}

	if IsDebug() {
		jsonResponse, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return nil, err
		}
		fmt.Println(string(jsonResponse))
	}

	return &response, nil
}

// loads certificates from ~/.threeport or /etc/threeport
func GetHTTPClient(authEnabled bool) (*http.Client, error) {

	if !authEnabled {
		return &http.Client{}, nil
	}

	homeDir, _ := os.UserHomeDir()
	var certFile, keyFile, caFile string

	_, errHomeDirectory := os.Stat(filepath.Join(homeDir, ".threeport"))
	_, errThreeportCert := os.Stat("/etc/threeport/cert")
	_, errThreeportCA := os.Stat("/etc/threeport/ca")

	if errHomeDirectory == nil {
		// Use certificates from ~/.threeport directory
		certFile = filepath.Join(homeDir, ".threeport", "tls.crt")
		keyFile = filepath.Join(homeDir, ".threeport", "tls.key")
		caFile = filepath.Join(homeDir, ".threeport", "ca.crt")
	} else if errThreeportCert == nil && errThreeportCA == nil {
		// Use certificates from /etc/threeport directory
		certFile = "/etc/threeport/cert/tls.crt"
		keyFile = "/etc/threeport/cert/tls.key"
		caFile = "/etc/threeport/ca/tls.crt"
	} else {
		return nil, errors.New("could not find certificate files")
	}

	// load client certificate and private key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	// load root certificate authority
	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	// create certificate pool and add certificate authority
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// create tls config required by http client
	tlsConfig := &tls.Config{
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
