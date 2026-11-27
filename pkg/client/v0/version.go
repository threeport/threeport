package v0

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

const ApiVersion = "v0"

// restApiVersionTimeout bounds one request for the REST API's version, so a
// caller that only wants to know the version does not wait out the API client's
// much longer request timeout on an endpoint that does not answer.  It is a
// variable so tests can shorten it.
var restApiVersionTimeout = 10 * time.Second

// restApiVersionBodyLimit caps how much of a /version response is read.  The
// real response is a few dozen bytes.
const restApiVersionBodyLimit = 1 << 20

// restApiVersionResponse is the body of the REST API's /version route.  Unlike
// every other route the response is not wrapped in an API response, so
// client_lib.GetResponse, which decodes that wrapper, drops the version.
type restApiVersionResponse struct {
	Version string
}

// GetRestApiVersion returns the version the REST API at apiEndpoint reports on
// its /version route.  The apiEndpoint has no scheme, as stored in the
// threeport config.  The request is bounded by restApiVersionTimeout, or by the
// client's own timeout if that is shorter.  The caller's client is not changed.
func GetRestApiVersion(apiClient *http.Client, apiEndpoint string) (string, error) {
	// bound the request on a copy so the caller's client keeps its own timeout
	boundedClient := *apiClient
	if boundedClient.Timeout == 0 || boundedClient.Timeout > restApiVersionTimeout {
		boundedClient.Timeout = restApiVersionTimeout
	}

	url := client_lib.ApiUrl(apiClient, fmt.Sprintf("%s/version", apiEndpoint))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request to threeport API: %w", err)
	}

	resp, err := boundedClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute call to threeport API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, restApiVersionBodyLimit))
	if err != nil {
		return "", fmt.Errorf("failed to read response body from threeport API: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		message := restApiErrorMessage(resp.StatusCode, body)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return "", fmt.Errorf("%w: %s", client_lib.ErrUnauthorized, message)
		case http.StatusForbidden:
			return "", fmt.Errorf("%w: %s", client_lib.ErrForbidden, message)
		}
		return "", fmt.Errorf("threeport API returned status %d: %s", resp.StatusCode, message)
	}

	var versionResponse restApiVersionResponse
	if err := json.Unmarshal(body, &versionResponse); err != nil {
		return "", fmt.Errorf("failed to decode version response from threeport API: %w", err)
	}
	version := strings.TrimSpace(versionResponse.Version)
	if version == "" {
		return "", fmt.Errorf("threeport API version response has no version")
	}

	return version, nil
}

// restApiErrorMessage describes a non-200 response.  It prefers the error the
// API itself reported and falls back to the start of the body, which is what an
// API gateway in front of the API sends.
func restApiErrorMessage(statusCode int, body []byte) string {
	var response apiserver_lib.Response
	if err := json.Unmarshal(body, &response); err == nil && response.Status.Error != "" {
		return response.Status.Error
	}

	message := strings.TrimSpace(string(body))
	if len(message) > 200 {
		message = message[:200] + "..."
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return message
}
