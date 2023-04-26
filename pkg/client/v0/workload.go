package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client"
)

// CreateWorkloadResourceDefinitions creates a new set of workload resource
// definitions.
func CreateWorkloadResourceDefinitions(
	workloadResourceDefinitions *[]v0.WorkloadResourceDefinition,
	apiAddr string,
) (*[]v0.WorkloadResourceDefinition, error) {
	jsonWorkloadResourceDefinitions, err := client.MarshalObject(workloadResourceDefinitions)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("failed to marshal provided objects to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s%s", apiAddr, v0.PathWorkloadResourceDefinitionSets),
		http.MethodPost,
		bytes.NewBuffer(jsonWorkloadResourceDefinitions),
		http.StatusCreated,
	)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadResourceDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return workloadResourceDefinitions, nil
}

// GetWorkloadResourceDefinitionsById fetches workload resource definitions
// by workload definition ID
func GetWorkloadResourceDefinitionsByWorkloadDefinitionID(id uint, apiAddr string) (*[]v0.WorkloadResourceDefinition, error) {
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s%s?workloaddefinitionid=%d", apiAddr, v0.PathWorkloadResourceDefinitions, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &workloadResourceDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &workloadResourceDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadResourceDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &workloadResourceDefinitions, nil
}

// GetWorkloadInstancesByWorkloadDefinitionID fetches a workload resource definition
// by workload definition ID
func GetWorkloadInstancesByWorkloadDefinitionID(id uint, apiAddr string) (*[]v0.WorkloadInstance, error) {
	var workloadInstances []v0.WorkloadInstance

	response, err := GetResponse(
		fmt.Sprintf("%s%s?workloaddefinitionid=%d", apiAddr, v0.PathWorkloadInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &workloadInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &workloadInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &workloadInstances, nil
}

// GetWorkloadResourceInstancesByWorkloadInstanceID fetches a workload resource definition
// by workload definition ID
func GetWorkloadResourceInstancesByWorkloadInstanceID(id uint, apiAddr string) (*[]v0.WorkloadResourceInstance, error) {
	var workloadResourceInstances []v0.WorkloadResourceInstance

	response, err := GetResponse(
		fmt.Sprintf("%s%s?workloadinstanceid=%d", apiAddr, v0.PathWorkloadResourceInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &workloadResourceInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &workloadResourceInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadResourceInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &workloadResourceInstances, nil
}
