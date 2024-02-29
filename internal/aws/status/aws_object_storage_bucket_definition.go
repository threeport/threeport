package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// AwsObjectStorageBucketDefinitionStatusDetail contains all the data for AWS
// EKS kubernetes runtime instance status info.
type AwsObjectStorageBucketDefinitionStatusDetail struct {
	AwsObjectStorageBucketInstances *[]v0.AwsObjectStorageBucketInstance
}

// GetAwsObjectStorageBucketDefinitionStatus inspects an AWS object storage
// bucket definition and returns the status detials for it.
func GetAwsObjectStorageBucketDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	awsObjectStorageBucketDefinition *v0.AwsObjectStorageBucketDefinition,
) (*AwsObjectStorageBucketDefinitionStatusDetail, error) {
	var awsObjectStorageBucketDefStatus AwsObjectStorageBucketDefinitionStatusDetail

	// retrieve AWS object storage bucket instances related to the definition
	awsObjectStorageBucketInsts, err := client.GetAwsObjectStorageBucketInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("awsekskubernetesruntimedefinitionid=%d", *awsObjectStorageBucketDefinition.ID),
	)
	if err != nil {
		return &awsObjectStorageBucketDefStatus, fmt.Errorf("failed to retrieve AWS EKS kubernetes runtime instances related to AWS EKS kubernetes runtime definition: %w", err)
	}
	awsObjectStorageBucketDefStatus.AwsObjectStorageBucketInstances = awsObjectStorageBucketInsts

	return &awsObjectStorageBucketDefStatus, nil
}
