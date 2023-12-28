package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// GetGatewayPortsAsString returns a string representation of the ports
// exposed by a gateway definition
func GetGatewayPortsAsString(apiClient *http.Client, apiAddr string, id uint) (string, error) {

	gatewayHttpPorts, err := GetGatewayHttpPortsByGatewayDefinitionId(apiClient, apiAddr, id)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway http ports: %w", err)
	}

	gatewayTcpPorts, err := GetGatewayTcpPortsByGatewayDefinitionId(apiClient, apiAddr, id)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway tcp ports: %w", err)
	}

	formattedPorts := []string{}

	for _, httpPort := range *gatewayHttpPorts {
		var protocol string
		if *httpPort.TLSEnabled {
			protocol = "https"
		} else {
			protocol = "http"
		}
		formattedPorts = append(formattedPorts, fmt.Sprintf("%s/%d", protocol, *httpPort.Port))
	}

	for _, tcpPort := range *gatewayTcpPorts {
		var protocol string
		if *tcpPort.TLSEnabled {
			protocol = "tls"
		} else {
			protocol = "tcp"
		}
		formattedPorts = append(formattedPorts, fmt.Sprintf("%s/%d", protocol, *tcpPort.Port))
	}

	return strings.Join(formattedPorts, ","), nil
}
