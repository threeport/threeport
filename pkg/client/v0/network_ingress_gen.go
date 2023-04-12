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

// GetNetworkIngressDefinitionByID feteches a network ingress definition by ID
func GetNetworkIngressDefinitionByID(id uint, apiAddr, apiToken string) (*v0.NetworkIngressDefinition, error) {
	var networkIngressDefinition v0.NetworkIngressDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-definitions/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &networkIngressDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &networkIngressDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return &networkIngressDefinition, nil
}

// GetNetworkIngressDefinitionByName feteches a network ingress definition by name
func GetNetworkIngressDefinitionByName(name, apiAddr, apiToken string) (*v0.NetworkIngressDefinition, error) {
	var networkIngressDefinitions []v0.NetworkIngressDefinition

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-definitions?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.NetworkIngressDefinition{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.NetworkIngressDefinition{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressDefinitions); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	switch {
	case len(networkIngressDefinitions) < 1:
		return &v0.NetworkIngressDefinition{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(networkIngressDefinitions) > 1:
		return &v0.NetworkIngressDefinition{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &networkIngressDefinitions[0], nil
}

// CreateNetworkIngressDefinition creates a new network ingress definition
func CreateNetworkIngressDefinition(networkIngressDefinition *v0.NetworkIngressDefinition, apiAddr, apiToken string) (*v0.NetworkIngressDefinition, error) {
	jsonNetworkIngressDefinition, err := client.MarshalObject(networkIngressDefinition)
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-definitions", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonNetworkIngressDefinition),
		http.StatusCreated,
	)
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return networkIngressDefinition, nil
}

// UpdateNetworkIngressDefinition updates a network ingress definition
func UpdateNetworkIngressDefinition(networkIngressDefinition *v0.NetworkIngressDefinition, apiAddr, apiToken string) (*v0.NetworkIngressDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	networkIngressDefinitionID := *networkIngressDefinition.ID
	networkIngressDefinition.ID = nil

	jsonNetworkIngressDefinition, err := client.MarshalObject(networkIngressDefinition)
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-definitions/%d", apiAddr, ApiVersion, networkIngressDefinitionID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonNetworkIngressDefinition),
		http.StatusOK,
	)
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return networkIngressDefinition, nil
}

// DeleteNetworkIngressDefinition delete a network ingress definition
func DeleteNetworkIngressDefinition(networkIngressDefinition *v0.NetworkIngressDefinition, apiAddr, apiToken string) (*v0.NetworkIngressDefinition, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	networkIngressDefinitionID := *networkIngressDefinition.ID
	networkIngressDefinition.ID = nil

	jsonNetworkIngressDefinition, err := client.MarshalObject(networkIngressDefinition)
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-definitions/%d", apiAddr, ApiVersion, networkIngressDefinitionID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonNetworkIngressDefinition),
		http.StatusNoContent,
	)
	if err != nil {
		return networkIngressDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return networkIngressDefinition, nil
}

// GetNetworkIngressInstanceByID feteches a network ingress instance by ID
func GetNetworkIngressInstanceByID(id uint, apiAddr, apiToken string) (*v0.NetworkIngressInstance, error) {
	var networkIngressInstance v0.NetworkIngressInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-instances/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &networkIngressInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &networkIngressInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return &networkIngressInstance, nil
}

// GetNetworkIngressInstanceByName feteches a network ingress instance by name
func GetNetworkIngressInstanceByName(name, apiAddr, apiToken string) (*v0.NetworkIngressInstance, error) {
	var networkIngressInstances []v0.NetworkIngressInstance

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-instances?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.NetworkIngressInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.NetworkIngressInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	switch {
	case len(networkIngressInstances) < 1:
		return &v0.NetworkIngressInstance{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(networkIngressInstances) > 1:
		return &v0.NetworkIngressInstance{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &networkIngressInstances[0], nil
}

// CreateNetworkIngressInstance creates a new network ingress instance
func CreateNetworkIngressInstance(networkIngressInstance *v0.NetworkIngressInstance, apiAddr, apiToken string) (*v0.NetworkIngressInstance, error) {
	jsonNetworkIngressInstance, err := client.MarshalObject(networkIngressInstance)
	if err != nil {
		return networkIngressInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-instances", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonNetworkIngressInstance),
		http.StatusCreated,
	)
	if err != nil {
		return networkIngressInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return networkIngressInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return networkIngressInstance, nil
}

// UpdateNetworkIngressInstance updates a network ingress instance
func UpdateNetworkIngressInstance(networkIngressInstance *v0.NetworkIngressInstance, apiAddr, apiToken string) (*v0.NetworkIngressInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	networkIngressInstanceID := *networkIngressInstance.ID
	networkIngressInstance.ID = nil

	jsonNetworkIngressInstance, err := client.MarshalObject(networkIngressInstance)
	if err != nil {
		return networkIngressInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-instances/%d", apiAddr, ApiVersion, networkIngressInstanceID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonNetworkIngressInstance),
		http.StatusOK,
	)
	if err != nil {
		return networkIngressInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return networkIngressInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&networkIngressInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API", err)
	}

	return networkIngressInstance, nil
}

// DeleteNetworkIngressInstance delete a network ingress instance
func DeleteNetworkIngressInstance(networkIngressInstance *v0.NetworkIngressInstance, apiAddr, apiToken string) (*v0.NetworkIngressInstance, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	networkIngressInstanceID := *networkIngressInstance.ID
	networkIngressInstance.ID = nil

	jsonNetworkIngressInstance, err := client.MarshalObject(networkIngressInstance)
	if err != nil {
		return networkIngressInstance, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/network-ingress-instances/%d", apiAddr, ApiVersion, networkIngressInstanceID),
		apiToken,
		http.MethodDelete,
		bytes.NewBuffer(jsonNetworkIngressInstance),
		http.StatusNoContent,
	)
	if err != nil {
		return networkIngressInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	return networkIngressInstance, nil
}
