package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// HelmWorkloadDefinitionStatusDetail contains all the data for helm workload instance
// status info.
type HelmWorkloadDefinitionStatusDetail struct {
	HelmWorkloadInstances *[]v0.HelmWorkloadInstance
}

// GetHelmWorkloadDefinitionStatus inspects a helm workload definition and returns the status
// detials for it.
func GetHelmWorkloadDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	helmWorkloadDefinitionId uint,
) (*HelmWorkloadDefinitionStatusDetail, error) {
	var helmWorkloadDefStatus HelmWorkloadDefinitionStatusDetail

	// retrieve helm workload instances related to helm workload definition
	helmWorkloadInsts, err := client.GetHelmWorkloadInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("domainnamedefinitionid=%d", helmWorkloadDefinitionId),
	)
	if err != nil {
		return &helmWorkloadDefStatus, fmt.Errorf("failed to retrieve helm workload instances related to helm workload definition: %w", err)
	}
	helmWorkloadDefStatus.HelmWorkloadInstances = helmWorkloadInsts

	return &helmWorkloadDefStatus, nil
}
