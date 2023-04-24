package kube

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/threeport/threeport/internal/util"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// SetNamespaces adds the namespace resource and namespace assignment as needed
// to an array of workload resource instances.
func SetNamespaces(
	workloadResourceInstances *[]v0.WorkloadResourceInstance,
	workloadInstance *v0.WorkloadInstance,
	discoveryClient *discovery.DiscoveryClient,
) (*[]v0.WorkloadResourceInstance, error) {
	// first check to see if any namespaces are included - if so assume
	// namespaces are managed by client and do nothing
	clientManagedNS := false
	for _, wri := range *workloadResourceInstances {
		var mapDef map[string]interface{}
		err := json.Unmarshal([]byte(*wri.JSONDefinition), &mapDef)
		if err != nil {
			return workloadResourceInstances, fmt.Errorf("failed to unmarshal json: %w", err)
		}
		if mapDef["kind"] == "Namespace" {
			clientManagedNS = true
			break
		}
	}
	if clientManagedNS {
		return workloadResourceInstances, nil
	}

	// we are managing namespaces for the client - create namespace and add to
	// array of processed workload resource instances
	managedNSName := fmt.Sprintf("%s-%s", *workloadInstance.Name, util.RandomString(10))
	namespaceWRI, err := createNamespaceWorkloadResourceInstance(managedNSName, *workloadInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create new workload resource instance for namespace: %w", err)
	}
	processedWRIs := []v0.WorkloadResourceInstance{*namespaceWRI}

	for _, wri := range *workloadResourceInstances {
		// check to see if this is a namespaced resource
		namespaced, err := isNamespaced(
			string(*wri.JSONDefinition),
			discoveryClient,
		)
		if err != nil {
			return &processedWRIs, fmt.Errorf("failed to determine if workload resource instance is namespaced: %w", err)
		}
		if !namespaced {
			// skip non-namespaced resources
			continue
		}

		// update the resource to set the namespace
		updatedJSONDef, err := updateNamespace([]byte(*wri.JSONDefinition), managedNSName)
		if err != nil {
			return &processedWRIs, fmt.Errorf("failed to update JSON definition to set namespace: %w", err)
		}

		// convert the resource back into a gorm.io/datatypes.JSON object
		var jsonObj datatypes.JSON
		if err := json.Unmarshal(updatedJSONDef, &jsonObj); err != nil {
			return &processedWRIs, fmt.Errorf("failed to convert resource definition back into gorm JSON object type: %w", err)
		}
		wri.JSONDefinition = &jsonObj
		processedWRIs = append(processedWRIs, wri)
	}

	return &processedWRIs, nil
}

// updateNamespace takes the JSON definition for a Kubernetes resource and sets
// the namespace.
func updateNamespace(jsonDef []byte, namespace string) ([]byte, error) {
	// unmarshal the JSON into a map
	var mapDef map[string]interface{}
	err := json.Unmarshal(jsonDef, &mapDef)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON definition to map: %w", err)
	}

	// set the namespace field in the metadata
	if metadata, ok := mapDef["metadata"].(map[string]interface{}); ok {
		metadata["namespace"] = namespace
	} else {
		return nil, errors.New("failed to find \"metadata\" field in JSON definition")
	}

	// marshal the modified map back to JSON
	modifiedJSON, err := json.Marshal(mapDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON from modified map: %w", err)
	}

	return modifiedJSON, nil
}

// isNamespaced returns true if a provided JSON definition represents a
// namespaced resource in Kubernetes.
func isNamespaced(
	jsonDef string,
	discoveryClient *discovery.DiscoveryClient,
) (bool, error) {
	// get the GroupVersionKind for provided JSON definition
	gvk, err := getGroupVersionKindFromJSON([]byte(jsonDef))
	if err != nil {
		return false, fmt.Errorf("failed to GroupVersionKind for provided resource definition: %w", err)
	}

	// get the kube API resource for the resource's GVK
	apiResource, err := getAPIResource(discoveryClient, gvk)
	if err != nil {
		return false, fmt.Errorf("failed to get API resource from Kubernetes: %w", err)
	}

	return apiResource.Namespaced, nil
}

// createNamespaceWorkloadResourceInstance returns a workload instance for a
// Kubernetes namespace resource with the desired name.
func createNamespaceWorkloadResourceInstance(
	namespaceName string,
	workloadInstanceID uint,
) (*v0.WorkloadResourceInstance, error) {
	// create the namespace resource with the desired name
	namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": namespaceName,
			},
		},
	}
	namespaceJSON, err := namespace.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json for namespace object: %w", err)
	}

	// construct the workload resource instance
	var JSONDef datatypes.JSON
	JSONDef = namespaceJSON
	workloadResourceInstance := v0.WorkloadResourceInstance{
		JSONDefinition:     &JSONDef,
		WorkloadInstanceID: &workloadInstanceID,
	}

	return &workloadResourceInstance, nil
}
