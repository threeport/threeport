package v0

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	resource, err := resourceClient.Get(context.Background(), resourceName, kubemetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get resource from kubernetes API: %w", err)
	}

	return resource, nil
}

// CreateResource takes an unstructured object, dynamic client interface and rest
// mapper and creates the resource in the target Kubernetes cluster.  If the
// object already exists, it returns the object.
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
		Create(context.Background(), kubeObject, kubemetav1.CreateOptions{})
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
// mapper and updates the resource in the target Kubernetes cluster. It returns the updated object.
func UpdateResource(
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
		Update(context.TODO(), kubeObject, kubemetav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update kubernetes resource: %w", err)
	}

	return result, nil
}

// CreateOrUpdateResource takes an unstructured object, dynamic client interface and rest
// mapper and creates the resource in the target Kubernetes cluster if it doesn't already
// exist.  If the resource exists, it is updated.
func CreateOrUpdateResource(
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

		// if the resource already exists, update it

		switch {
		case errors.IsAlreadyExists(err):
			if result, err = UpdateResource(kubeObject, kubeClient, mapper, mapping); err != nil {
				return nil, fmt.Errorf("failed to update kubernetes resource:%w", err)
			}

		// If the resource is an existing service and its nodeport is already configured, the
		// kube API will return an IsInvalid error instead of an IsAlreadyExists error.
		// If the service is not already created and is also invalid, then an error should
		// be thrown by UpdateResource.
		case errors.IsInvalid(err) &&
			mapping.GroupVersionKind.Kind == "Service":
			if result, err = UpdateResource(kubeObject, kubeClient, mapper, mapping); err != nil {
				return nil, fmt.Errorf("failed to update kubernetes resource:%w", err)
			}
		default:
			return nil, fmt.Errorf("failed to create kubernetes resource:%w", err)
		}
	}

	return result, nil
}

// UpdateResource updates a Kubernetes resource.
func UpdateResource(
	kubeObject *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
	mapping *meta.RESTMapping,
) (*unstructured.Unstructured, error) {

	// get the existing resource
	existingResource, err := GetResource(
		kubeObject.GroupVersionKind().Group,
		kubeObject.GroupVersionKind().Version,
		kubeObject.GroupVersionKind().Kind,
		kubeObject.GetNamespace(),
		kubeObject.GetName(),
		kubeClient,
		mapper,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing resource: %w", err)
	}

	// set the resource version
	kubeObject.SetResourceVersion(existingResource.GetResourceVersion())

	// update the resource
	result, err := kubeClient.
		Resource(mapping.Resource).
		Namespace(kubeObject.GetNamespace()).
		Update(context.TODO(), kubeObject, kubemetav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update kubernetes resource:%w", err)
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
		Delete(context.Background(), kubeObject.GetName(), kubemetav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete kubernetes resource:%w", err)
	}

	return nil
}

// DeleteLabelledPodsInNamespace takes a namespace, set of labels, kube client
// and mapper and deletes all the pods.
func DeleteLabelledPodsInNamespace(
	namespace string,
	labels map[string]string,
	restConfig *rest.Config,
) error {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to generate Kubernete clientset from REST config: %w", err)
	}

	var labelSelectorSlice []string
	for k, v := range labels {
		labelSelectorSlice = append(labelSelectorSlice, fmt.Sprintf("%s=%s", k, v))
	}
	labelSelectors := strings.Join(labelSelectorSlice, ",")

	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), kubemetav1.ListOptions{
		LabelSelector: labelSelectors,
	})
	if err != nil {
		return fmt.Errorf("failed get pods in namespace %s with desired labels: %w", namespace, err)
	}

	for _, pod := range pods.Items {
		err := clientset.CoreV1().Pods(namespace).Delete(context.Background(), pod.Name, kubemetav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete pod %s: %w", pod.Name, err)
		}
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
