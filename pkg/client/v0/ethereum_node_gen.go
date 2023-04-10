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

// GetEthereumNodeDefinitionByID feteches a ethereum node definition by ID
func GetEthereumNodeDefinitionByID(id uint, apiAddr, apiToken string) (*v0.EthereumNodeDefinition, error) {
	var ethereumNodeDefinition v0.EthereumNodeDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-definitions/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &ethereumNodeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &ethereumNodeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return &ethereumNodeDefinition, nil
}

// GetEthereumNodeDefinitionByName feteches a ethereum node definition by name
func GetEthereumNodeDefinitionByName(name, apiAddr, apiToken string) (*v0.EthereumNodeDefinition, error) {
	var ethereumNodeDefinitions []v0.EthereumNodeDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-definitions?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.EthereumNodeDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.EthereumNodeDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	switch {
	case len(ethereumNodeDefinitions) < 1:
		return &v0.EthereumNodeDefinition{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(ethereumNodeDefinitions) > 1:
		return &v0.EthereumNodeDefinition{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &ethereumNodeDefinitions[0], nil
}

// CreateEthereumNodeDefinition creates a new ethereum node definition
func CreateEthereumNodeDefinition(ethereumNodeDefinition *v0.EthereumNodeDefinition, apiAddr, apiToken string) (*v0.EthereumNodeDefinition, error) {
	jsonEthereumNodeDefinition, err := client.MarshalObject(ethereumNodeDefinition)
	if err != nil {
		return ethereumNodeDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-definitions", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonEthereumNodeDefinition),
		http.StatusCreated,
	)
	if err != nil {
		return ethereumNodeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return ethereumNodeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return ethereumNodeDefinition, nil
}

// UpdateEthereumNodeDefinition updates a ethereum node definition
func UpdateEthereumNodeDefinition(ethereumNodeDefinition *v0.EthereumNodeDefinition, apiAddr, apiToken string) (*v0.EthereumNodeDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	ethereumNodeDefinitionID := *ethereumNodeDefinition.ID
	ethereumNodeDefinition.ID = nil

	jsonEthereumNodeDefinition, err := client.MarshalObject(ethereumNodeDefinition)
	if err != nil {
		return ethereumNodeDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-definitions/%d", apiAddr, ApiVersion, ethereumNodeDefinitionID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonEthereumNodeDefinition),
		http.StatusOK,
	)
	if err != nil {
		return ethereumNodeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return ethereumNodeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return ethereumNodeDefinition, nil
}

// GetEthereumNodeInstanceByID feteches a ethereum node instance by ID
func GetEthereumNodeInstanceByID(id uint, apiAddr, apiToken string) (*v0.EthereumNodeInstance, error) {
	var ethereumNodeInstance v0.EthereumNodeInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-instances/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &ethereumNodeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &ethereumNodeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return &ethereumNodeInstance, nil
}

// GetEthereumNodeInstanceByName feteches a ethereum node instance by name
func GetEthereumNodeInstanceByName(name, apiAddr, apiToken string) (*v0.EthereumNodeInstance, error) {
	var ethereumNodeInstances []v0.EthereumNodeInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-instances?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.EthereumNodeInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.EthereumNodeInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	switch {
	case len(ethereumNodeInstances) < 1:
		return &v0.EthereumNodeInstance{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(ethereumNodeInstances) > 1:
		return &v0.EthereumNodeInstance{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &ethereumNodeInstances[0], nil
}

// CreateEthereumNodeInstance creates a new ethereum node instance
func CreateEthereumNodeInstance(ethereumNodeInstance *v0.EthereumNodeInstance, apiAddr, apiToken string) (*v0.EthereumNodeInstance, error) {
	jsonEthereumNodeInstance, err := client.MarshalObject(ethereumNodeInstance)
	if err != nil {
		return ethereumNodeInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-instances", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonEthereumNodeInstance),
		http.StatusCreated,
	)
	if err != nil {
		return ethereumNodeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return ethereumNodeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return ethereumNodeInstance, nil
}

// UpdateEthereumNodeInstance updates a ethereum node instance
func UpdateEthereumNodeInstance(ethereumNodeInstance *v0.EthereumNodeInstance, apiAddr, apiToken string) (*v0.EthereumNodeInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	ethereumNodeInstanceID := *ethereumNodeInstance.ID
	ethereumNodeInstance.ID = nil

	jsonEthereumNodeInstance, err := client.MarshalObject(ethereumNodeInstance)
	if err != nil {
		return ethereumNodeInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/ethereum-node-instances/%d", apiAddr, ApiVersion, ethereumNodeInstanceID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonEthereumNodeInstance),
		http.StatusOK,
	)
	if err != nil {
		return ethereumNodeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return ethereumNodeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ethereumNodeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return ethereumNodeInstance, nil
}
