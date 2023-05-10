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

// GetDomainNameDefinitions fetches all domain name definitions.
// TODO: implement pagination
func GetDomainNameDefinitions(apiClient *http.Client, apiAddr string) (*[]v0.DomainNameDefinition, error) {
	var domainNameDefinitions []v0.DomainNameDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-definitions", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &domainNameDefinitions, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &domainNameDefinitions, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &domainNameDefinitions, nil
}

// GetDomainNameDefinitionByID fetches a domain name definition by ID.
func GetDomainNameDefinitionByID(apiClient *http.Client, apiAddr string, id uint) (*v0.DomainNameDefinition, error) {
	var domainNameDefinition v0.DomainNameDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-definitions/%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &domainNameDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &domainNameDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &domainNameDefinition, nil
}

// GetDomainNameDefinitionByName fetches a domain name definition by name.
func GetDomainNameDefinitionByName(apiClient *http.Client, apiAddr, name string) (*v0.DomainNameDefinition, error) {
	var domainNameDefinitions []v0.DomainNameDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-definitions?name=%s", apiAddr, ApiVersion, name),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.DomainNameDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.DomainNameDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(domainNameDefinitions) < 1:
		return &v0.DomainNameDefinition{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(domainNameDefinitions) > 1:
		return &v0.DomainNameDefinition{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &domainNameDefinitions[0], nil
}

// CreateDomainNameDefinition creates a new domain name definition.
func CreateDomainNameDefinition(apiClient *http.Client, apiAddr string, domainNameDefinition *v0.DomainNameDefinition) (*v0.DomainNameDefinition, error) {
	jsonDomainNameDefinition, err := client.MarshalObject(domainNameDefinition)
	if err != nil {
		return domainNameDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-definitions", apiAddr, ApiVersion),
		http.MethodPost,
		bytes.NewBuffer(jsonDomainNameDefinition),
		http.StatusCreated,
	)
	if err != nil {
		return domainNameDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return domainNameDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return domainNameDefinition, nil
}

// UpdateDomainNameDefinition updates a domain name definition.
func UpdateDomainNameDefinition(apiClient *http.Client, apiAddr string, domainNameDefinition *v0.DomainNameDefinition) (*v0.DomainNameDefinition, error) {
	// capture the object ID then remove fields that cannot be updated in the API
	domainNameDefinitionID := *domainNameDefinition.ID
	domainNameDefinition.ID = nil
	domainNameDefinition.CreatedAt = nil
	domainNameDefinition.UpdatedAt = nil

	jsonDomainNameDefinition, err := client.MarshalObject(domainNameDefinition)
	if err != nil {
		return domainNameDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-definitions/%d", apiAddr, ApiVersion, domainNameDefinitionID),
		http.MethodPatch,
		bytes.NewBuffer(jsonDomainNameDefinition),
		http.StatusOK,
	)
	if err != nil {
		return domainNameDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return domainNameDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return domainNameDefinition, nil
}

// DeleteDomainNameDefinition deletes a domain name definition by ID.
func DeleteDomainNameDefinition(apiClient *http.Client, apiAddr string, id uint) (*v0.DomainNameDefinition, error) {
	var domainNameDefinition v0.DomainNameDefinition

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-definitions/%d", apiAddr, ApiVersion, id),
		http.MethodDelete,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &domainNameDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &domainNameDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &domainNameDefinition, nil
}

// GetDomainNameInstances fetches all domain name instances.
// TODO: implement pagination
func GetDomainNameInstances(apiClient *http.Client, apiAddr string) (*[]v0.DomainNameInstance, error) {
	var domainNameInstances []v0.DomainNameInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-instances", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &domainNameInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &domainNameInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &domainNameInstances, nil
}

// GetDomainNameInstanceByID fetches a domain name instance by ID.
func GetDomainNameInstanceByID(apiClient *http.Client, apiAddr string, id uint) (*v0.DomainNameInstance, error) {
	var domainNameInstance v0.DomainNameInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-instances/%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &domainNameInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &domainNameInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &domainNameInstance, nil
}

// GetDomainNameInstanceByName fetches a domain name instance by name.
func GetDomainNameInstanceByName(apiClient *http.Client, apiAddr, name string) (*v0.DomainNameInstance, error) {
	var domainNameInstances []v0.DomainNameInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-instances?name=%s", apiAddr, ApiVersion, name),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.DomainNameInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.DomainNameInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(domainNameInstances) < 1:
		return &v0.DomainNameInstance{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(domainNameInstances) > 1:
		return &v0.DomainNameInstance{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &domainNameInstances[0], nil
}

// CreateDomainNameInstance creates a new domain name instance.
func CreateDomainNameInstance(apiClient *http.Client, apiAddr string, domainNameInstance *v0.DomainNameInstance) (*v0.DomainNameInstance, error) {
	jsonDomainNameInstance, err := client.MarshalObject(domainNameInstance)
	if err != nil {
		return domainNameInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-instances", apiAddr, ApiVersion),
		http.MethodPost,
		bytes.NewBuffer(jsonDomainNameInstance),
		http.StatusCreated,
	)
	if err != nil {
		return domainNameInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return domainNameInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return domainNameInstance, nil
}

// UpdateDomainNameInstance updates a domain name instance.
func UpdateDomainNameInstance(apiClient *http.Client, apiAddr string, domainNameInstance *v0.DomainNameInstance) (*v0.DomainNameInstance, error) {
	// capture the object ID then remove fields that cannot be updated in the API
	domainNameInstanceID := *domainNameInstance.ID
	domainNameInstance.ID = nil
	domainNameInstance.CreatedAt = nil
	domainNameInstance.UpdatedAt = nil

	jsonDomainNameInstance, err := client.MarshalObject(domainNameInstance)
	if err != nil {
		return domainNameInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-instances/%d", apiAddr, ApiVersion, domainNameInstanceID),
		http.MethodPatch,
		bytes.NewBuffer(jsonDomainNameInstance),
		http.StatusOK,
	)
	if err != nil {
		return domainNameInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return domainNameInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return domainNameInstance, nil
}

// DeleteDomainNameInstance deletes a domain name instance by ID.
func DeleteDomainNameInstance(apiClient *http.Client, apiAddr string, id uint) (*v0.DomainNameInstance, error) {
	var domainNameInstance v0.DomainNameInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/domain-name-instances/%d", apiAddr, ApiVersion, id),
		http.MethodDelete,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &domainNameInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &domainNameInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&domainNameInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &domainNameInstance, nil
}
