package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// KubernetesRuntimeDefinitionStatusDetail contains all the data for
// kubernetes runtime instance status info.
type KubernetesRuntimeDefinitionStatusDetail struct {
	KubernetesRuntimeInstances *[]v0.KubernetesRuntimeInstance
}

// GetKubernetesRuntimeDefinitionStatus inspects a kubernetes
// runtime definition and returns the status detials for it.
func GetKubernetesRuntimeDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	KubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
) (*KubernetesRuntimeDefinitionStatusDetail, error) {
	var kubernetesRuntimeDefStatus KubernetesRuntimeDefinitionStatusDetail

	// retrieve kubernetes runtime instances related to the definition
	kubernetesRuntimeInsts, err := client.GetKubernetesRuntimeInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("kubernetesruntimedefinitionid=%d", *KubernetesRuntimeDefinition.ID),
	)
	if err != nil {
		return &kubernetesRuntimeDefStatus, fmt.Errorf("failed to retrieve kubernetes runtime instances related to kubernetes runtime definition: %w", err)
	}
	kubernetesRuntimeDefStatus.KubernetesRuntimeInstances = kubernetesRuntimeInsts

	return &kubernetesRuntimeDefStatus, nil
}
