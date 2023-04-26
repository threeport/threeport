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

// GetWorkloadDependencies feteches all workload dependencies.
// TODO: implement pagination
func GetWorkloadDependencies(apiAddr, apiToken string) (*[]v0.WorkloadDependency, error) {
	var workloadDependencies []v0.WorkloadDependency

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/workload-dependencies", apiAddr, ApiVersion),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &workloadDependencies, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &workloadDependencies, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadDependencies); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &workloadDependencies, nil
}

// GetWorkloadDependencyByID feteches a workload dependency by ID.
func GetWorkloadDependencyByID(id uint, apiAddr, apiToken string) (*v0.WorkloadDependency, error) {
	var workloadDependency v0.WorkloadDependency

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/workload-dependencies/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &workloadDependency, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &workloadDependency, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadDependency); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &workloadDependency, nil
}

// GetWorkloadDependencyByName feteches a workload dependency by name.
func GetWorkloadDependencyByName(name, apiAddr, apiToken string) (*v0.WorkloadDependency, error) {
	var workloadDependencies []v0.WorkloadDependency

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/workload-dependencies?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.WorkloadDependency{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.WorkloadDependency{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadDependencies); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(workloadDependencies) < 1:
		return &v0.WorkloadDependency{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(workloadDependencies) > 1:
		return &v0.WorkloadDependency{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &workloadDependencies[0], nil
}

// CreateWorkloadDependency creates a new workload dependency.
func CreateWorkloadDependency(workloadDependency *v0.WorkloadDependency, apiAddr, apiToken string) (*v0.WorkloadDependency, error) {
	jsonWorkloadDependency, err := client.MarshalObject(workloadDependency)
	if err != nil {
		return workloadDependency, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/workload-dependencies", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonWorkloadDependency),
		http.StatusCreated,
	)
	if err != nil {
		return workloadDependency, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return workloadDependency, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadDependency); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return workloadDependency, nil
}

// UpdateWorkloadDependency updates a workload dependency.
func UpdateWorkloadDependency(workloadDependency *v0.WorkloadDependency, apiAddr, apiToken string) (*v0.WorkloadDependency, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	workloadDependencyID := *workloadDependency.ID
	workloadDependency.ID = nil

	jsonWorkloadDependency, err := client.MarshalObject(workloadDependency)
	if err != nil {
		return workloadDependency, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/workload-dependencies/%d", apiAddr, ApiVersion, workloadDependencyID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonWorkloadDependency),
		http.StatusOK,
	)
	if err != nil {
		return workloadDependency, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return workloadDependency, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadDependency); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return workloadDependency, nil
}

// DeleteWorkloadDependency deletes a workload dependency by ID.
func DeleteWorkloadDependency(id uint, apiAddr, apiToken string) (*v0.WorkloadDependency, error) {
	var workloadDependency v0.WorkloadDependency

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/workload-dependencies/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodDelete,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &workloadDependency, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &workloadDependency, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&workloadDependency); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &workloadDependency, nil
}
