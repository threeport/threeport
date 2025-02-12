// generated by 'threeport-sdk gen' - do not edit

package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"net/http"
)

// GetKubernetesRuntimeDefinitions fetches all kubernetes runtime definitions.
// TODO: implement pagination
func GetKubernetesRuntimeDefinitions(apiClient *http.Client, apiAddr string) (*[]v0.KubernetesRuntimeDefinition, error) {
	var kubernetesRuntimeDefinitions []v0.KubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathKubernetesRuntimeDefinitions),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &kubernetesRuntimeDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeDefinitions, nil
}

// GetKubernetesRuntimeDefinitionByID fetches a kubernetes runtime definition by ID.
func GetKubernetesRuntimeDefinitionByID(apiClient *http.Client, apiAddr string, id uint) (*v0.KubernetesRuntimeDefinition, error) {
	var kubernetesRuntimeDefinition v0.KubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathKubernetesRuntimeDefinitions, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &kubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeDefinition, nil
}

// GetKubernetesRuntimeDefinitionsByQueryString fetches kubernetes runtime definitions by provided query string.
func GetKubernetesRuntimeDefinitionsByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.KubernetesRuntimeDefinition, error) {
	var kubernetesRuntimeDefinitions []v0.KubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathKubernetesRuntimeDefinitions, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &kubernetesRuntimeDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeDefinitions, nil
}

// GetKubernetesRuntimeDefinitionByName fetches a kubernetes runtime definition by name.
func GetKubernetesRuntimeDefinitionByName(apiClient *http.Client, apiAddr, name string) (*v0.KubernetesRuntimeDefinition, error) {
	var kubernetesRuntimeDefinitions []v0.KubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathKubernetesRuntimeDefinitions, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.KubernetesRuntimeDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.KubernetesRuntimeDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(kubernetesRuntimeDefinitions) < 1:
		return &v0.KubernetesRuntimeDefinition{}, errors.New(fmt.Sprintf("no kubernetes runtime definition with name %s", name))
	case len(kubernetesRuntimeDefinitions) > 1:
		return &v0.KubernetesRuntimeDefinition{}, errors.New(fmt.Sprintf("more than one kubernetes runtime definition with name %s returned", name))
	}

	return &kubernetesRuntimeDefinitions[0], nil
}

// CreateKubernetesRuntimeDefinition creates a new kubernetes runtime definition.
func CreateKubernetesRuntimeDefinition(apiClient *http.Client, apiAddr string, kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition) (*v0.KubernetesRuntimeDefinition, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(kubernetesRuntimeDefinition)
	jsonKubernetesRuntimeDefinition, err := util.MarshalObject(kubernetesRuntimeDefinition)
	if err != nil {
		return kubernetesRuntimeDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathKubernetesRuntimeDefinitions),
		http.MethodPost,
		bytes.NewBuffer(jsonKubernetesRuntimeDefinition),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return kubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return kubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return kubernetesRuntimeDefinition, nil
}

// UpdateKubernetesRuntimeDefinition updates a kubernetes runtime definition.
func UpdateKubernetesRuntimeDefinition(apiClient *http.Client, apiAddr string, kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition) (*v0.KubernetesRuntimeDefinition, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(kubernetesRuntimeDefinition)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	kubernetesRuntimeDefinitionID := *kubernetesRuntimeDefinition.ID
	payloadKubernetesRuntimeDefinition := *kubernetesRuntimeDefinition
	payloadKubernetesRuntimeDefinition.ID = nil
	payloadKubernetesRuntimeDefinition.CreatedAt = nil
	payloadKubernetesRuntimeDefinition.UpdatedAt = nil

	jsonKubernetesRuntimeDefinition, err := util.MarshalObject(payloadKubernetesRuntimeDefinition)
	if err != nil {
		return kubernetesRuntimeDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathKubernetesRuntimeDefinitions, kubernetesRuntimeDefinitionID),
		http.MethodPatch,
		bytes.NewBuffer(jsonKubernetesRuntimeDefinition),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return kubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return kubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadKubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadKubernetesRuntimeDefinition.ID = &kubernetesRuntimeDefinitionID
	return &payloadKubernetesRuntimeDefinition, nil
}

// DeleteKubernetesRuntimeDefinition deletes a kubernetes runtime definition by ID.
func DeleteKubernetesRuntimeDefinition(apiClient *http.Client, apiAddr string, id uint) (*v0.KubernetesRuntimeDefinition, error) {
	var kubernetesRuntimeDefinition v0.KubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathKubernetesRuntimeDefinitions, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &kubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeDefinition, nil
}

// GetKubernetesRuntimeInstances fetches all kubernetes runtime instances.
// TODO: implement pagination
func GetKubernetesRuntimeInstances(apiClient *http.Client, apiAddr string) (*[]v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstances []v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathKubernetesRuntimeInstances),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &kubernetesRuntimeInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeInstances, nil
}

// GetKubernetesRuntimeInstanceByID fetches a kubernetes runtime instance by ID.
func GetKubernetesRuntimeInstanceByID(apiClient *http.Client, apiAddr string, id uint) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathKubernetesRuntimeInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &kubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeInstance, nil
}

// GetKubernetesRuntimeInstancesByQueryString fetches kubernetes runtime instances by provided query string.
func GetKubernetesRuntimeInstancesByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstances []v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathKubernetesRuntimeInstances, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &kubernetesRuntimeInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeInstances, nil
}

// GetKubernetesRuntimeInstanceByName fetches a kubernetes runtime instance by name.
func GetKubernetesRuntimeInstanceByName(apiClient *http.Client, apiAddr, name string) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstances []v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathKubernetesRuntimeInstances, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.KubernetesRuntimeInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.KubernetesRuntimeInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(kubernetesRuntimeInstances) < 1:
		return &v0.KubernetesRuntimeInstance{}, errors.New(fmt.Sprintf("no kubernetes runtime instance with name %s", name))
	case len(kubernetesRuntimeInstances) > 1:
		return &v0.KubernetesRuntimeInstance{}, errors.New(fmt.Sprintf("more than one kubernetes runtime instance with name %s returned", name))
	}

	return &kubernetesRuntimeInstances[0], nil
}

// CreateKubernetesRuntimeInstance creates a new kubernetes runtime instance.
func CreateKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string, kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance) (*v0.KubernetesRuntimeInstance, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(kubernetesRuntimeInstance)
	jsonKubernetesRuntimeInstance, err := util.MarshalObject(kubernetesRuntimeInstance)
	if err != nil {
		return kubernetesRuntimeInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathKubernetesRuntimeInstances),
		http.MethodPost,
		bytes.NewBuffer(jsonKubernetesRuntimeInstance),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return kubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return kubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return kubernetesRuntimeInstance, nil
}

// UpdateKubernetesRuntimeInstance updates a kubernetes runtime instance.
func UpdateKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string, kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance) (*v0.KubernetesRuntimeInstance, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(kubernetesRuntimeInstance)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	kubernetesRuntimeInstanceID := *kubernetesRuntimeInstance.ID
	payloadKubernetesRuntimeInstance := *kubernetesRuntimeInstance
	payloadKubernetesRuntimeInstance.ID = nil
	payloadKubernetesRuntimeInstance.CreatedAt = nil
	payloadKubernetesRuntimeInstance.UpdatedAt = nil

	jsonKubernetesRuntimeInstance, err := util.MarshalObject(payloadKubernetesRuntimeInstance)
	if err != nil {
		return kubernetesRuntimeInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathKubernetesRuntimeInstances, kubernetesRuntimeInstanceID),
		http.MethodPatch,
		bytes.NewBuffer(jsonKubernetesRuntimeInstance),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return kubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return kubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadKubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadKubernetesRuntimeInstance.ID = &kubernetesRuntimeInstanceID
	return &payloadKubernetesRuntimeInstance, nil
}

// DeleteKubernetesRuntimeInstance deletes a kubernetes runtime instance by ID.
func DeleteKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string, id uint) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathKubernetesRuntimeInstances, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &kubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&kubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &kubernetesRuntimeInstance, nil
}
