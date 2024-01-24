package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

var ErrorObjectNotFound = errors.New("object not found")
var ErrUnauthorized = errors.New("unauthorized")
var ErrForbidden = errors.New("forbidden")
var ErrConflict = errors.New("conflict")

// GetResponse calls the threeport API and returns a response.
func GetResponse(
	client *http.Client,
	url string,
	httpMethod string,
	reqBody *bytes.Buffer,
	reqHeader map[string]string,
	expectedStatusCode int,
) (*v0.Response, error) {

	urlScheme := "http://"

	// check if TLS is configured
	tlsConfigured := false
	if transport, ok := client.Transport.(*CustomTransport); ok {
		tlsConfigured = transport.IsTlsEnabled
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

	for key, value := range reqHeader {
		req.Header.Add(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute call to threeport API: %w", err)
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
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("%w: %s", ErrorObjectNotFound, response.Status.Error)
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("%w: %s", ErrUnauthorized, response.Status.Error)
		case http.StatusForbidden:
			return nil, fmt.Errorf("%w: %s", ErrForbidden, response.Status.Error)
		case http.StatusConflict:
			return nil, fmt.Errorf("%w: %s", ErrConflict, response.Status.Error)
		default:
			return nil, errors.New(fmt.Sprintf("API returned status: %d, %s\n%s\nexpected: %d", response.Status.Code, response.Status.Message, string(status), expectedStatusCode))
		}
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
