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

// GetEventsFilteredByQueryString fetches events matching
// queryString, paging until the server has no more. max>0 caps the result; 0 fetches all.
func GetEventsFilteredByQueryString(
	apiClient *http.Client,
	apiAddr string,
	queryString string,
	max int,
) (*[]v0.Event, error) {
	var events []v0.Event

	// use max as the page size when it fits the server page cap; larger max keeps the default
	pageLimit := 0
	if max > 0 && max <= apiserver_lib.MaxPaginationLimitValue {
		pageLimit = max
	}

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?%s", apiAddr, v0.PathEventsFiltered, queryString)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?%s&queryid=%s&cursor=%d", apiAddr, v0.PathEventsFiltered, queryString, queryId, nextCursor)
		}
		if pageLimit > 0 {
			url = fmt.Sprintf("%s&limit=%d", url, pageLimit)
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

		// stop once the result has reached max; trim the last page's overshoot
		if max > 0 && len(allPageData) >= max {
			allPageData = allPageData[:max]
			break
		}

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
