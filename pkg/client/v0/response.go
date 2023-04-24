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

// GetResponse calls the threeport API and returns a response.
func GetResponse(
	url string,
	apiToken string,
	httpMethod string,
	reqBody *bytes.Buffer,
	expectedStatusCode int,
) (*v0.Response, error) {

	req, err := http.NewRequest(httpMethod, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to build request to threeport API: %w", err)
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}

	// load certificates to authenticate via TLS conneection
	tlsConfig, err := loadCertificates()
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates: %w", err)
	}

	// configure http client to use certificates
	client.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

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
func loadCertificates() (*tls.Config, error) {
	homeDir, _ := os.UserHomeDir()
	var certFile, keyFile, caFile string

	_, errHomeDirectory := os.Stat(filepath.Join(homeDir, ".threeport"))
	_, errThreeportCert := os.Stat("/etc/threeport/cert")
	_, errThreeportCA := os.Stat("/etc/threeport/ca")

	if errHomeDirectory == nil {
		// Use certificates from ~/.threeport directory
		certFile = filepath.Join(homeDir, ".threeport", "cert")
		keyFile = filepath.Join(homeDir, ".threeport", "key")
		caFile = filepath.Join(homeDir, ".threeport", "ca")
	} else if errThreeportCert == nil && errThreeportCA == nil {
		// Use certificates from /etc/threeport directory
		certFile = "/etc/threeport/cert"
		keyFile = "/etc/threeport/key"
		caFile = "/etc/threeport/ca"
	} else {
		return nil, errors.New("could not find certificate files")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	return tlsConfig, nil
}
