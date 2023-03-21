package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetWorkloadDefinitionByID fetches a workload definition by ID
func GetWorkloadDefinitionByID(id uint, apiAddr, apiToken string) (*v0.WorkloadDefinition, error) {
	var workloadDefinition v0.WorkloadDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_definitions/%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &workloadDefinition, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &workloadDefinition, err
	}

	err = json.Unmarshal(jsonData, &workloadDefinition)
	if err != nil {
		return &workloadDefinition, err
	}

	return &workloadDefinition, nil
}

// GetWorkloadDefinitionByName fetches a workload definition from the Threeport
// API by name.
func GetWorkloadDefinitionByName(name, apiAddr, apiToken string) (*v0.WorkloadDefinition, error) {
	var workloadDefinitions []v0.WorkloadDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_definitions?name=%s", apiAddr, ApiVersion, name), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}
	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	err = json.Unmarshal(jsonData, &workloadDefinitions)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	switch {
	case len(workloadDefinitions) < 1:
		return &v0.WorkloadDefinition{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(workloadDefinitions) > 1:
		return &v0.WorkloadDefinition{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &workloadDefinitions[0], nil
}

// CreateWorkloadDefinition creates a new workload definition in the Threeport API
// from a json object that contains the workload definition attributes.
func CreateWorkloadDefinition(jsonWorkloadDefinition []byte, apiAddr, apiToken string) (*v0.WorkloadDefinition, error) {
	var workloadDefinition v0.WorkloadDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_definitions", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonWorkloadDefinition), http.StatusCreated)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	err = json.Unmarshal(jsonData, &workloadDefinition)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	return &workloadDefinition, nil
}

// UpdateWorkloadDefinition updates a workload service dependency.
func UpdateWorkloadDefinition(id uint, jsonWorkloadDefinition []byte, apiAddr, apiToken string) (*v0.WorkloadDefinition, error) {
	var workloadDefinition v0.WorkloadDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_definitions/%d", apiAddr, ApiVersion, id), apiToken, http.MethodPatch, bytes.NewBuffer(jsonWorkloadDefinition), http.StatusOK)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	err = json.Unmarshal(jsonData, &workloadDefinition)
	if err != nil {
		return &v0.WorkloadDefinition{}, err
	}

	return &workloadDefinition, nil
}

// CreateWorkloadResourceDefinition creates a new workload resource definition
// in the Threeport API from a json object that contains the workload resource
// definition attributes.
func CreateWorkloadResourceDefinition(jsonWorkloadResourceDefinition []byte, apiAddr, apiToken string) (*v0.WorkloadResourceDefinition, error) {
	var workloadResourceDefinition v0.WorkloadResourceDefinition

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_resource_definitions", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonWorkloadResourceDefinition), http.StatusCreated)
	if err != nil {
		return &v0.WorkloadResourceDefinition{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.WorkloadResourceDefinition{}, err
	}

	err = json.Unmarshal(jsonData, &workloadResourceDefinition)
	if err != nil {
		return &v0.WorkloadResourceDefinition{}, err
	}

	return &workloadResourceDefinition, nil
}

// CreateWorkloadResourceDefinitions creates a new set of workload resource
// definitions in the Threeport API.
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

// GetWorkloadResourceDefinitionById fetches a workload resource definition from the
// Threeport API by workload definition ID
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

// GetWorkloadInstanceByID fetches a workload instance by ID
func GetWorkloadInstanceByID(id uint, apiAddr, apiToken string) (*v0.WorkloadInstance, error) {
	var workloadInstance v0.WorkloadInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_instances/%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &workloadInstance, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &workloadInstance, err
	}

	err = json.Unmarshal(jsonData, &workloadInstance)
	if err != nil {
		return &workloadInstance, err
	}

	return &workloadInstance, nil
}

// GetWorkloadInstancesByWorkloadClusterID fetches a workload instance by ID
func GetWorkloadInstancesByWorkloadClusterID(id uint, apiAddr, apiToken string) (*[]v0.WorkloadInstance, error) {
	var workloadInstances []v0.WorkloadInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_instances?workloadclusterid=%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &[]v0.WorkloadInstance{}, err
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &[]v0.WorkloadInstance{}, err
	}

	err = json.Unmarshal(jsonData, &workloadInstances)
	if err != nil {
		return &[]v0.WorkloadInstance{}, err
	}

	return &workloadInstances, nil
}

// GetWorkloadInstanceByName fetches a workload instance from the Threeport
// API by name.
func GetWorkloadInstanceByName(name, apiAddr, apiToken string) (*v0.WorkloadInstance, error) {
	var workloadInstances []v0.WorkloadInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_instances?name=%s", apiAddr, ApiVersion, name), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &v0.WorkloadInstance{}, err
	}
	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.WorkloadInstance{}, err
	}

	err = json.Unmarshal(jsonData, &workloadInstances)
	if err != nil {
		return &v0.WorkloadInstance{}, err
	}

	switch {
	case len(workloadInstances) < 1:
		return &v0.WorkloadInstance{}, errors.New(fmt.Sprintf("no workload instances with name %s", name))
	case len(workloadInstances) > 1:
		return &v0.WorkloadInstance{}, errors.New(fmt.Sprintf("more than one workload instance with name %s returned", name))
	}

	return &workloadInstances[0], nil
}

// CreateWorkloadInstance creates a new workload instance in the Threeport API
// from a json object that contains the workload instance attributes.
func CreateWorkloadInstance(jsonWorkloadInstance []byte, apiAddr, apiToken string) (*v0.WorkloadInstance, error) {
	var workloadInstance v0.WorkloadInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_instances", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonWorkloadInstance), http.StatusCreated)
	if err != nil {
		return &v0.WorkloadInstance{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.WorkloadInstance{}, err
	}

	err = json.Unmarshal(jsonData, &workloadInstance)
	if err != nil {
		return &v0.WorkloadInstance{}, err
	}

	return &workloadInstance, nil
}

//// GetWorkloadServiceDependencyByName fetches a workload service dependency from the Threeport
//// API by name.
//func GetWorkloadServiceDependencyByName(name, apiAddr, apiToken string) (*v0.WorkloadServiceDependency, error) {
//	var workloadServiceDependencies []v0.WorkloadServiceDependency
//
//	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_service_dependencies?name=%s", apiAddr, ApiVersion, name), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//	jsonData, err := json.Marshal(response.Data)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	err = json.Unmarshal(jsonData, &workloadServiceDependencies)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	switch {
//	case len(workloadServiceDependencies) < 1:
//		return &v0.WorkloadServiceDependency{}, errors.New(fmt.Sprintf("no workload service dependencies with name %s", name))
//	case len(workloadServiceDependencies) > 1:
//		return &v0.WorkloadServiceDependency{}, errors.New(fmt.Sprintf("more than one workload service dependency with name %s returned", name))
//	}
//
//	return &workloadServiceDependencies[0], nil
//}
//
//// GetWorkloadServiceDependencyByID fetches a workload service dependency by ID
//func GetWorkloadServiceDependencyByID(id uint, apiAddr, apiToken string) (*v0.WorkloadServiceDependency, error) {
//	var workloadServiceDependency v0.WorkloadServiceDependency
//
//	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_service_dependencies/%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
//	if err != nil {
//		return &workloadServiceDependency, err
//	}
//
//	jsonData, err := json.Marshal(response.Data[0])
//	if err != nil {
//		return &workloadServiceDependency, err
//	}
//
//	err = json.Unmarshal(jsonData, &workloadServiceDependency)
//	if err != nil {
//		return &workloadServiceDependency, err
//	}
//
//	return &workloadServiceDependency, nil
//}
//
//// CreateWorkloadServiceDependency creates a new workload service dependency in the Threeport API
//// from a json object that contains the workload service dependency attributes.
//func CreateWorkloadServiceDependency(jsonWorkloadServiceDependency []byte, apiAddr, apiToken string) (*v0.WorkloadServiceDependency, error) {
//	var workloadServiceDependency v0.WorkloadServiceDependency
//
//	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_service_dependencies", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonWorkloadServiceDependency), http.StatusCreated)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	jsonData, err := json.Marshal(response.Data[0])
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	err = json.Unmarshal(jsonData, &workloadServiceDependency)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	return &workloadServiceDependency, nil
//}
//
//// UpdateWorkloadServiceDependency updates a workload service dependency.
//func UpdateWorkloadServiceDependency(id uint, jsonWorkloadServiceDependency []byte, apiAddr, apiToken string) (*v0.WorkloadServiceDependency, error) {
//	var workloadServiceDependency v0.WorkloadServiceDependency
//
//	response, err := GetResponse(fmt.Sprintf("%s/%s/workload_service_dependencies/%d", apiAddr, ApiVersion, id), apiToken, http.MethodPatch, bytes.NewBuffer(jsonWorkloadServiceDependency), http.StatusOK)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	jsonData, err := json.Marshal(response.Data[0])
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	err = json.Unmarshal(jsonData, &workloadServiceDependency)
//	if err != nil {
//		return &v0.WorkloadServiceDependency{}, err
//	}
//
//	return &workloadServiceDependency, nil
//}
