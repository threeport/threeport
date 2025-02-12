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

// GetLogBackends fetches all log backends.
// TODO: implement pagination
func GetLogBackends(apiClient *http.Client, apiAddr string) (*[]v0.LogBackend, error) {
	var logBackends []v0.LogBackend

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathLogBackends),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logBackends, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &logBackends, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logBackends); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logBackends, nil
}

// GetLogBackendByID fetches a log backend by ID.
func GetLogBackendByID(apiClient *http.Client, apiAddr string, id uint) (*v0.LogBackend, error) {
	var logBackend v0.LogBackend

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogBackends, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logBackend, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &logBackend, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logBackend); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logBackend, nil
}

// GetLogBackendsByQueryString fetches log backends by provided query string.
func GetLogBackendsByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.LogBackend, error) {
	var logBackends []v0.LogBackend

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathLogBackends, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logBackends, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &logBackends, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logBackends); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logBackends, nil
}

// GetLogBackendByName fetches a log backend by name.
func GetLogBackendByName(apiClient *http.Client, apiAddr, name string) (*v0.LogBackend, error) {
	var logBackends []v0.LogBackend

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathLogBackends, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.LogBackend{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.LogBackend{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logBackends); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(logBackends) < 1:
		return &v0.LogBackend{}, errors.New(fmt.Sprintf("no log backend with name %s", name))
	case len(logBackends) > 1:
		return &v0.LogBackend{}, errors.New(fmt.Sprintf("more than one log backend with name %s returned", name))
	}

	return &logBackends[0], nil
}

// CreateLogBackend creates a new log backend.
func CreateLogBackend(apiClient *http.Client, apiAddr string, logBackend *v0.LogBackend) (*v0.LogBackend, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(logBackend)
	jsonLogBackend, err := util.MarshalObject(logBackend)
	if err != nil {
		return logBackend, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathLogBackends),
		http.MethodPost,
		bytes.NewBuffer(jsonLogBackend),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return logBackend, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return logBackend, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logBackend); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return logBackend, nil
}

// UpdateLogBackend updates a log backend.
func UpdateLogBackend(apiClient *http.Client, apiAddr string, logBackend *v0.LogBackend) (*v0.LogBackend, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(logBackend)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	logBackendID := *logBackend.ID
	payloadLogBackend := *logBackend
	payloadLogBackend.ID = nil
	payloadLogBackend.CreatedAt = nil
	payloadLogBackend.UpdatedAt = nil

	jsonLogBackend, err := util.MarshalObject(payloadLogBackend)
	if err != nil {
		return logBackend, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogBackends, logBackendID),
		http.MethodPatch,
		bytes.NewBuffer(jsonLogBackend),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return logBackend, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return logBackend, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadLogBackend); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadLogBackend.ID = &logBackendID
	return &payloadLogBackend, nil
}

// DeleteLogBackend deletes a log backend by ID.
func DeleteLogBackend(apiClient *http.Client, apiAddr string, id uint) (*v0.LogBackend, error) {
	var logBackend v0.LogBackend

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogBackends, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logBackend, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &logBackend, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logBackend); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logBackend, nil
}

// GetLogStorageDefinitions fetches all log storage definitions.
// TODO: implement pagination
func GetLogStorageDefinitions(apiClient *http.Client, apiAddr string) (*[]v0.LogStorageDefinition, error) {
	var logStorageDefinitions []v0.LogStorageDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathLogStorageDefinitions),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &logStorageDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageDefinitions, nil
}

// GetLogStorageDefinitionByID fetches a log storage definition by ID.
func GetLogStorageDefinitionByID(apiClient *http.Client, apiAddr string, id uint) (*v0.LogStorageDefinition, error) {
	var logStorageDefinition v0.LogStorageDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogStorageDefinitions, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &logStorageDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageDefinition, nil
}

// GetLogStorageDefinitionsByQueryString fetches log storage definitions by provided query string.
func GetLogStorageDefinitionsByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.LogStorageDefinition, error) {
	var logStorageDefinitions []v0.LogStorageDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathLogStorageDefinitions, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &logStorageDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageDefinitions, nil
}

// GetLogStorageDefinitionByName fetches a log storage definition by name.
func GetLogStorageDefinitionByName(apiClient *http.Client, apiAddr, name string) (*v0.LogStorageDefinition, error) {
	var logStorageDefinitions []v0.LogStorageDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathLogStorageDefinitions, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.LogStorageDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.LogStorageDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(logStorageDefinitions) < 1:
		return &v0.LogStorageDefinition{}, errors.New(fmt.Sprintf("no log storage definition with name %s", name))
	case len(logStorageDefinitions) > 1:
		return &v0.LogStorageDefinition{}, errors.New(fmt.Sprintf("more than one log storage definition with name %s returned", name))
	}

	return &logStorageDefinitions[0], nil
}

// CreateLogStorageDefinition creates a new log storage definition.
func CreateLogStorageDefinition(apiClient *http.Client, apiAddr string, logStorageDefinition *v0.LogStorageDefinition) (*v0.LogStorageDefinition, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(logStorageDefinition)
	jsonLogStorageDefinition, err := util.MarshalObject(logStorageDefinition)
	if err != nil {
		return logStorageDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathLogStorageDefinitions),
		http.MethodPost,
		bytes.NewBuffer(jsonLogStorageDefinition),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return logStorageDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return logStorageDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return logStorageDefinition, nil
}

// UpdateLogStorageDefinition updates a log storage definition.
func UpdateLogStorageDefinition(apiClient *http.Client, apiAddr string, logStorageDefinition *v0.LogStorageDefinition) (*v0.LogStorageDefinition, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(logStorageDefinition)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	logStorageDefinitionID := *logStorageDefinition.ID
	payloadLogStorageDefinition := *logStorageDefinition
	payloadLogStorageDefinition.ID = nil
	payloadLogStorageDefinition.CreatedAt = nil
	payloadLogStorageDefinition.UpdatedAt = nil

	jsonLogStorageDefinition, err := util.MarshalObject(payloadLogStorageDefinition)
	if err != nil {
		return logStorageDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogStorageDefinitions, logStorageDefinitionID),
		http.MethodPatch,
		bytes.NewBuffer(jsonLogStorageDefinition),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return logStorageDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return logStorageDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadLogStorageDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadLogStorageDefinition.ID = &logStorageDefinitionID
	return &payloadLogStorageDefinition, nil
}

// DeleteLogStorageDefinition deletes a log storage definition by ID.
func DeleteLogStorageDefinition(apiClient *http.Client, apiAddr string, id uint) (*v0.LogStorageDefinition, error) {
	var logStorageDefinition v0.LogStorageDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogStorageDefinitions, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &logStorageDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageDefinition, nil
}

// GetLogStorageInstances fetches all log storage instances.
// TODO: implement pagination
func GetLogStorageInstances(apiClient *http.Client, apiAddr string) (*[]v0.LogStorageInstance, error) {
	var logStorageInstances []v0.LogStorageInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathLogStorageInstances),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &logStorageInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageInstances, nil
}

// GetLogStorageInstanceByID fetches a log storage instance by ID.
func GetLogStorageInstanceByID(apiClient *http.Client, apiAddr string, id uint) (*v0.LogStorageInstance, error) {
	var logStorageInstance v0.LogStorageInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogStorageInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &logStorageInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageInstance, nil
}

// GetLogStorageInstancesByQueryString fetches log storage instances by provided query string.
func GetLogStorageInstancesByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.LogStorageInstance, error) {
	var logStorageInstances []v0.LogStorageInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathLogStorageInstances, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &logStorageInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageInstances, nil
}

// GetLogStorageInstanceByName fetches a log storage instance by name.
func GetLogStorageInstanceByName(apiClient *http.Client, apiAddr, name string) (*v0.LogStorageInstance, error) {
	var logStorageInstances []v0.LogStorageInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathLogStorageInstances, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.LogStorageInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.LogStorageInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(logStorageInstances) < 1:
		return &v0.LogStorageInstance{}, errors.New(fmt.Sprintf("no log storage instance with name %s", name))
	case len(logStorageInstances) > 1:
		return &v0.LogStorageInstance{}, errors.New(fmt.Sprintf("more than one log storage instance with name %s returned", name))
	}

	return &logStorageInstances[0], nil
}

// CreateLogStorageInstance creates a new log storage instance.
func CreateLogStorageInstance(apiClient *http.Client, apiAddr string, logStorageInstance *v0.LogStorageInstance) (*v0.LogStorageInstance, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(logStorageInstance)
	jsonLogStorageInstance, err := util.MarshalObject(logStorageInstance)
	if err != nil {
		return logStorageInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathLogStorageInstances),
		http.MethodPost,
		bytes.NewBuffer(jsonLogStorageInstance),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return logStorageInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return logStorageInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return logStorageInstance, nil
}

// UpdateLogStorageInstance updates a log storage instance.
func UpdateLogStorageInstance(apiClient *http.Client, apiAddr string, logStorageInstance *v0.LogStorageInstance) (*v0.LogStorageInstance, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(logStorageInstance)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	logStorageInstanceID := *logStorageInstance.ID
	payloadLogStorageInstance := *logStorageInstance
	payloadLogStorageInstance.ID = nil
	payloadLogStorageInstance.CreatedAt = nil
	payloadLogStorageInstance.UpdatedAt = nil

	jsonLogStorageInstance, err := util.MarshalObject(payloadLogStorageInstance)
	if err != nil {
		return logStorageInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogStorageInstances, logStorageInstanceID),
		http.MethodPatch,
		bytes.NewBuffer(jsonLogStorageInstance),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return logStorageInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return logStorageInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadLogStorageInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadLogStorageInstance.ID = &logStorageInstanceID
	return &payloadLogStorageInstance, nil
}

// DeleteLogStorageInstance deletes a log storage instance by ID.
func DeleteLogStorageInstance(apiClient *http.Client, apiAddr string, id uint) (*v0.LogStorageInstance, error) {
	var logStorageInstance v0.LogStorageInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathLogStorageInstances, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &logStorageInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &logStorageInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&logStorageInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &logStorageInstance, nil
}
