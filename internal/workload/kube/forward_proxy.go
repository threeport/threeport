package kube

//"k8s.io/apimachinery/pkg/api/meta"
//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
//"k8s.io/client-go/dynamic"

const ForwardProxyNamespace = "forward-proxy-system"

//func UpdateForwardProxyResource(
//	desiredObject *unstructured.Unstructured,
//	kubeClient dynamic.Interface,
//	mapper meta.RESTMapper,
//) (*unstructured.Unstructured, error) {
//	// get the mapping for resource from kube object's group, kind
//	mapping, err := getResourceMapping(desiredObject, mapper)
//	if err != nil {
//		return nil, err
//	}
//
//	// get desired values to update
//	desiredUpstreamHost, found, err := unstructured.NestedString(desiredObject.Object, "spec", "upstreamHost")
//	if err != nil || !found || desiredUpstreamHost == "" {
//		return nil, fmt.Errorf("failed to extract upstream host from desired forward proxy object: %w", err)
//	}
//	desiredUpstreamPath, found, err := unstructured.NestedString(desiredObject.Object, "spec", "upstreamPath")
//	if err != nil || !found || desiredUpstreamPath == "" {
//		return nil, fmt.Errorf("failed to extract upstream path from desired forward proxy object: %w", err)
//	}
//
//	// retrieve the existing resource
//	existing, err := kubeClient.
//		Resource(mapping.Resource).
//		Namespace(desiredObject.GetNamespace()).
//		Get(context.TODO(), desiredObject.GetName(), metav1.GetOptions{})
//
//	// update upstream host and path
//	if err := unstructured.SetNestedField(
//		existing.Object,
//		desiredUpstreamHost,
//		"spec", "upstreamHost",
//	); err != nil {
//		return nil, fmt.Errorf("failed to set upstream host on forward proxy object: %w", err)
//	}
//	if err := unstructured.SetNestedField(
//		existing.Object,
//		desiredUpstreamPath,
//		"spec", "upstreamPath",
//	); err != nil {
//		return nil, fmt.Errorf("failed to set upstream path on forward proxy object: %w", err)
//	}
//
//	// update the resource
//	result, err := kubeClient.Resource(mapping.Resource).
//		Namespace(desiredObject.GetNamespace()).
//		Update(context.TODO(), existing, metav1.UpdateOptions{})
//	if err != nil {
//		return nil, fmt.Errorf("failed to update forward proxy resource: %w", err)
//	}
//
//	return result, nil
//}
