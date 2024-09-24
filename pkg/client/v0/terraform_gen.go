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

// GetTerraformDefinitions fetches all terraform definitions.
// TODO: implement pagination
func GetTerraformDefinitions(apiClient *http.Client, apiAddr string) (*[]v0.TerraformDefinition, error) {
	var terraformDefinitions []v0.TerraformDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathTerraformDefinitions),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &terraformDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformDefinitions, nil
}

// GetTerraformDefinitionByID fetches a terraform definition by ID.
func GetTerraformDefinitionByID(apiClient *http.Client, apiAddr string, id uint) (*v0.TerraformDefinition, error) {
	var terraformDefinition v0.TerraformDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathTerraformDefinitions, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &terraformDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformDefinition, nil
}

// GetTerraformDefinitionsByQueryString fetches terraform definitions by provided query string.
func GetTerraformDefinitionsByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.TerraformDefinition, error) {
	var terraformDefinitions []v0.TerraformDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathTerraformDefinitions, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &terraformDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformDefinitions, nil
}

// GetTerraformDefinitionByName fetches a terraform definition by name.
func GetTerraformDefinitionByName(apiClient *http.Client, apiAddr, name string) (*v0.TerraformDefinition, error) {
	var terraformDefinitions []v0.TerraformDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathTerraformDefinitions, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.TerraformDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.TerraformDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(terraformDefinitions) < 1:
		return &v0.TerraformDefinition{}, errors.New(fmt.Sprintf("no terraform definition with name %s", name))
	case len(terraformDefinitions) > 1:
		return &v0.TerraformDefinition{}, errors.New(fmt.Sprintf("more than one terraform definition with name %s returned", name))
	}

	return &terraformDefinitions[0], nil
}

// CreateTerraformDefinition creates a new terraform definition.
func CreateTerraformDefinition(apiClient *http.Client, apiAddr string, terraformDefinition *v0.TerraformDefinition) (*v0.TerraformDefinition, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(terraformDefinition)
	jsonTerraformDefinition, err := util.MarshalObject(terraformDefinition)
	if err != nil {
		return terraformDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathTerraformDefinitions),
		http.MethodPost,
		bytes.NewBuffer(jsonTerraformDefinition),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return terraformDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return terraformDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return terraformDefinition, nil
}

// UpdateTerraformDefinition updates a terraform definition.
func UpdateTerraformDefinition(apiClient *http.Client, apiAddr string, terraformDefinition *v0.TerraformDefinition) (*v0.TerraformDefinition, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(terraformDefinition)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	terraformDefinitionID := *terraformDefinition.ID
	payloadTerraformDefinition := *terraformDefinition
	payloadTerraformDefinition.ID = nil
	payloadTerraformDefinition.CreatedAt = nil
	payloadTerraformDefinition.UpdatedAt = nil

	jsonTerraformDefinition, err := util.MarshalObject(payloadTerraformDefinition)
	if err != nil {
		return terraformDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathTerraformDefinitions, terraformDefinitionID),
		http.MethodPatch,
		bytes.NewBuffer(jsonTerraformDefinition),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return terraformDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return terraformDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadTerraformDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadTerraformDefinition.ID = &terraformDefinitionID
	return &payloadTerraformDefinition, nil
}

// DeleteTerraformDefinition deletes a terraform definition by ID.
func DeleteTerraformDefinition(apiClient *http.Client, apiAddr string, id uint) (*v0.TerraformDefinition, error) {
	var terraformDefinition v0.TerraformDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathTerraformDefinitions, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &terraformDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformDefinition, nil
}

// GetTerraformInstances fetches all terraform instances.
// TODO: implement pagination
func GetTerraformInstances(apiClient *http.Client, apiAddr string) (*[]v0.TerraformInstance, error) {
	var terraformInstances []v0.TerraformInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathTerraformInstances),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &terraformInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformInstances, nil
}

// GetTerraformInstanceByID fetches a terraform instance by ID.
func GetTerraformInstanceByID(apiClient *http.Client, apiAddr string, id uint) (*v0.TerraformInstance, error) {
	var terraformInstance v0.TerraformInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathTerraformInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &terraformInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformInstance, nil
}

// GetTerraformInstancesByQueryString fetches terraform instances by provided query string.
func GetTerraformInstancesByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.TerraformInstance, error) {
	var terraformInstances []v0.TerraformInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathTerraformInstances, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &terraformInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformInstances, nil
}

// GetTerraformInstanceByName fetches a terraform instance by name.
func GetTerraformInstanceByName(apiClient *http.Client, apiAddr, name string) (*v0.TerraformInstance, error) {
	var terraformInstances []v0.TerraformInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathTerraformInstances, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.TerraformInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.TerraformInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(terraformInstances) < 1:
		return &v0.TerraformInstance{}, errors.New(fmt.Sprintf("no terraform instance with name %s", name))
	case len(terraformInstances) > 1:
		return &v0.TerraformInstance{}, errors.New(fmt.Sprintf("more than one terraform instance with name %s returned", name))
	}

	return &terraformInstances[0], nil
}

// CreateTerraformInstance creates a new terraform instance.
func CreateTerraformInstance(apiClient *http.Client, apiAddr string, terraformInstance *v0.TerraformInstance) (*v0.TerraformInstance, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(terraformInstance)
	jsonTerraformInstance, err := util.MarshalObject(terraformInstance)
	if err != nil {
		return terraformInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathTerraformInstances),
		http.MethodPost,
		bytes.NewBuffer(jsonTerraformInstance),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return terraformInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return terraformInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return terraformInstance, nil
}

// UpdateTerraformInstance updates a terraform instance.
func UpdateTerraformInstance(apiClient *http.Client, apiAddr string, terraformInstance *v0.TerraformInstance) (*v0.TerraformInstance, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(terraformInstance)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	terraformInstanceID := *terraformInstance.ID
	payloadTerraformInstance := *terraformInstance
	payloadTerraformInstance.ID = nil
	payloadTerraformInstance.CreatedAt = nil
	payloadTerraformInstance.UpdatedAt = nil

	jsonTerraformInstance, err := util.MarshalObject(payloadTerraformInstance)
	if err != nil {
		return terraformInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathTerraformInstances, terraformInstanceID),
		http.MethodPatch,
		bytes.NewBuffer(jsonTerraformInstance),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return terraformInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return terraformInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadTerraformInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadTerraformInstance.ID = &terraformInstanceID
	return &payloadTerraformInstance, nil
}

// DeleteTerraformInstance deletes a terraform instance by ID.
func DeleteTerraformInstance(apiClient *http.Client, apiAddr string, id uint) (*v0.TerraformInstance, error) {
	var terraformInstance v0.TerraformInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathTerraformInstances, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &terraformInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &terraformInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&terraformInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &terraformInstance, nil
}
