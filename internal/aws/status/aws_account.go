package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// AwsAccountStatusDetail contains all the data for AWS account status info.
type AwsAccountStatusDetail struct {
	AwsEksKubernetesRuntimeDefinitions *[]v0.AwsEksKubernetesRuntimeDefinition
}

// GetAwsAccountStatus inspects an AWS Account and returns the status details
// for it.
func GetAwsAccountStatus(
	apiClient *http.Client,
	apiEndpoint string,
	awsAccountId uint,
) (*AwsAccountStatusDetail, error) {
	var awsAccountStatus AwsAccountStatusDetail

	// retrieve AWS EKS Kubernetes runtime definitions related to this account
	eksRuntimeDefs, err := client.GetAwsEksKubernetesRuntimeDefinitionsByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("awsaccountid=%d", awsAccountId),
	)
	if err != nil {
		return &awsAccountStatus, fmt.Errorf("failed to retrieve AWS EKS Kubernetes runtime definitions related to AWS account: %w", err)
	}
	awsAccountStatus.AwsEksKubernetesRuntimeDefinitions = eksRuntimeDefs

	return &awsAccountStatus, nil
}
