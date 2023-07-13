package kube

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// GetResource returns a specific Kubernetes resource.  If an empty string for
// namespace is provided, this function will search for a non-namespaced
// resource.  Namespaced resources must have the namespace provided, even if in
// the "default" namespace.  Core resources should provide "core" or  an empty
// string for kubeAPIGroup.
func GetResource(
	kubeAPIGroup string,
	kubeAPIVersion string,
	kubeKind string,
	namespace string,
	resourceName string,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (*unstructured.Unstructured, error) {
	// map the resource kind
	var gvk schema.GroupVersionKind
	if kubeAPIGroup == "" || kubeAPIGroup == "core" {
		gvk = schema.GroupVersionKind{Version: kubeAPIVersion, Kind: kubeKind}
	} else {
		gvk = schema.GroupVersionKind{Group: kubeAPIGroup, Version: kubeAPIVersion, Kind: kubeKind}
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to map kubernetes API version and kind: %w", err)
	}

	// create a resource client using the mapping
	var resourceClient dynamic.ResourceInterface
	if namespace == "" {
		resourceClient = kubeClient.Resource(mapping.Resource)
	} else {
		resourceClient = kubeClient.Resource(mapping.Resource).Namespace(namespace)
	}

	// get the resource
	resource, err := resourceClient.Get(context.TODO(), resourceName, kubemetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get resource from kubernetes API: %w", err)
	}

	return resource, nil
}

// CreateResource takes an unstructured object, dynamic client interface and rest
// mapper and creates the resource in the target Kubernetes cluster.
func CreateResource(
	kubeObject *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (*unstructured.Unstructured, error) {
	// get the mapping for resource from kube object's group, kind
	mapping, err := getResourceMapping(kubeObject, mapper)
	if err != nil {
		return nil, fmt.Errorf("failed to get REST mapping for kubernetes resource: %w", err)
	}

	// create the kube resource
	result, err := kubeClient.
		Resource(mapping.Resource).
		Namespace(kubeObject.GetNamespace()).
		Create(context.TODO(), kubeObject, kubemetav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return kubeObject, nil
		} else {
			return nil, fmt.Errorf("failed to create kubernetes resource:%w", err)
		}
	}

	return result, nil

}

// UpdateResource takes an unstructured object, dynamic client interface and rest
// mapper and updates the resource in the target Kubernetes cluster.
func UpdateResource(
	kubeObject *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (*unstructured.Unstructured, error) {
	// get the mapping for resource from kube object's group, kind
	mapping, err := getResourceMapping(kubeObject, mapper)
	if err != nil {
		return nil, err
	}

	// create the kube resource
	result, err := kubeClient.
		Resource(mapping.Resource).
		Namespace(kubeObject.GetNamespace()).
		Update(context.TODO(), kubeObject, kubemetav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteResource takes an unstructured object, dynamic client interface and rest
// mapper and deletes the resource in the target Kubernetes cluster.
func DeleteResource(
	kubeObject *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) error {
	// get the mapping for resource from kube object's group, kind
	mapping, err := getResourceMapping(kubeObject, mapper)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for kubernetes resource: %w", err)
	}

	// delete the kube resource
	err = kubeClient.
		Resource(mapping.Resource).
		Namespace(kubeObject.GetNamespace()).
		Delete(context.TODO(), kubeObject.GetName(), kubemetav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete kubernetes resource:%w", err)
	}

	return nil
}

// getResourceMapping gets the REST mapping for a given unstructured Kubernetes
// object.
func getResourceMapping(kubeObject *unstructured.Unstructured, mapper meta.RESTMapper) (*meta.RESTMapping, error) {
	gk := schema.GroupKind{
		Group: kubeObject.GroupVersionKind().Group,
		Kind:  kubeObject.GetKind(),
	}
	mapping, err := mapper.RESTMapping(gk)
	if err != nil {
		return nil, fmt.Errorf("failed to map kube object group kind to resource: %w", err)
	}

	return mapping, nil
}
