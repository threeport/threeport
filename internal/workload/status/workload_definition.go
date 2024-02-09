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
	YamlDocument      string
}

// GetWorkloadDefinitionStatus inspects a workload instance and returns the status
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

	// get YAML document for workload definition
	workloadDef, err := client.GetWorkloadDefinitionByID(
		apiClient,
		apiEndpoint,
		workloadDefinitionId,
	)
	if err != nil {
		return &workloadDefStatus, fmt.Errorf("failed to retrieve workload definition: %w", err)
	}
	workloadDefStatus.YamlDocument = *workloadDef.YAMLDocument

	return &workloadDefStatus, nil
}
