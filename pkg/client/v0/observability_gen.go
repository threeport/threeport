// generated by 'threeport-codegen api-model' - do not edit

package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"net/http"
)

// GetMetricsDefinitions fetches all metrics definitions.
// TODO: implement pagination
func GetMetricsDefinitions(apiClient *http.Client, apiAddr string) (*[]v0.MetricsDefinition, error) {
	var metricsDefinitions []v0.MetricsDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &metricsDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsDefinitions, nil
}

// GetMetricsDefinitionByID fetches a metrics definition by ID.
func GetMetricsDefinitionByID(apiClient *http.Client, apiAddr string, id uint) (*v0.MetricsDefinition, error) {
	var metricsDefinition v0.MetricsDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions/%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &metricsDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsDefinition, nil
}

// GetMetricsDefinitionsByQueryString fetches metrics definitions by provided query string.
func GetMetricsDefinitionsByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.MetricsDefinition, error) {
	var metricsDefinitions []v0.MetricsDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions?%s", apiAddr, ApiVersion, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &metricsDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsDefinitions, nil
}

// GetMetricsDefinitionByName fetches a metrics definition by name.
func GetMetricsDefinitionByName(apiClient *http.Client, apiAddr, name string) (*v0.MetricsDefinition, error) {
	var metricsDefinitions []v0.MetricsDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions?name=%s", apiAddr, ApiVersion, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.MetricsDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.MetricsDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(metricsDefinitions) < 1:
		return &v0.MetricsDefinition{}, errors.New(fmt.Sprintf("no metrics definition with name %s", name))
	case len(metricsDefinitions) > 1:
		return &v0.MetricsDefinition{}, errors.New(fmt.Sprintf("more than one metrics definition with name %s returned", name))
	}

	return &metricsDefinitions[0], nil
}

// CreateMetricsDefinition creates a new metrics definition.
func CreateMetricsDefinition(apiClient *http.Client, apiAddr string, metricsDefinition *v0.MetricsDefinition) (*v0.MetricsDefinition, error) {
	ReplaceAssociatedObjectsWithNil(metricsDefinition)
	jsonMetricsDefinition, err := util.MarshalObject(metricsDefinition)
	if err != nil {
		return metricsDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions", apiAddr, ApiVersion),
		http.MethodPost,
		bytes.NewBuffer(jsonMetricsDefinition),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return metricsDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return metricsDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return metricsDefinition, nil
}

// UpdateMetricsDefinition updates a metrics definition.
func UpdateMetricsDefinition(apiClient *http.Client, apiAddr string, metricsDefinition *v0.MetricsDefinition) (*v0.MetricsDefinition, error) {
	ReplaceAssociatedObjectsWithNil(metricsDefinition)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	metricsDefinitionID := *metricsDefinition.ID
	payloadMetricsDefinition := *metricsDefinition
	payloadMetricsDefinition.ID = nil
	payloadMetricsDefinition.CreatedAt = nil
	payloadMetricsDefinition.UpdatedAt = nil

	jsonMetricsDefinition, err := util.MarshalObject(payloadMetricsDefinition)
	if err != nil {
		return metricsDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions/%d", apiAddr, ApiVersion, metricsDefinitionID),
		http.MethodPatch,
		bytes.NewBuffer(jsonMetricsDefinition),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return metricsDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return metricsDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadMetricsDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadMetricsDefinition.ID = &metricsDefinitionID
	return &payloadMetricsDefinition, nil
}

// DeleteMetricsDefinition deletes a metrics definition by ID.
func DeleteMetricsDefinition(apiClient *http.Client, apiAddr string, id uint) (*v0.MetricsDefinition, error) {
	var metricsDefinition v0.MetricsDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-definitions/%d", apiAddr, ApiVersion, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &metricsDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsDefinition, nil
}

// GetMetricsInstances fetches all metrics instances.
// TODO: implement pagination
func GetMetricsInstances(apiClient *http.Client, apiAddr string) (*[]v0.MetricsInstance, error) {
	var metricsInstances []v0.MetricsInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &metricsInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsInstances, nil
}

// GetMetricsInstanceByID fetches a metrics instance by ID.
func GetMetricsInstanceByID(apiClient *http.Client, apiAddr string, id uint) (*v0.MetricsInstance, error) {
	var metricsInstance v0.MetricsInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances/%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &metricsInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsInstance, nil
}

// GetMetricsInstancesByQueryString fetches metrics instances by provided query string.
func GetMetricsInstancesByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.MetricsInstance, error) {
	var metricsInstances []v0.MetricsInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances?%s", apiAddr, ApiVersion, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &metricsInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsInstances, nil
}

// GetMetricsInstanceByName fetches a metrics instance by name.
func GetMetricsInstanceByName(apiClient *http.Client, apiAddr, name string) (*v0.MetricsInstance, error) {
	var metricsInstances []v0.MetricsInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances?name=%s", apiAddr, ApiVersion, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.MetricsInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.MetricsInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(metricsInstances) < 1:
		return &v0.MetricsInstance{}, errors.New(fmt.Sprintf("no metrics instance with name %s", name))
	case len(metricsInstances) > 1:
		return &v0.MetricsInstance{}, errors.New(fmt.Sprintf("more than one metrics instance with name %s returned", name))
	}

	return &metricsInstances[0], nil
}

// CreateMetricsInstance creates a new metrics instance.
func CreateMetricsInstance(apiClient *http.Client, apiAddr string, metricsInstance *v0.MetricsInstance) (*v0.MetricsInstance, error) {
	ReplaceAssociatedObjectsWithNil(metricsInstance)
	jsonMetricsInstance, err := util.MarshalObject(metricsInstance)
	if err != nil {
		return metricsInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances", apiAddr, ApiVersion),
		http.MethodPost,
		bytes.NewBuffer(jsonMetricsInstance),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return metricsInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return metricsInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return metricsInstance, nil
}

// UpdateMetricsInstance updates a metrics instance.
func UpdateMetricsInstance(apiClient *http.Client, apiAddr string, metricsInstance *v0.MetricsInstance) (*v0.MetricsInstance, error) {
	ReplaceAssociatedObjectsWithNil(metricsInstance)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	metricsInstanceID := *metricsInstance.ID
	payloadMetricsInstance := *metricsInstance
	payloadMetricsInstance.ID = nil
	payloadMetricsInstance.CreatedAt = nil
	payloadMetricsInstance.UpdatedAt = nil

	jsonMetricsInstance, err := util.MarshalObject(payloadMetricsInstance)
	if err != nil {
		return metricsInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances/%d", apiAddr, ApiVersion, metricsInstanceID),
		http.MethodPatch,
		bytes.NewBuffer(jsonMetricsInstance),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return metricsInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return metricsInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadMetricsInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadMetricsInstance.ID = &metricsInstanceID
	return &payloadMetricsInstance, nil
}

// DeleteMetricsInstance deletes a metrics instance by ID.
func DeleteMetricsInstance(apiClient *http.Client, apiAddr string, id uint) (*v0.MetricsInstance, error) {
	var metricsInstance v0.MetricsInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/metrics-instances/%d", apiAddr, ApiVersion, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &metricsInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &metricsInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&metricsInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &metricsInstance, nil
}