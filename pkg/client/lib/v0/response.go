package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

var ErrObjectNotFound = errors.New("object not found")
var ErrUnauthorized = errors.New("unauthorized")
var ErrForbidden = errors.New("forbidden")
var ErrConflict = errors.New("conflict")
var ErrBadRequest = errors.New("bad request")
var ErrObjectOwned = errors.New("object owned externally")

// GetResponse calls the threeport API and returns a response.
func GetResponse(
	client *http.Client,
	url string,
	httpMethod string,
	reqBody *bytes.Buffer,
	reqHeader map[string]string,
	expectedStatusCode int,
) (*apiserver_lib.Response, error) {

	// If no scheme is present, determine based on transport configuration
	urlScheme := "http://"

	// check if TLS is configured
	if transport, ok := client.Transport.(*CustomTransport); ok && transport.IsTlsEnabled {
		// with auth enabled in Threeport, a CustomTransport is used with IsTlsEnabled=true
		urlScheme = "https://"
	} else if transport, ok := client.Transport.(*http.Transport); ok {
		// this is not used in Threeport, but can be used for connections to proxies and gateways
		// that are in front of the Threeport API and require HTTPS connections but perhaps without
		// client certificate authentication
		if transport.TLSClientConfig != nil {
			urlScheme = "https://"
		}
	}

	// Prepend the scheme to the URL
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from threeport API: %w", err)
	}

	var response apiserver_lib.Response
	if resp.StatusCode != expectedStatusCode {
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response body '%s' from threeport API: %w", string(respBody), err)
		}
		status, err := json.MarshalIndent(response.Status, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response status from threeport API: %w", err)
		}

		// If the error message is NOT from the Threeport API, e.g. an API gateway,
		// then use the response body as the error message
		errMessage := response.Status.Error
		if response.Status.Error == "" {
			errMessage = string(respBody)
		}

		// return specific errors that need to be identified with `errors.As`
		// elsewhere
		switch resp.StatusCode {
		case http.StatusBadRequest:
			if strings.Contains(errMessage, api_v0.ErrMsgExternalUpdateBlocked) {
				return nil, fmt.Errorf("%w: %s", ErrObjectOwned, errMessage)
			}
			return nil, fmt.Errorf("%w: %s", ErrBadRequest, errMessage)
		case http.StatusNotFound:
			return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, errMessage)
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("%w: %s", ErrUnauthorized, errMessage)
		case http.StatusForbidden:
			return nil, fmt.Errorf("%w: %s", ErrForbidden, errMessage)
		case http.StatusConflict:
			return nil, fmt.Errorf("%w: %s", ErrConflict, errMessage)
		default:
			return nil, fmt.Errorf(
				"API returned status: %d, %s\n%s\nexpected: %d",
				response.Status.Code,
				response.Status.Message,
				string(status),
				expectedStatusCode,
			)
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
