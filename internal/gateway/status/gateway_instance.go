package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// GatewayInstanceStatusDetail contains all the data for
// gateway instance status info.
type GatewayInstanceStatusDetail struct {
	GatewayDefinition *v0.GatewayDefinition
}

// GetGatewayInstanceStatus inspects a gateway instance
// and returns the status detials for it.
func GetGatewayInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	gatewayInstance *v0.GatewayInstance,
) (*GatewayInstanceStatusDetail, error) {
	var gatewayInstStatus GatewayInstanceStatusDetail

	// retrieve gateway definition for the instance
	gatewayDef, err := client.GetGatewayDefinitionByID(
		apiClient,
		apiEndpoint,
		*gatewayInstance.GatewayDefinitionID,
	)
	if err != nil {
		return &gatewayInstStatus, fmt.Errorf("failed to retrieve gateway definition related to gateway instance: %w", err)
	}
	gatewayInstStatus.GatewayDefinition = gatewayDef

	return &gatewayInstStatus, nil
}
