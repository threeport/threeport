package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v1 "github.com/threeport/threeport/pkg/api/v1"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// GetEventsJoinAttachedObjectReferenceByQueryString retrieves events joined to attached object reference by object ID.
func GetEventsJoinAttachedObjectReferenceByQueryString(apiClient *http.Client, apiAddr, queryString string) (*[]v1.Event, error) {
	var events []v1.Event

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/v1/events-join-attached-object-references?%s", apiAddr, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &events, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &events, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &events, nil
}