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
	apiToken string,
) (*[]v0.WorkloadResourceDefinition, error) {
	jsonWorkloadResourceDefinitions, err := client.MarshalObject(workloadResourceDefinitions)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("failed to marshal provided objects to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s%s", apiAddr, v0.PathWorkloadResourceDefinitionSets),
		apiToken,
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

// UpdateWorkloadResourceDefinitions updates set of existing workload resource
// definitions.
func UpdateWorkloadResourceDefinitions(
	workloadResourceDefinitions *[]v0.WorkloadResourceDefinition,
	apiAddr string,
	apiToken string,
) (*[]v0.WorkloadResourceDefinition, error) {
	jsonWorkloadResourceDefinitions, err := client.MarshalObject(workloadResourceDefinitions)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("failed to marshal provided objects to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s%s", apiAddr, v0.PathWorkloadResourceDefinitionSets),
		apiToken,
		http.MethodPut,
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
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return workloadResourceDefinitions, nil
}

// DeleteWorkloadResourceDefinitions deletes set of workload resource
// definitions.
func DeleteWorkloadResourceDefinitions(
	workloadResourceDefinitions *[]v0.WorkloadResourceDefinition,
	apiAddr string,
	apiToken string,
) (*[]v0.WorkloadResourceDefinition, error) {
	jsonWorkloadResourceDefinitions, err := client.MarshalObject(workloadResourceDefinitions)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("failed to marshal provided objects to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s%s", apiAddr, v0.PathWorkloadResourceDefinitionSets),
		apiToken,
		http.MethodDelete,
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
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return workloadResourceDefinitions, nil
}

// GetWorkloadResourceDefinitionById fetches a workload resource definition
// by workload definition ID
func GetWorkloadResourceDefinitionsByWorkloadDefinitionID(id uint, apiAddr, apiToken string) (*[]v0.WorkloadResourceDefinition, error) {
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s%s?workloaddefinitionid=%d", apiAddr, v0.PathWorkloadResourceDefinitions, id),
		apiToken,
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
