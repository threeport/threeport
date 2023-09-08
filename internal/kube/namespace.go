package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/datatypes"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// NamespaceFinder is used to get namespace info from JSON representations of
// Kubernetes objects.
type NamespaceFinder struct {
	Kind     string              `json:"kind"`
	Metadata NamespaceFinderMeta `json:"metadata"`
}

// NamespaceFinderMeta is used to get namespace info from the metadata field of
// JSON representations of Kubernetes objects.
type NamespaceFinderMeta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// GetNamespaceFromJSON takes the JSON representation of a Kubernetes object and
// returns the namespace name if it is a Namespace object, the
// metaddata.namespace value if a namespaced object or an empty string if it is
// a non-namespaced ojbect.
func GetNamespaceFromJSON(jsonObject []byte) (string, error) {
	var object NamespaceFinder
	if err := json.Unmarshal(jsonObject, &object); err != nil {
		return "", fmt.Errorf("failed to unmarshal JSON object: %w", err)
	}

	if object.Kind == "Namespace" {
		return object.Metadata.Name, nil
	}

	return object.Metadata.Namespace, nil
}

// SetNamespaces adds the namespace resource and namespace assignment as needed
// to an array of workload resource instances.
func SetNamespaces(
	workloadResourceInstances *[]v0.WorkloadResourceInstance,
	workloadInstance *v0.WorkloadInstance,
	discoveryClient *discovery.DiscoveryClient,
) (*[]v0.WorkloadResourceInstance, error) {
	// first check to see if any namespaces are included - if so assume
	// namespaces are managed by client and do nothing
	clientManagedNS := ""
	for _, wri := range *workloadResourceInstances {
		var mapDef map[string]interface{}
		err := json.Unmarshal([]byte(*wri.JSONDefinition), &mapDef)
		if err != nil {
			return workloadResourceInstances, fmt.Errorf("failed to unmarshal json: %w", err)
		}
		if mapDef["kind"] == "Namespace" {
			metadata := mapDef["metadata"].(map[string]interface{})
			clientManagedNS = metadata["name"].(string)
			break
		}
	}

	namespace := ""
	if clientManagedNS == "" {
		// we are managing namespaces for the client - create namespace and add to
		// array of processed workload resource instances
		namespace = fmt.Sprintf("%s-%s", *workloadInstance.Name, util.RandomAlphaNumericString(10))
	} else {
		namespace = clientManagedNS
	}

	processedWRIs := []v0.WorkloadResourceInstance{}
	namespacedObjectCount := 0
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
			processedWRIs = append(processedWRIs, wri)
			continue
		}
		namespacedObjectCount++

		// update the resource to set the namespace
		updatedJSONDef, err := updateNamespace([]byte(*wri.JSONDefinition), namespace)
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

	// only prepend the namespace resource if there are namespaced resources that require it
	if namespacedObjectCount > 0 && clientManagedNS == "" {

		namespaceWRI, err := createNamespaceWorkloadResourceInstance(namespace, *workloadInstance.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create new workload resource instance for namespace: %w", err)
		}

		// move first resource to the back of the array, then prepend the namespace
		processedWRIs = append(processedWRIs, processedWRIs[0])
		processedWRIs[0] = *namespaceWRI
	}
	return &processedWRIs, nil
}

// GetManagedNamespaceNames returns the names of the namespaces created and manged
// for the user by threeport.
func GetManagedNamespaceNames(kubeClient dynamic.Interface) ([]string, error) {
	var namespaceNames []string
	gvr := schema.GroupVersionResource{
		Version:  "v1",
		Resource: "namespaces",
	}
	namespaces, err := kubeClient.Resource(gvr).List(context.TODO(), kubemetav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/managed-by=%s", KubeManagedByLabel),
	})
	if err != nil {
		return namespaceNames, fmt.Errorf("failed to list namespaces by label selector")
	}

	for _, ns := range namespaces.Items {
		namespaceNames = append(namespaceNames, ns.GetName())
	}

	return namespaceNames, nil
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
	reconciled := false
	workloadResourceInstance := v0.WorkloadResourceInstance{
		JSONDefinition:     &JSONDef,
		WorkloadInstanceID: &workloadInstanceID,
		Reconciled:         &reconciled,
	}

	return &workloadResourceInstance, nil
}
