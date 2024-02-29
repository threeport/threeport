package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// GatewayDefinitionStatusDetail contains all the data for gateway definition
// status info.
type GatewayDefinitionStatusDetail struct {
	GatewayInstances *[]v0.GatewayInstance
}

// GetGatewayDefinitionStatus inspects a gateway definition and returns the status
// detials for it.
func GetGatewayDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	gatewayDefinitionId uint,
) (*GatewayDefinitionStatusDetail, error) {
	var gatewayDefStatus GatewayDefinitionStatusDetail

	// retrieve gateway instances related to gateway definition
	gatewayInsts, err := client.GetGatewayInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("gatewaydefinitionid=%d", gatewayDefinitionId),
	)
	if err != nil {
		return &gatewayDefStatus, fmt.Errorf("failed to retrieve gateway instances related to gateway definition: %w", err)
	}
	gatewayDefStatus.GatewayInstances = gatewayInsts

	return &gatewayDefStatus, nil
}
