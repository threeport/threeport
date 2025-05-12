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

// GetOciAccountByDefaultAccount fetches the default OCI account.
func GetOciAccountByDefaultAccount(apiClient *http.Client, apiAddr string) (*v0.OciAccount, error) {
	var ociAccount v0.OciAccount

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/oci-accounts?default=true", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &ociAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &ociAccount, errors.New("no default OCI account found")
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &ociAccount, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ociAccount); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &ociAccount, nil
}

// GetOciAccountByTenancyID fetches a OCI account by the OCI Tenancy ID.
func GetOciAccountByTenancyID(apiClient *http.Client, apiAddr string, tenancyID string) (*v0.OciAccount, error) {
	var ociAccount v0.OciAccount

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/oci-accounts?tenancyid=%s", apiAddr, ApiVersion, tenancyID),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &ociAccount, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &ociAccount, errors.New(fmt.Sprintf("no OCI account found with tenancy ID %s", tenancyID))
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &ociAccount, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ociAccount); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &ociAccount, nil
}

// GetOciOkeKubernetesRuntimeDefinitionByK8sRuntimeDef fetches a OCI OKE kubernetes runtime definition by ID.
func GetOciOkeKubernetesRuntimeDefinitionByK8sRuntimeDef(apiClient *http.Client, apiAddr string, id uint) (*v0.OciOkeKubernetesRuntimeDefinition, error) {
	var ociOkeKubernetesRuntimeDefinition v0.OciOkeKubernetesRuntimeDefinition

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/oci-oke-kubernetes-runtime-definitions?kubernetesruntimedefinitionid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &ociOkeKubernetesRuntimeDefinition, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &ociOkeKubernetesRuntimeDefinition, errors.New(fmt.Sprintf("no object found with ID %d", id))
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &ociOkeKubernetesRuntimeDefinition, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ociOkeKubernetesRuntimeDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &ociOkeKubernetesRuntimeDefinition, nil
}

// GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst fetches a OCI OKE kubernetes runtime instance by ID.
func GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient *http.Client, apiAddr string, id uint) (*v0.OciOkeKubernetesRuntimeInstance, error) {
	var ociOkeKubernetesRuntimeInstance v0.OciOkeKubernetesRuntimeInstance

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/oci-oke-kubernetes-runtime-instances?kubernetesruntimeinstanceid=%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &ociOkeKubernetesRuntimeInstance, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &ociOkeKubernetesRuntimeInstance, errors.New(fmt.Sprintf("no object found with ID %d", id))
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &ociOkeKubernetesRuntimeInstance, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&ociOkeKubernetesRuntimeInstance); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &ociOkeKubernetesRuntimeInstance, nil
}
