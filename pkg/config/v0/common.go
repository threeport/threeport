package v0

import (
	"fmt"
	"net/http"

	client "github.com/threeport/threeport/pkg/client/v0"
)

type StateAssertion interface {
	Exists(apiClient *http.Client, apiEndpoint string, exists bool) error
}

func assert(err error, exists bool, object, name string) error {
	switch {
	case err == nil && exists:
		return nil
	case err != nil && !exists:
		return nil
	case err == nil && !exists:
		return fmt.Errorf("%s with name %s already exists", object, name)
	case err != nil && exists:
		return fmt.Errorf("failed to assert that %s with name %s exists: %w", object, name, err)
	default:
		return nil
	}
}

func (v *WorkloadDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "workload definition", v.Name)
}

func (v *WorkloadInstanceValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "workload instance", v.Name)
}

func (v *KubernetesRuntimeInstanceValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "kubernetes runtime instance", v.Name)
}

func (v *DomainNameDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "domain name definition", v.Name)
}

func (v *DomainNameInstanceValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, v.DomainNameDefinition.Name)
	return assert(err, exists, "domain name instance", v.DomainNameDefinition.Name)
}

func (v *GatewayDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "gateway definition", v.Name)
}

func (v *GatewayInstanceValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, v.GatewayDefinition.Name)
	return assert(err, exists, "gateway instance", v.GatewayDefinition.Name)
}

func (v *AwsRelationalDatabaseValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	awsRelationalDatabaseDefinition := AwsRelationalDatabaseDefinitionValues{
		Name: v.Name,
	}
	awsRelationalDatabaseInstanceValues := AwsRelationalDatabaseInstanceValues{
		Name: v.Name,
	}
	if err := awsRelationalDatabaseDefinition.Exists(apiClient, apiEndpoint, exists); err != nil {
		return err
	}
	if err := awsRelationalDatabaseInstanceValues.Exists(apiClient, apiEndpoint, exists); err != nil {
		return err
	}
	return nil
}

func (v *AwsRelationalDatabaseDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "aws relational database definition", v.Name)
}

func (v *AwsRelationalDatabaseInstanceValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "aws relational database instance", v.Name)
}

func (v *AwsObjectStorageBucketValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	awsObjectStorageBucketDefinition := AwsObjectStorageBucketDefinitionValues{
		Name: v.Name,
	}
	awsObjectStorageBucketInstance := AwsObjectStorageBucketInstanceValues{
		Name: v.Name,
	}
	if err := awsObjectStorageBucketDefinition.Exists(apiClient, apiEndpoint, exists); err != nil {
		return err
	}
	if err := awsObjectStorageBucketInstance.Exists(apiClient, apiEndpoint, exists); err != nil {
		return err
	}
	return nil
}

func (v *AwsObjectStorageBucketDefinitionValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetAwsObjectStorageBucketDefinitionByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "object storage bucket definition", v.Name)
}

func (v *AwsObjectStorageBucketInstanceValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	_, err := client.GetAwsObjectStorageBucketInstanceByName(apiClient, apiEndpoint, v.Name)
	return assert(err, exists, "object storage bucket instance", v.Name)
}

func AssertDoesExist(apiClient *http.Client, apiEndpoint string, v ...StateAssertion) error {
	for _, value := range v {
		err := value.Exists(apiClient, apiEndpoint, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func AssertDoesNotExist(apiClient *http.Client, apiEndpoint string, v ...StateAssertion) error {
	for _, value := range v {
		err := value.Exists(apiClient, apiEndpoint, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *WorkloadValues) Exists(apiClient *http.Client, apiEndpoint string, exists bool) error {
	workloadDefinition := WorkloadDefinitionValues{
		Name: w.Name,
	}
	workloadInstance := WorkloadInstanceValues{
		Name: w.Name,
	}
	if err := workloadDefinition.Exists(apiClient, apiEndpoint, exists); err != nil {
		return err
	}
	if err := workloadInstance.Exists(apiClient, apiEndpoint, exists); err != nil {
		return err
	}
	return nil
}

func (w *WorkloadValues) ValidateThreeportState(apiClient *http.Client, apiEndpoint string) error {

	assertions := []StateAssertion{w}

	if w.DomainName != nil && w.Gateway != nil {
		assertions = append(assertions, w.DomainName)
		assertions = append(assertions, w.Gateway)
	}

	if w.AwsRelationalDatabase != nil {
		assertions = append(assertions, w.AwsRelationalDatabase)
	}

	if w.AwsObjectStorageBucket != nil {
		assertions = append(assertions, w.AwsObjectStorageBucket)
	}

	if err := AssertDoesNotExist(apiClient, apiEndpoint, assertions...); err != nil {
		return err
	}

	if err := AssertDoesExist(apiClient, apiEndpoint, w.KubernetesRuntimeInstance); err != nil {
		return err
	}

	return nil
}
