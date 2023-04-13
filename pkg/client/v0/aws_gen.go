// generated by 'threeport-codegen api-model' - do not edit

package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client"
	"net/http"
)

// GetAwsAccountByID feteches a aws account by ID
func GetAwsAccountByID(id uint, apiAddr, apiToken string) (*v0.AwsAccount, error) {
	var awsAccount v0.AwsAccount

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-accounts/%d", apiAddr, ApiVersion, id),
		apiToken,
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

// GetAwsAccountByName feteches a aws account by name
func GetAwsAccountByName(name, apiAddr, apiToken string) (*v0.AwsAccount, error) {
	var awsAccounts []v0.AwsAccount

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-accounts?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.AwsAccount{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.AwsAccount{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsAccounts); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(awsAccounts) < 1:
		return &v0.AwsAccount{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(awsAccounts) > 1:
		return &v0.AwsAccount{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &awsAccounts[0], nil
}

// CreateAwsAccount creates a new aws account
func CreateAwsAccount(awsAccount *v0.AwsAccount, apiAddr, apiToken string) (*v0.AwsAccount, error) {
	jsonAwsAccount, err := client.MarshalObject(awsAccount)
	if err != nil {
		return awsAccount, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-accounts", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonAwsAccount),
		http.StatusCreated,
	)
	if err != nil {
		return awsAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsAccount, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsAccount); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsAccount, nil
}

// UpdateAwsAccount updates a aws account
func UpdateAwsAccount(awsAccount *v0.AwsAccount, apiAddr, apiToken string) (*v0.AwsAccount, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsAccountID := *awsAccount.ID
	awsAccount.ID = nil

	jsonAwsAccount, err := client.MarshalObject(awsAccount)
	if err != nil {
		return awsAccount, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-accounts/%d", apiAddr, ApiVersion, awsAccountID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonAwsAccount),
		http.StatusOK,
	)
	if err != nil {
		return awsAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsAccount, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsAccount); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsAccount, nil
}

// DeleteAwsAccount delete a aws account
func DeleteAwsAccount(awsAccount *v0.AwsAccount, apiAddr, apiToken string) (*v0.AwsAccount, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsAccountID := *awsAccount.ID
	awsAccount.ID = nil

	jsonAwsAccount, err := client.MarshalObject(awsAccount)
	if err != nil {
		return awsAccount, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	_, err = GetResponse(
		fmt.Sprintf("%s/%s/aws-accounts/%d", apiAddr, ApiVersion, awsAccountID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonAwsAccount),
		http.StatusNoContent,
	)
	if err != nil {
		return awsAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return awsAccount, nil
}

// GetAwsEksClusterDefinitionByID feteches a aws eks cluster definition by ID
func GetAwsEksClusterDefinitionByID(id uint, apiAddr, apiToken string) (*v0.AwsEksClusterDefinition, error) {
	var awsEksClusterDefinition v0.AwsEksClusterDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-definitions/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsEksClusterDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsEksClusterDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsEksClusterDefinition, nil
}

// GetAwsEksClusterDefinitionByName feteches a aws eks cluster definition by name
func GetAwsEksClusterDefinitionByName(name, apiAddr, apiToken string) (*v0.AwsEksClusterDefinition, error) {
	var awsEksClusterDefinitions []v0.AwsEksClusterDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-definitions?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.AwsEksClusterDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.AwsEksClusterDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(awsEksClusterDefinitions) < 1:
		return &v0.AwsEksClusterDefinition{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(awsEksClusterDefinitions) > 1:
		return &v0.AwsEksClusterDefinition{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &awsEksClusterDefinitions[0], nil
}

// CreateAwsEksClusterDefinition creates a new aws eks cluster definition
func CreateAwsEksClusterDefinition(awsEksClusterDefinition *v0.AwsEksClusterDefinition, apiAddr, apiToken string) (*v0.AwsEksClusterDefinition, error) {
	jsonAwsEksClusterDefinition, err := client.MarshalObject(awsEksClusterDefinition)
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-definitions", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonAwsEksClusterDefinition),
		http.StatusCreated,
	)
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsEksClusterDefinition, nil
}

// UpdateAwsEksClusterDefinition updates a aws eks cluster definition
func UpdateAwsEksClusterDefinition(awsEksClusterDefinition *v0.AwsEksClusterDefinition, apiAddr, apiToken string) (*v0.AwsEksClusterDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsEksClusterDefinitionID := *awsEksClusterDefinition.ID
	awsEksClusterDefinition.ID = nil

	jsonAwsEksClusterDefinition, err := client.MarshalObject(awsEksClusterDefinition)
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-definitions/%d", apiAddr, ApiVersion, awsEksClusterDefinitionID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonAwsEksClusterDefinition),
		http.StatusOK,
	)
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsEksClusterDefinition, nil
}

// DeleteAwsEksClusterDefinition delete a aws eks cluster definition
func DeleteAwsEksClusterDefinition(awsEksClusterDefinition *v0.AwsEksClusterDefinition, apiAddr, apiToken string) (*v0.AwsEksClusterDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsEksClusterDefinitionID := *awsEksClusterDefinition.ID
	awsEksClusterDefinition.ID = nil

	jsonAwsEksClusterDefinition, err := client.MarshalObject(awsEksClusterDefinition)
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	_, err = GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-definitions/%d", apiAddr, ApiVersion, awsEksClusterDefinitionID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonAwsEksClusterDefinition),
		http.StatusNoContent,
	)
	if err != nil {
		return awsEksClusterDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return awsEksClusterDefinition, nil
}

// GetAwsEksClusterInstanceByID feteches a aws eks cluster instance by ID
func GetAwsEksClusterInstanceByID(id uint, apiAddr, apiToken string) (*v0.AwsEksClusterInstance, error) {
	var awsEksClusterInstance v0.AwsEksClusterInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-instances/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsEksClusterInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsEksClusterInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsEksClusterInstance, nil
}

// GetAwsEksClusterInstanceByName feteches a aws eks cluster instance by name
func GetAwsEksClusterInstanceByName(name, apiAddr, apiToken string) (*v0.AwsEksClusterInstance, error) {
	var awsEksClusterInstances []v0.AwsEksClusterInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-instances?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.AwsEksClusterInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.AwsEksClusterInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(awsEksClusterInstances) < 1:
		return &v0.AwsEksClusterInstance{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(awsEksClusterInstances) > 1:
		return &v0.AwsEksClusterInstance{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &awsEksClusterInstances[0], nil
}

// CreateAwsEksClusterInstance creates a new aws eks cluster instance
func CreateAwsEksClusterInstance(awsEksClusterInstance *v0.AwsEksClusterInstance, apiAddr, apiToken string) (*v0.AwsEksClusterInstance, error) {
	jsonAwsEksClusterInstance, err := client.MarshalObject(awsEksClusterInstance)
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-instances", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonAwsEksClusterInstance),
		http.StatusCreated,
	)
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsEksClusterInstance, nil
}

// UpdateAwsEksClusterInstance updates a aws eks cluster instance
func UpdateAwsEksClusterInstance(awsEksClusterInstance *v0.AwsEksClusterInstance, apiAddr, apiToken string) (*v0.AwsEksClusterInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsEksClusterInstanceID := *awsEksClusterInstance.ID
	awsEksClusterInstance.ID = nil

	jsonAwsEksClusterInstance, err := client.MarshalObject(awsEksClusterInstance)
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-instances/%d", apiAddr, ApiVersion, awsEksClusterInstanceID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonAwsEksClusterInstance),
		http.StatusOK,
	)
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsEksClusterInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsEksClusterInstance, nil
}

// DeleteAwsEksClusterInstance delete a aws eks cluster instance
func DeleteAwsEksClusterInstance(awsEksClusterInstance *v0.AwsEksClusterInstance, apiAddr, apiToken string) (*v0.AwsEksClusterInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsEksClusterInstanceID := *awsEksClusterInstance.ID
	awsEksClusterInstance.ID = nil

	jsonAwsEksClusterInstance, err := client.MarshalObject(awsEksClusterInstance)
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	_, err = GetResponse(
		fmt.Sprintf("%s/%s/aws-eks-cluster-instances/%d", apiAddr, ApiVersion, awsEksClusterInstanceID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonAwsEksClusterInstance),
		http.StatusNoContent,
	)
	if err != nil {
		return awsEksClusterInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return awsEksClusterInstance, nil
}

// GetAwsRelationalDatabaseDefinitionByID feteches a aws relational database definition by ID
func GetAwsRelationalDatabaseDefinitionByID(id uint, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseDefinition, error) {
	var awsRelationalDatabaseDefinition v0.AwsRelationalDatabaseDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-definitions/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsRelationalDatabaseDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsRelationalDatabaseDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsRelationalDatabaseDefinition, nil
}

// GetAwsRelationalDatabaseDefinitionByName feteches a aws relational database definition by name
func GetAwsRelationalDatabaseDefinitionByName(name, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseDefinition, error) {
	var awsRelationalDatabaseDefinitions []v0.AwsRelationalDatabaseDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-definitions?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.AwsRelationalDatabaseDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.AwsRelationalDatabaseDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(awsRelationalDatabaseDefinitions) < 1:
		return &v0.AwsRelationalDatabaseDefinition{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(awsRelationalDatabaseDefinitions) > 1:
		return &v0.AwsRelationalDatabaseDefinition{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &awsRelationalDatabaseDefinitions[0], nil
}

// CreateAwsRelationalDatabaseDefinition creates a new aws relational database definition
func CreateAwsRelationalDatabaseDefinition(awsRelationalDatabaseDefinition *v0.AwsRelationalDatabaseDefinition, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseDefinition, error) {
	jsonAwsRelationalDatabaseDefinition, err := client.MarshalObject(awsRelationalDatabaseDefinition)
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-definitions", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonAwsRelationalDatabaseDefinition),
		http.StatusCreated,
	)
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsRelationalDatabaseDefinition, nil
}

// UpdateAwsRelationalDatabaseDefinition updates a aws relational database definition
func UpdateAwsRelationalDatabaseDefinition(awsRelationalDatabaseDefinition *v0.AwsRelationalDatabaseDefinition, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsRelationalDatabaseDefinitionID := *awsRelationalDatabaseDefinition.ID
	awsRelationalDatabaseDefinition.ID = nil

	jsonAwsRelationalDatabaseDefinition, err := client.MarshalObject(awsRelationalDatabaseDefinition)
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-definitions/%d", apiAddr, ApiVersion, awsRelationalDatabaseDefinitionID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonAwsRelationalDatabaseDefinition),
		http.StatusOK,
	)
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsRelationalDatabaseDefinition, nil
}

// DeleteAwsRelationalDatabaseDefinition delete a aws relational database definition
func DeleteAwsRelationalDatabaseDefinition(awsRelationalDatabaseDefinition *v0.AwsRelationalDatabaseDefinition, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsRelationalDatabaseDefinitionID := *awsRelationalDatabaseDefinition.ID
	awsRelationalDatabaseDefinition.ID = nil

	jsonAwsRelationalDatabaseDefinition, err := client.MarshalObject(awsRelationalDatabaseDefinition)
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	_, err = GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-definitions/%d", apiAddr, ApiVersion, awsRelationalDatabaseDefinitionID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonAwsRelationalDatabaseDefinition),
		http.StatusNoContent,
	)
	if err != nil {
		return awsRelationalDatabaseDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return awsRelationalDatabaseDefinition, nil
}

// GetAwsRelationalDatabaseInstanceByID feteches a aws relational database instance by ID
func GetAwsRelationalDatabaseInstanceByID(id uint, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseInstance, error) {
	var awsRelationalDatabaseInstance v0.AwsRelationalDatabaseInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-instances/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &awsRelationalDatabaseInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &awsRelationalDatabaseInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &awsRelationalDatabaseInstance, nil
}

// GetAwsRelationalDatabaseInstanceByName feteches a aws relational database instance by name
func GetAwsRelationalDatabaseInstanceByName(name, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseInstance, error) {
	var awsRelationalDatabaseInstances []v0.AwsRelationalDatabaseInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-instances?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.AwsRelationalDatabaseInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.AwsRelationalDatabaseInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(awsRelationalDatabaseInstances) < 1:
		return &v0.AwsRelationalDatabaseInstance{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(awsRelationalDatabaseInstances) > 1:
		return &v0.AwsRelationalDatabaseInstance{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &awsRelationalDatabaseInstances[0], nil
}

// CreateAwsRelationalDatabaseInstance creates a new aws relational database instance
func CreateAwsRelationalDatabaseInstance(awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseInstance, error) {
	jsonAwsRelationalDatabaseInstance, err := client.MarshalObject(awsRelationalDatabaseInstance)
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-instances", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonAwsRelationalDatabaseInstance),
		http.StatusCreated,
	)
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsRelationalDatabaseInstance, nil
}

// UpdateAwsRelationalDatabaseInstance updates a aws relational database instance
func UpdateAwsRelationalDatabaseInstance(awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsRelationalDatabaseInstanceID := *awsRelationalDatabaseInstance.ID
	awsRelationalDatabaseInstance.ID = nil

	jsonAwsRelationalDatabaseInstance, err := client.MarshalObject(awsRelationalDatabaseInstance)
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-instances/%d", apiAddr, ApiVersion, awsRelationalDatabaseInstanceID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonAwsRelationalDatabaseInstance),
		http.StatusOK,
	)
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&awsRelationalDatabaseInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return awsRelationalDatabaseInstance, nil
}

// DeleteAwsRelationalDatabaseInstance delete a aws relational database instance
func DeleteAwsRelationalDatabaseInstance(awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance, apiAddr, apiToken string) (*v0.AwsRelationalDatabaseInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	awsRelationalDatabaseInstanceID := *awsRelationalDatabaseInstance.ID
	awsRelationalDatabaseInstance.ID = nil

	jsonAwsRelationalDatabaseInstance, err := client.MarshalObject(awsRelationalDatabaseInstance)
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	_, err = GetResponse(
		fmt.Sprintf("%s/%s/aws-relational-database-instances/%d", apiAddr, ApiVersion, awsRelationalDatabaseInstanceID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonAwsRelationalDatabaseInstance),
		http.StatusNoContent,
	)
	if err != nil {
		return awsRelationalDatabaseInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return awsRelationalDatabaseInstance, nil
}
