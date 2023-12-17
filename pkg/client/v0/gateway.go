package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetGatewayHttpPortsByGatewayDefinitionId fetches a gateway http port by gateway definition ID.
func GetGatewayHttpPortsByGatewayDefinitionId(apiClient *http.Client, apiAddr string, id uint) (*[]v0.GatewayHttpPort, error) {
	var gatewayHttpPort []v0.GatewayHttpPort

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/gateway-http-ports?gatewaydefinitionid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &gatewayHttpPort, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &gatewayHttpPort, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gatewayHttpPort); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gatewayHttpPort, nil
}

// GetGatewayTcpPortsByGatewayDefinitionId fetches a gateway http port by gateway definition ID.
func GetGatewayTcpPortsByGatewayDefinitionId(apiClient *http.Client, apiAddr string, id uint) (*[]v0.GatewayTcpPort, error) {
	var gatewayTcpPort []v0.GatewayTcpPort

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/gateway-tcp-ports?gatewaydefinitionid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &gatewayTcpPort, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &gatewayTcpPort, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gatewayTcpPort); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gatewayTcpPort, nil
}
