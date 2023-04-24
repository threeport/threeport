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
