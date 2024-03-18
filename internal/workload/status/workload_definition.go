package status

import (
	"fmt"
	"net/http"

	v1 "github.com/threeport/threeport/pkg/api/v1"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
)

// WorkloadDefinitionStatusDetail contains all the data for workload instance
// status info.
type WorkloadDefinitionStatusDetail struct {
	WorkloadInstances *[]v1.WorkloadInstance
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
	workloadInsts, err := client_v1.GetWorkloadInstancesByQueryString(
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
