package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetDefaultClusterInstance gets the default cluster instance.
func GetDefaultClusterInstance(httpsClient *http.Client, apiAddr string) (*v0.ClusterInstance, error) {
	var clusterInstance v0.ClusterInstance

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/cluster-instances?defaultcluster=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &clusterInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &clusterInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&clusterInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &clusterInstance, nil
}
