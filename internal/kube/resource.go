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
		//if strings.Contains(err.Error(), "already exists") {
		if errors.IsAlreadyExists(err) {
			return kubeObject, nil
		} else {
			return nil, fmt.Errorf("failed to create kubernetes resource:%w", err)
		}
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
