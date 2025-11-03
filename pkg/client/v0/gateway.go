package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// GetGatewayHttpPortsByGatewayDefinitionId fetches a gateway http port by gateway definition ID.
func GetGatewayHttpPortsByGatewayDefinitionId(apiClient *http.Client, apiAddr string, id uint) (*[]v0.GatewayHttpPort, error) {
	var gatewayHttpPort []v0.GatewayHttpPort

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?gatewaydefinitionid=%d", apiAddr, v0.PathGatewayHttpPorts, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?gatewaydefinitionid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathGatewayHttpPorts, id, queryId, nextCursor)
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
			return &gatewayHttpPort, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
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

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?gatewaydefinitionid=%d", apiAddr, v0.PathGatewayTcpPorts, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?gatewaydefinitionid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathGatewayTcpPorts, id, queryId, nextCursor)
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
			return &gatewayTcpPort, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
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
		return &gatewayTcpPort, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gatewayTcpPort); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gatewayTcpPort, nil
}

// GetGatewayHttpAndTcpPortsByGatewayDefinitionId fetches gateway http and tcp ports by gateway definition ID.
func GetGatewayHttpAndTcpPortsByGatewayDefinitionId(apiClient *http.Client, apiAddr string, id uint) (*[]v0.GatewayHttpPort, *[]v0.GatewayTcpPort, error) {
	gatewayHttpPorts, err := GetGatewayHttpPortsByGatewayDefinitionId(apiClient, apiAddr, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}

	gatewayTcpPorts, err := GetGatewayTcpPortsByGatewayDefinitionId(apiClient, apiAddr, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gateway tcp ports: %w", err)
	}

	return gatewayHttpPorts, gatewayTcpPorts, nil
}

// GetGatewayPortsAsString returns a string representation of the ports
// exposed by a gateway definition
func GetGatewayPortsAsString(apiClient *http.Client, apiAddr string, id uint) (string, error) {
	gatewayHttpPorts, gatewayTcpPorts, err := GetGatewayHttpAndTcpPortsByGatewayDefinitionId(apiClient, apiAddr, id)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}
	formattedPorts := []string{}

	for _, httpPort := range *gatewayHttpPorts {
		var protocol string
		if httpPort.TLSEnabled != nil && *httpPort.TLSEnabled {
			protocol = "https"
		} else {
			protocol = "http"
		}
		formattedPorts = append(formattedPorts, fmt.Sprintf("%s/%d", protocol, *httpPort.Port))
	}

	for _, tcpPort := range *gatewayTcpPorts {
		var protocol string
		if tcpPort.TLSEnabled != nil && *tcpPort.TLSEnabled {
			protocol = "tls"
		} else {
			protocol = "tcp"
		}
		formattedPorts = append(formattedPorts, fmt.Sprintf("%s/%d", protocol, *tcpPort.Port))
	}

	return strings.Join(formattedPorts, ","), nil
}
