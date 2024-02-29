package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// AwsEksKubernetesRuntimeInstanceStatusDetail contains all the data for AWS
// EKS kubernetes runtime instance status info.
type AwsEksKubernetesRuntimeInstanceStatusDetail struct {
	AwsEksKubernetesRuntimeDefinition *v0.AwsEksKubernetesRuntimeDefinition
	KubernetesRuntimeInstance         *v0.KubernetesRuntimeInstance
}

// GetAwsEksKubernetesRuntimeInstanceStatus inspects a AWS EKS kubernetes
// runtime instances and returns the status detials for it.
func GetAwsEksKubernetesRuntimeInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
) (*AwsEksKubernetesRuntimeInstanceStatusDetail, error) {
	var awsEksKubernetesRuntimeInstStatus AwsEksKubernetesRuntimeInstanceStatusDetail

	// retrieve AWS EKS kubernetes runtime definition for the instance
	awsEksKubernetesRuntimeDef, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		apiClient,
		apiEndpoint,
		*awsEksKubernetesRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return &awsEksKubernetesRuntimeInstStatus, fmt.Errorf("failed to retrieve AWS EKS kubernetes runtime definition related to AWS EKS kubernetes runtime instance: %w", err)
	}
	awsEksKubernetesRuntimeInstStatus.AwsEksKubernetesRuntimeDefinition = awsEksKubernetesRuntimeDef

	// get associated kubernetes runtime instance
	kubernetesRuntimeInst, err := client.GetKubernetesRuntimeInstanceByID(
		apiClient,
		apiEndpoint,
		*awsEksKubernetesRuntimeInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return &awsEksKubernetesRuntimeInstStatus, fmt.Errorf("failed to retrieve associated kubernetes runtime instances: %w", err)
	}
	awsEksKubernetesRuntimeInstStatus.KubernetesRuntimeInstance = kubernetesRuntimeInst

	return &awsEksKubernetesRuntimeInstStatus, nil
}
