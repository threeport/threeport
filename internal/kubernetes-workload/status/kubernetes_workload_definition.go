package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// WorkloadDefinitionStatusDetail contains all the data for kubernetes workload instance
// status info.
type WorkloadDefinitionStatusDetail struct {
	KubernetesWorkloadInstances *[]v0.KubernetesWorkloadInstance
}

// GetKubernetesWorkloadDefinitionStatus inspects a kubernetes workload definition and
// returns the status details for it.
func GetKubernetesWorkloadDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	workloadDefinitionId uint,
) (*WorkloadDefinitionStatusDetail, error) {
	var workloadDefStatus WorkloadDefinitionStatusDetail

	// retrieve workload instances related to kubernetes workload definition
	workloadInsts, err := client.GetKubernetesWorkloadInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("workloaddefinitionid=%d", workloadDefinitionId),
	)
	if err != nil {
		return &workloadDefStatus, fmt.Errorf("failed to retrieve workload instances related to kubernetes workload definition: %w", err)
	}
	workloadDefStatus.KubernetesWorkloadInstances = workloadInsts

	return &workloadDefStatus, nil
}
