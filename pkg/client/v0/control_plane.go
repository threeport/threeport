package v0

import (
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetWorkloadInstancesByWorkloadDefinitionID fetches workload instances
// by workload definition ID
func GetControlPlaneInstancesByControlPlaneDefinitionID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, fmt.Sprintf("controlplanedefinitionid=%d", id))
	if err != nil {
		return nil, fmt.Errorf("could not retrieve control plane instances with definition id: %w", err)
	}

	return controlPlaneInstances, nil
}

// GetSelfControlPlaneInstance fetches the control plane instance that represents the control plane being run on
func GetSelfControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, "isself=true")
	if err != nil {
		return nil, fmt.Errorf("could not retrieve self control plane instance: %w", err)
	}

	switch {
	case len(*controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, errors.New("no local control plane instance")
	case len(*controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(*controlPlaneInstances)))
	}

	return &(*controlPlaneInstances)[0], nil
}

// GetGenesisControlPlaneInstance fetches the genesis control instance
func GetGenesisControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, "genesis=true")
	if err != nil {
		return nil, fmt.Errorf("could not retrieve genesis control plane instance: %w", err)
	}

	switch {
	case len(*controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, errors.New("no local control plane instance")
	case len(*controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(*controlPlaneInstances)))
	}

	return &(*controlPlaneInstances)[0], nil
}
