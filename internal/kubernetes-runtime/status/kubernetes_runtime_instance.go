package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// KubernetesRuntimeInstanceStatusDetail contains all the data for
// kubernetes runtime instance status info.
type KubernetesRuntimeInstanceStatusDetail struct {
	KubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition
}

// GetKubernetesRuntimeInstanceStatus inspects a kubernetes
// runtime definition and returns the status detials for it.
func GetKubernetesRuntimeInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
) (*KubernetesRuntimeInstanceStatusDetail, error) {
	var kubernetesRuntimeInstStatus KubernetesRuntimeInstanceStatusDetail

	// retrieve kubernetes runtime definition for the instance
	kubernetesRuntimeDef, err := client.GetKubernetesRuntimeDefinitionByID(
		apiClient,
		apiEndpoint,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return &kubernetesRuntimeInstStatus, fmt.Errorf("failed to retrieve kubernetes runtime definition related to kubernetes runtime instance: %w", err)
	}
	kubernetesRuntimeInstStatus.KubernetesRuntimeDefinition = kubernetesRuntimeDef

	return &kubernetesRuntimeInstStatus, nil
}
