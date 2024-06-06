package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// GetDefaultKubernetesRuntimeInstance gets the default kubernetes runtime instance.
func GetDefaultKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/kubernetes-runtime-instances?defaultruntime=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
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

// GetKubernetesRuntimeInstancesByKubernetesRuntimeDefinitionID fetches kubernetes runtime
// instances by kubernetes runtime definition ID
func GetKubernetesRuntimeInstancesByKubernetesRuntimeDefinitionID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstances []v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?kubernetesruntimedefinitionid=%d", apiAddr, v0.PathKubernetesRuntimeInstances, id),
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

// GetThreeportControlPlaneKubernetesRuntimeInstance gets the kubernetes runtime instance hosting the
// threeport control plane.
func GetThreeportControlPlaneKubernetesRuntimeInstance(apiClient *http.Client, apiAddr string) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/kubernetes-runtime-instances?threeportcontrolplanehost=true", apiAddr, ApiVersion),
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

// GetInfraProviderByKubernetesRuntimeInstanceID gets the infrastructure provider from the kubernetes runtime instance.
func GetInfraProviderByKubernetesRuntimeInstanceID(apiClient *http.Client, apiAddr string, kubernetesRuntimeInstanceId *uint) (*string, error) {

	// get kubernetes runtime instance
	kri, err := GetKubernetesRuntimeInstanceByID(apiClient, apiAddr, *kubernetesRuntimeInstanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// get kubernetes runtime definition
	krd, err := GetKubernetesRuntimeDefinitionByID(apiClient, apiAddr, *kri.KubernetesRuntimeDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
	}

	return krd.InfraProvider, nil
}
