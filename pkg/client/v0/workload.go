package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// CreateWorkloadResourceDefinitions creates a new set of workload resource
// definitions.
func CreateWorkloadResourceDefinitions(jsonWorkloadResourceDefinitions []byte, apiAddr, apiToken string) (*[]v0.WorkloadResourceDefinition, error) {
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_resource_definition_sets", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonWorkloadResourceDefinitions), http.StatusCreated)
	if err != nil {
		return &[]v0.WorkloadResourceDefinition{}, err
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &[]v0.WorkloadResourceDefinition{}, err
	}

	err = json.Unmarshal(jsonData, &workloadResourceDefinitions)
	if err != nil {
		return &[]v0.WorkloadResourceDefinition{}, err
	}

	return &workloadResourceDefinitions, nil
}

// GetWorkloadResourceDefinitionById fetches a workload resource definition
// by workload definition ID
func GetWorkloadResourceDefinitionByWorkloadDefinitionID(id uint, apiAddr, apiToken string) (*[]v0.WorkloadResourceDefinition, error) {
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_resource_definitions?workloaddefinitionid=%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &workloadResourceDefinitions, err
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &workloadResourceDefinitions, err
	}

	err = json.Unmarshal(jsonData, &workloadResourceDefinitions)
	if err != nil {
		return &workloadResourceDefinitions, err
	}

	return &workloadResourceDefinitions, nil
}
