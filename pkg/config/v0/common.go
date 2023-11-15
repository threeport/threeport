package v0

import (
	"fmt"
	"net/http"

	client "github.com/threeport/threeport/pkg/client/v0"
)

type Exists interface {
	Exists(apiClient *http.Client, apiEndpoint string) (bool, error)
}

func (v *WorkloadDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find kubernetes runtime instance with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *WorkloadInstanceValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find kubernetes runtime instance with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *KubernetesRuntimeInstanceValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find kubernetes runtime instance with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *DomainNameDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find domain name definition with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *DomainNameInstanceValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, v.DomainNameDefinition.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find domain name definition with name %s: %w", v.DomainNameDefinition.Name, err)
	}
	return true, nil
}

func (v *GatewayDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find gateway definition with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *GatewayInstanceValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, v.GatewayDefinition.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find gateway definition with name %s: %w", v.GatewayDefinition.Name, err)
	}
	return true, nil
}

func (v *AwsRelationalDatabaseValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	awsRelationalDatabaseDefinition := AwsRelationalDatabaseDefinitionValues{
		Name: v.Name,
	}
	awsRelationalDatabaseInstanceValues := AwsRelationalDatabaseInstanceValues{
		Name: v.Name,
	}
	if _, err := awsRelationalDatabaseDefinition.Exists(apiClient, apiEndpoint); err != nil {
		return false, err
	}
	if _, err := awsRelationalDatabaseInstanceValues.Exists(apiClient, apiEndpoint); err != nil {
		return false, err
	}
	return true, nil
}

func (v *AwsRelationalDatabaseDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find aws relational database definition with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *AwsRelationalDatabaseInstanceValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find aws relational database definition with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *AwsObjectStorageBucketValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	awsObjectStorageBucketDefinition := AwsObjectStorageBucketDefinitionValues{
		Name: v.Name,
	}
	awsObjectStorageBucketInstance := AwsObjectStorageBucketInstanceValues{
		Name: v.Name,
	}
	if _, err := awsObjectStorageBucketDefinition.Exists(apiClient, apiEndpoint); err != nil {
		return false, err
	}
	if _, err := awsObjectStorageBucketInstance.Exists(apiClient, apiEndpoint); err != nil {
		return false, err
	}
	return true, nil
}

func (v *AwsObjectStorageBucketDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetAwsObjectStorageBucketDefinitionByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find aws object storage bucket definition with name %s: %w", v.Name, err)
	}
	return true, nil
}

func (v *AwsObjectStorageBucketInstanceValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	_, err := client.GetAwsObjectStorageBucketInstanceByName(apiClient, apiEndpoint, v.Name)
	if err != nil {
		return false, fmt.Errorf("failed to find aws object storage bucket definition with name %s: %w", v.Name, err)
	}
	return true, nil
}

func AssertDoesExist(apiClient *http.Client, apiEndpoint string, v ...Exists) (bool, error) {
	for _, value := range v {
		_, err := value.Exists(apiClient, apiEndpoint)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func AssertDoesNotExist(apiClient *http.Client, apiEndpoint string, v ...Exists) (bool, error) {
	for _, value := range v {
		exists, err := value.Exists(apiClient, apiEndpoint)
		if err != nil {
			return false, err
		}
		if exists {
			return false, fmt.Errorf("value %v already exists", )
		}
	}
	return true, nil
}

func (w *WorkloadValues) Exists(apiClient *http.Client, apiEndpoint string) (bool, error) {
	workloadDefinition := WorkloadDefinitionValues{
		Name: w.Name,
	}
	workloadInstance := WorkloadInstanceValues{
		Name: w.Name,
	}
	if _, err := workloadDefinition.Exists(apiClient, apiEndpoint); err != nil {
		return false, err
	}
	if _, err := workloadInstance.Exists(apiClient, apiEndpoint); err != nil {
		return false, err
	}
	return true, nil
}

func (w *WorkloadValues) Validate(apiClient *http.Client, apiEndpoint string) (bool, error) {

	exists := []Exists{w}

	if w.DomainName != nil && w.Gateway != nil {
		exists = append(exists, w.DomainName)
		exists = append(exists, w.Gateway)
	}

	if w.AwsRelationalDatabase != nil {
		exists = append(exists, w.AwsRelationalDatabase)
	}

	if w.AwsObjectStorageBucket != nil {
		exists = append(exists, w.AwsObjectStorageBucket)
	}

	assertion, err := AssertDoesNotExist(apiClient, apiEndpoint, exists...)
	if err != nil {
		return false, err
	}
	if !assertion {
		return false, fmt.Errorf("workload with name %s already exists", w.Name)
	}

	if assertion, err = AssertDoesExist(apiClient, apiEndpoint, w.KubernetesRuntimeInstance); err != nil {
		return false, err
	}
	if !assertion {
		return false, fmt.Errorf("kubernetes runtime instance with name %s does not exist", w.KubernetesRuntimeInstance.Name)
	}

	return true, nil
}
