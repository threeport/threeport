package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// AwsEksKubernetesRuntimeDefinitionStatusDetail contains all the data for AWS
// EKS kubernetes runtime instance status info.
type AwsEksKubernetesRuntimeDefinitionStatusDetail struct {
	AwsEksKubernetesRuntimeInstances *[]v0.AwsEksKubernetesRuntimeInstance
	KubernetesRuntimeDefinition      *v0.KubernetesRuntimeDefinition
}

// GetAwsEksKubernetesRuntimeDefinitionStatus inspects a AWS EKS kubernetes
// runtime definition and returns the status detials for it.
func GetAwsEksKubernetesRuntimeDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	awsEksKubernetesRuntimeDefinition *v0.AwsEksKubernetesRuntimeDefinition,
) (*AwsEksKubernetesRuntimeDefinitionStatusDetail, error) {
	var awsEksKubernetesRuntimeDefStatus AwsEksKubernetesRuntimeDefinitionStatusDetail

	// retrieve AWS EKS kubernetes runtime instances related to the definition
	awsEksKubernetesRuntimeInsts, err := client.GetAwsEksKubernetesRuntimeInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("awsekskubernetesruntimedefinitionid=%d", *awsEksKubernetesRuntimeDefinition.ID),
	)
	if err != nil {
		return &awsEksKubernetesRuntimeDefStatus, fmt.Errorf("failed to retrieve AWS EKS kubernetes runtime instances related to AWS EKS kubernetes runtime definition: %w", err)
	}
	awsEksKubernetesRuntimeDefStatus.AwsEksKubernetesRuntimeInstances = awsEksKubernetesRuntimeInsts

	// get associated kubernetes runtime definition
	kubernetesRuntimeDef, err := client.GetKubernetesRuntimeDefinitionByID(
		apiClient,
		apiEndpoint,
		*awsEksKubernetesRuntimeDefinition.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return &awsEksKubernetesRuntimeDefStatus, fmt.Errorf("failed to retrieve associated kubernetes runtime definition: %w", err)
	}
	awsEksKubernetesRuntimeDefStatus.KubernetesRuntimeDefinition = kubernetesRuntimeDef

	return &awsEksKubernetesRuntimeDefStatus, nil
}
