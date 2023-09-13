package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetWorkloadInstancesByWorkloadDefinitionID fetches workload instances
// by workload definition ID
func GetControlPlaneInstancesByControlPlaneDefinitionID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.ControlPlaneInstance, error) {
	var controlPlaneInstances []v0.ControlPlaneInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?controlplanedefinitionid=%d", apiAddr, v0.PathControlPlaneInstances, id),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &controlPlaneInstances, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &controlPlaneInstances, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&controlPlaneInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &controlPlaneInstances, nil
}

// GetSelfControlPlaneInstance fetches the control plane instance that represents the control plane being run on
func GetSelfControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	var controlPlaneInstances []v0.ControlPlaneInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?isself=true", apiAddr, v0.PathControlPlaneInstances),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.ControlPlaneInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.ControlPlaneInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&controlPlaneInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, errors.New("no local control plane instance")
	case len(controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(controlPlaneInstances)))
	}

	return &controlPlaneInstances[0], nil
}

// GetGenesisControlPlaneInstance fetches the genesis control instance
func GetGenesisControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	var controlPlaneInstances []v0.ControlPlaneInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?genesis=true", apiAddr, v0.PathControlPlaneInstances),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.ControlPlaneInstance{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.ControlPlaneInstance{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&controlPlaneInstances); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, errors.New("no local control plane instance")
	case len(controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(controlPlaneInstances)))
	}

	return &controlPlaneInstances[0], nil
}
