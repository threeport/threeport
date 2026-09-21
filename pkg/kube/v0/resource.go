package v0

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	yamlv3 "gopkg.in/yaml.v3"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

// createRetryAttempts is the number of times a transient kube-apiserver
// error (server-side timeouts, quota-evaluator timeouts) is retried before
// the caller sees the failure. Enough tries to ride out a short apiserver
// backpressure spike during control plane bring-up, but not so many that a
// genuine misconfiguration hides for minutes.
const createRetryAttempts = 5

// createRetryBaseDelay backs off exponentially between retries starting
// from this base (250ms, 500ms, 1s, 2s) so the apiserver has a chance to
// catch up on quota evaluation before the next attempt. It is a variable
// rather than a constant so tests can shrink the wait.
var createRetryBaseDelay = 250 * time.Millisecond

// isTransientKubeError reports whether a Kubernetes API error is worth
// retrying. Covers server-side timeouts, temporary service unavailable
// responses, and the "Internal error occurred: resource quota evaluation
// timed out" surface kube-apiserver emits when its quota evaluator is
// backpressured during a burst of resource creates. Every other error
// (AlreadyExists, NotFound, Forbidden, Invalid) is deterministic and
// should not be retried.
func isTransientKubeError(err error) bool {
	if err == nil {
		return false
	}
	if kubeerr.IsServerTimeout(err) || kubeerr.IsTimeout(err) || kubeerr.IsServiceUnavailable(err) || kubeerr.IsTooManyRequests(err) {
		return true
	}
	if kubeerr.IsInternalError(err) && strings.Contains(err.Error(), "resource quota evaluation timed out") {
		return true
	}
	return false
}

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

	// create the kube resource, retrying transient apiserver errors
	// (quota-evaluator timeout, server-side timeouts) so a busy control
	// plane during genesis bring-up does not abort the whole install.
	var result *unstructured.Unstructured
	delay := createRetryBaseDelay
	for attempt := 0; attempt < createRetryAttempts; attempt++ {
		result, err = kubeClient.
			Resource(mapping.Resource).
			Namespace(kubeObject.GetNamespace()).
			Create(context.Background(), kubeObject, kubemetav1.CreateOptions{})
		if err == nil {
			return result, nil
		}
		if kubeerr.IsAlreadyExists(err) {
			return kubeObject, nil
		}
		if !isTransientKubeError(err) {
			return nil, fmt.Errorf("failed to create kubernetes resource:%w", err)
		}
		if attempt < createRetryAttempts-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return nil, fmt.Errorf("failed to create kubernetes resource after %d attempts:%w", createRetryAttempts, err)

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
		case kubeerr.IsAlreadyExists(err):
			if result, err = UpdateResource(kubeObject, kubeClient, mapper, mapping); err != nil {
				return nil, fmt.Errorf("failed to update kubernetes resource:%w", err)
			}

		// If the resource is an existing service and its nodeport is already configured, the
		// kube API will return an IsInvalid error instead of an IsAlreadyExists error.
		// If the service is not already created and is also invalid, then an error should
		// be thrown by UpdateResource.
		case kubeerr.IsInvalid(err) &&
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
//
// A missing CRD on the target cluster, meaning no REST mapping for the
// object's GroupKind, counts as already deleted: an unregistered kind cannot
// have instances, so there is nothing left to remove. That matches how a
// NotFound on the delete call itself is treated, and together they keep a
// delete reconciler from retrying forever on a resource that has nowhere to
// live.
func DeleteResource(
	kubeObject *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) error {
	// get the mapping for resource from kube object's group, kind
	mapping, err := getResourceMapping(kubeObject, mapper)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to get REST mapping for kubernetes resource: %w", err)
	}

	// delete the kube resource
	err = kubeClient.
		Resource(mapping.Resource).
		Namespace(kubeObject.GetNamespace()).
		Delete(context.Background(), kubeObject.GetName(), kubemetav1.DeleteOptions{})
	if err != nil && !kubeerr.IsNotFound(err) {
		return fmt.Errorf("failed to delete kubernetes resource:%w", err)
	}

	return nil
}

// DeletePod deletes a pod.
func DeletePod(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	name,
	namespace string,
) error {

	// initiate namespace deletion
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	// delete the pod
	// get the mapping for resource from kube object's group, kind
	mapping, err := getResourceMapping(pod, *mapper)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for kubernetes resource: %w", err)
	}

	// Define your label selector here
	labelSelector := kubemetav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name": fmt.Sprintf("threeport-%s", name),
		},
	}

	// convert label selector to string
	selector := labels.Set(labelSelector.MatchLabels).String()

	// list all resources matching the label selector
	resourceList, err := kubeClient.
		Resource(mapping.Resource).
		Namespace(pod.GetNamespace()).
		List(
			context.Background(),
			kubemetav1.ListOptions{LabelSelector: selector},
		)
	if err != nil {
		return fmt.Errorf("failed to list kubernetes resources: %w", err)
	}

	// delete the kube resource
	for _, resource := range resourceList.Items {
		err = kubeClient.
			Resource(mapping.Resource).
			Namespace(pod.GetNamespace()).
			Delete(context.Background(), resource.GetName(), kubemetav1.DeleteOptions{})
		if err != nil && !kubeerr.IsNotFound(err) {
			return fmt.Errorf("failed to delete kubernetes resource:%w", err)
		}
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

// GetJsonResourcesFromYamlDoc takes a YAML document with any number of
// Kubernetes resources defined and returns a slice of JSON objects as byte
// arrays.
func GetJsonResourcesFromYamlDoc(yamlDoc string) ([][]byte, error) {
	decoder := yamlv3.NewDecoder(strings.NewReader(yamlDoc))

	var jsonObjects [][]byte
	for {
		// decode the next resource, exit loop if the end has been reached
		var node yamlv3.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return jsonObjects, fmt.Errorf("failed to decode yaml node in kubernetes workload definition: %w", err)
		}

		// marshal the yaml
		yamlContent, err := yamlv3.Marshal(&node)
		if err != nil {
			return jsonObjects, fmt.Errorf("failed to marshal yaml from kubernetes workload definition: %w", err)
		}

		// convert yaml to json
		jsonContent, err := yaml.YAMLToJSON(yamlContent)
		if err != nil {
			return jsonObjects, fmt.Errorf("failed to convert yaml to json: %w", err)
		}

		jsonObjects = append(jsonObjects, jsonContent)
	}

	return jsonObjects, nil
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
