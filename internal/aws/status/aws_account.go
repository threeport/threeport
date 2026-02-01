package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// AwsProviderStatusDetail contains all the data for AWS provider status info.
type AwsProviderStatusDetail struct {
	AwsEksKubernetesRuntimeDefinitions *[]v0.AwsEksKubernetesRuntimeDefinition
}

// GetAwsProviderStatus inspects an AWS Provider and returns the status details
// for it.
func GetAwsProviderStatus(
	apiClient *http.Client,
	apiEndpoint string,
	awsProviderId uint,
) (*AwsProviderStatusDetail, error) {
	var awsProviderStatus AwsProviderStatusDetail

	// retrieve AWS EKS Kubernetes runtime definitions related to this provider
	eksRuntimeDefs, err := client.GetAwsEksKubernetesRuntimeDefinitionsByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("awsproviderid=%d", awsProviderId),
	)
	if err != nil {
		return &awsProviderStatus, fmt.Errorf("failed to retrieve AWS EKS Kubernetes runtime definitions related to AWS provider: %w", err)
	}
	awsProviderStatus.AwsEksKubernetesRuntimeDefinitions = eksRuntimeDefs

	return &awsProviderStatus, nil
}
