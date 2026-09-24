package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// GetEventsJoinAttachedObjectReferenceByQueryString retrieves events joined to
// attached object reference by object ID.
func GetEventsJoinAttachedObjectReferenceByQueryString(
	apiClient *http.Client,
	apiAddr string,
	queryString string,
) (*[]v0.Event, error) {
	var events []v0.Event

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s/v0/events-join-attached-object-references?%s", apiAddr, queryString)
		if queryId != "" {
			url = fmt.Sprintf("%s/v0/events-join-attached-object-references?%s&queryid=%s&cursor=%d", apiAddr, queryString, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &events, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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
