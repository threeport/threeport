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

// GetGcpProviderByDefaultProvider fetches the default GCP provider.
func GetGcpProviderByDefaultProvider(apiClient *http.Client, apiAddr string) (*v0.GcpProvider, error) {
	var gcpProvider v0.GcpProvider

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/gcp-providers?defaultprovider=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &gcpProvider, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &gcpProvider, errors.New("no default GCP provider found")
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &gcpProvider, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gcpProvider); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gcpProvider, nil
}

// GetGcpProviderByProjectID fetches a GCP provider by the GCP Project ID.
func GetGcpProviderByProjectID(apiClient *http.Client, apiAddr string, projectID string) (*v0.GcpProvider, error) {
	var gcpProvider v0.GcpProvider

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/gcp-providers?projectid=%s", apiAddr, ApiVersion, projectID),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &gcpProvider, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &gcpProvider, fmt.Errorf("no GCP provider found with project ID %s", projectID)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &gcpProvider, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gcpProvider); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gcpProvider, nil
}

// GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef fetches a GCP GKE kubernetes runtime definition by ID.
func GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef(apiClient *http.Client, apiAddr string, id uint) (*v0.GcpGkeKubernetesRuntimeDefinition, error) {
	var gcpGkeKubernetesRuntimeDefinition v0.GcpGkeKubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/gcp-gke-kubernetes-runtime-definitions?kubernetesruntimedefinitionid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &gcpGkeKubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &gcpGkeKubernetesRuntimeDefinition, fmt.Errorf("no object found with ID %d", id)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &gcpGkeKubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gcpGkeKubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gcpGkeKubernetesRuntimeDefinition, nil
}

// GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst fetches a GCP GKE kubernetes runtime instance by ID.
func GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient *http.Client, apiAddr string, id uint) (*v0.GcpGkeKubernetesRuntimeInstance, error) {
	var gcpGkeKubernetesRuntimeInstance v0.GcpGkeKubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/gcp-gke-kubernetes-runtime-instances?kubernetesruntimeinstanceid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &gcpGkeKubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &gcpGkeKubernetesRuntimeInstance, fmt.Errorf("no object found with ID %d", id)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &gcpGkeKubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&gcpGkeKubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &gcpGkeKubernetesRuntimeInstance, nil
}
