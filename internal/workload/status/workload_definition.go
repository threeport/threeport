package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// WorkloadDefinitionStatusDetail contains all the data for workload instance
// status info.
type WorkloadDefinitionStatusDetail struct {
	WorkloadInstances *[]v0.WorkloadInstance
}

// GetWorkloadDefinitionStatus inspects a workload definition and returns the status
// detials for it.
func GetWorkloadDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	workloadDefinitionId uint,
) (*WorkloadDefinitionStatusDetail, error) {
	var workloadDefStatus WorkloadDefinitionStatusDetail

	// retrieve workload instances related to workload definition
	workloadInsts, err := client.GetWorkloadInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("workloaddefinitionid=%d", workloadDefinitionId),
	)
	if err != nil {
		return &workloadDefStatus, fmt.Errorf("failed to retrieve workload instances related to workload definition: %w", err)
	}
	workloadDefStatus.WorkloadInstances = workloadInsts

	return &workloadDefStatus, nil
}
