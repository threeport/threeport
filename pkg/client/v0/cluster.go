package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetClusterInstanceByID fetches a workload cluster by ID
func GetClusterInstanceByID(id uint, apiAddr, apiToken string) (*v0.ClusterInstance, error) {
	var clusterInstance v0.ClusterInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/cluster_instances/%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &clusterInstance, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &clusterInstance, err
	}

	err = json.Unmarshal(jsonData, &clusterInstance)
	if err != nil {
		return &clusterInstance, err
	}

	return &clusterInstance, nil
}

// GetClusterInstanceByName fetches a workload cluster from the Threeport API by
// name.
func GetClusterInstanceByName(name, apiAddr, apiToken string) (*v0.ClusterInstance, error) {
	var clusterInstances []v0.ClusterInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/cluster_instances?name=%s", apiAddr, ApiVersion, name), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &v0.ClusterInstance{}, err
	}
	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.ClusterInstance{}, err
	}

	err = json.Unmarshal(jsonData, &clusterInstances)
	if err != nil {
		return &v0.ClusterInstance{}, err
	}

	switch {
	case len(clusterInstances) < 1:
		return &v0.ClusterInstance{}, errors.New(fmt.Sprintf("no workload clusters with name %s", name))
	case len(clusterInstances) > 1:
		return &v0.ClusterInstance{}, errors.New(fmt.Sprintf("more than one workload cluster with name %s returned", name))
	}

	return &clusterInstances[0], nil
}

// CreateClusterInstance creates a new workload cluster in the Threeport API
// from a json object that contains the workload cluster attributes.
func CreateClusterInstance(jsonClusterInstance []byte, apiAddr, apiToken string) (*v0.ClusterInstance, error) {
	var clusterInstance v0.ClusterInstance

	response, err := GetResponse(fmt.Sprintf("%s/%s/cluster_instances", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonClusterInstance), http.StatusCreated)
	if err != nil {
		return &v0.ClusterInstance{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.ClusterInstance{}, err
	}

	err = json.Unmarshal(jsonData, &clusterInstance)
	if err != nil {
		return &v0.ClusterInstance{}, err
	}

	return &clusterInstance, nil
}
