package v0

import (
	"encoding/json"
	"fmt"

	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// getAPIResource returns the APIResource for a given GroupVersionKind.
func getAPIResource(
	dc *discovery.DiscoveryClient,
	gvk *schema.GroupVersionKind,
) (*kubemetav1.APIResource, error) {
	resourceList, err := dc.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return nil, fmt.Errorf("failed to get resource list for the provided GroupVersionKind: %w", err)
	}

	for _, apiResource := range resourceList.APIResources {
		if apiResource.Kind == gvk.Kind {
			return &apiResource, nil
		}
	}

	return nil, fmt.Errorf("failed to find APIResource for provided GroupVersionKind: %v", gvk)
}

// getGroupVersionKindFromJSON takes the JSON representation of a Kubernetes
// resource and returns the GroupVersionKind object.
func getGroupVersionKindFromJSON(resourceJSON []byte) (*schema.GroupVersionKind, error) {
	// unmarshal the JSON representation into an unstructured.Unstructured object
	var resource unstructured.Unstructured
	err := json.Unmarshal(resourceJSON, &resource)
	if err != nil {
		return &schema.GroupVersionKind{}, fmt.Errorf("failed to unmarshal JSON into unstructured Kubernetes object: %w", err)
	}
	gvk := resource.GroupVersionKind()

	return &gvk, nil
}
