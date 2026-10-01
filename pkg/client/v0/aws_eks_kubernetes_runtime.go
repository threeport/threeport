package v0

import (
	"fmt"
	"net/http"
)

// AwsEksClusterIdentity identifies an EKS cluster well enough to derive the
// ARNs of the IAM roles threeport creates for it.
type AwsEksClusterIdentity struct {
	// ClusterName is the name of the EKS cluster.
	ClusterName string

	// AccountId is the AWS account the cluster belongs to.
	AccountId string
}

// GetAwsEksClusterIdentityByK8sRuntimeInst returns the name and AWS account of
// the EKS cluster behind a Kubernetes runtime instance.  The IAM roles the
// cluster's add-ons assume are named after the cluster, so their ARNs are
// derived from this rather than read back from stored state.
func GetAwsEksClusterIdentityByK8sRuntimeInst(
	apiClient *http.Client,
	apiAddr string,
	kubernetesRuntimeInstanceId *uint,
) (*AwsEksClusterIdentity, error) {
	awsEksKubernetesRuntimeInstance, err := GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
		apiClient,
		apiAddr,
		*kubernetesRuntimeInstanceId,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws eks kubernetes runtime instance: %w", err)
	}
	if awsEksKubernetesRuntimeInstance.Name == nil {
		return nil, fmt.Errorf("aws eks kubernetes runtime instance has no name")
	}
	if awsEksKubernetesRuntimeInstance.AwsProviderID == nil {
		return nil, fmt.Errorf("aws eks kubernetes runtime instance has no AWS provider")
	}

	awsProvider, err := GetAwsProviderByID(
		apiClient,
		apiAddr,
		*awsEksKubernetesRuntimeInstance.AwsProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws provider: %w", err)
	}
	if awsProvider.AccountID == nil {
		return nil, fmt.Errorf("aws provider has no account ID")
	}

	return &AwsEksClusterIdentity{
		ClusterName: *awsEksKubernetesRuntimeInstance.Name,
		AccountId:   *awsProvider.AccountID,
	}, nil
}
