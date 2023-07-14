package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetDefaultKubernetesRuntimeInstance gets the default kubernetes runtime instance.
func GetDefaultKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/kubernetes-runtime-instances?defaultruntime=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &kubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) == 0 {
		return &kubernetesRuntimeInstance, errors.New("no default kubernetes runtime instance found")
	}
	if len(response.Data) > 1 {
		return &kubernetesRuntimeInstance, errors.New("multiple kubernetes runtime instances marked as default")
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

// GetThreeportControlPlaneKubernetesRuntimeInstance gets the kubernetes runtime instance hosting the
// threeport control plane.
func GetThreeportControlPlaneKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/kubernetes-runtime-instances?threeportcontrolplanehost=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
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
