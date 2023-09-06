package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetAwsAccountByDefaultAccount fetches the default AWS account.
func GetAwsAccountByDefaultAccount(apiClient *http.Client, apiAddr string) (*v0.AwsAccount, error) {
	var awsAccount v0.AwsAccount

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/aws-accounts?default=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}
	// TODO: check for response.Data len == 0

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsAccount, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsAccount); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsAccount, nil
}

// GetAwsAccountByAccountID fetches a AWS account by the AWS Account ID.
func GetAwsAccountByAccountID(apiClient *http.Client, apiAddr string, accountID string) (*v0.AwsAccount, error) {
	var awsAccount v0.AwsAccount

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/aws-accounts?accountid=%s", apiAddr, ApiVersion, accountID),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsAccount, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsAccount); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsAccount, nil
}

// GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef fetches a aws eks kubernetes runtime definition by ID.
func GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(apiClient *http.Client, apiAddr string, id uint) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	var awsEksKubernetesRuntimeDefinition v0.AwsEksKubernetesRuntimeDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/aws-eks-kubernetes-runtime-definitions?kubernetesruntimedefinitionid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsEksKubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &awsEksKubernetesRuntimeDefinition, errors.New(fmt.Sprintf("no object found with ID %d", id))
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsEksKubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksKubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsEksKubernetesRuntimeDefinition, nil
}

// GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst fetches a aws eks kubernetes runtime instance by ID.
func GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient *http.Client, apiAddr string, id uint) (*v0.AwsEksKubernetesRuntimeInstance, error) {
	var awsEksKubernetesRuntimeInstance v0.AwsEksKubernetesRuntimeInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/aws-eks-kubernetes-runtime-instances?kubernetesruntimeinstanceid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsEksKubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &awsEksKubernetesRuntimeInstance, errors.New(fmt.Sprintf("no object found with ID %d", id))
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsEksKubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksKubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsEksKubernetesRuntimeInstance, nil
}

// GetAwsRelationalDatabaseInstancesByAwsRelationalDatabaseDefinitionID fetches
// AWS relational database instances by AWS relational database definition ID.
func GetAwsRelationalDatabaseInstancesByAwsRelationalDatabaseDefinitionID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.AwsRelationalDatabaseInstance, error) {
	var awsRelationalDatabaseInstances []v0.AwsRelationalDatabaseInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?awsrelationaldatabasedefinitionid=%d", apiAddr, v0.PathAwsRelationalDatabaseInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsRelationalDatabaseInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &awsRelationalDatabaseInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsRelationalDatabaseInstances, nil
}
