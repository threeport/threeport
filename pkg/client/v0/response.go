package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func GetResponse(url string, apiToken string, httpMethod string, reqBody *bytes.Buffer, expectedStatusCode int) (*v0.Response, error) {

	req, err := http.NewRequest(httpMethod, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response v0.Response
	if resp.StatusCode != expectedStatusCode {
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, err
		}
		status, err := json.MarshalIndent(response.Status, "", "  ")
		if err != nil {
			return nil, err
		}
		return nil, errors.New(fmt.Sprintf("API returned status: %d, %s\n%s", response.Status.Code, response.Status.Message, string(status)))
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
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
