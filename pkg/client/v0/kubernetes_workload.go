package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// CreateKubernetesWorkloadResourceDefinitions creates a new set of kubernetes
// workload resource definitions.
func CreateKubernetesWorkloadResourceDefinitions(
	apiClient *http.Client,
	apiAddr string,
	workloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition,
) (*[]v0.KubernetesWorkloadResourceDefinition, error) {
	jsonWorkloadResourceDefinitions, err := util.MarshalObject(workloadResourceDefinitions)
	if err != nil {
		return workloadResourceDefinitions, fmt.Errorf("failed to marshal provided objects to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathKubernetesWorkloadResourceDefinitionSets),
		http.MethodPost,
		bytes.NewBuffer(jsonWorkloadResourceDefinitions),
		map[string]string{},
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

// GetKubernetesWorkloadResourceDefinitionsByID fetches kubernetes workload
// resource definitions by kubernetes workload definition ID.
func GetKubernetesWorkloadResourceDefinitionsByID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.KubernetesWorkloadResourceDefinition, error) {
	var workloadResourceDefinitions []v0.KubernetesWorkloadResourceDefinition

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?kubernetesworkloaddefinitionid=%d", apiAddr, v0.PathKubernetesWorkloadResourceDefinitions, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?kubernetesworkloaddefinitionid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathKubernetesWorkloadResourceDefinitions, id, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &workloadResourceDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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

// GetKubernetesWorkloadInstancesByID fetches kubernetes workload instances
// by kubernetes workload definition ID.
func GetKubernetesWorkloadInstancesByID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.KubernetesWorkloadInstance, error) {
	var workloadInstances []v0.KubernetesWorkloadInstance

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?kubernetesworkloaddefinitionid=%d", apiAddr, v0.PathKubernetesWorkloadInstances, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?kubernetesworkloaddefinitionid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathKubernetesWorkloadInstances, id, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &workloadInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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

// GetKubernetesWorkloadResourceInstancesByID fetches kubernetes workload
// resource instances by kubernetes workload instance ID.
func GetKubernetesWorkloadResourceInstancesByID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.KubernetesWorkloadResourceInstance, error) {
	var workloadResourceInstances []v0.KubernetesWorkloadResourceInstance

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?kubernetesworkloadinstanceid=%d", apiAddr, v0.PathKubernetesWorkloadResourceInstances, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?kubernetesworkloadinstanceid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathKubernetesWorkloadResourceInstances, id, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &workloadResourceInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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

// GetKubernetesWorkloadInstancesByKubernetesRuntimeInstanceID fetches kubernetes
// workload instances by kubernetes runtime instance ID.
func GetKubernetesWorkloadInstancesByKubernetesRuntimeInstanceID(apiClient *http.Client, apiAddr string, kubernetesRuntimeID uint) (*[]v0.KubernetesWorkloadInstance, error) {
	var workloadInstances []v0.KubernetesWorkloadInstance

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?kubernetesruntimeinstanceid=%d", apiAddr, v0.PathKubernetesWorkloadInstances, kubernetesRuntimeID)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?kubernetesruntimeinstanceid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathKubernetesWorkloadInstances, kubernetesRuntimeID, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &workloadInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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
