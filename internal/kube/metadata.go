package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const KubeManagedByLabel = "threeport"

// SetLabels sets the labels used by threeport to identify managed resources in
// Kubernetes.
func SetLabels(
	kubeObject *unstructured.Unstructured,
	workloadDefName string,
	workloadInstName string,
) (*unstructured.Unstructured, error) {
	labels := map[string]interface{}{
		"app.kubernetes.io/managed-by": KubeManagedByLabel,
		"app.kubernetes.io/name":       workloadDefName,
		"app.kubernetes.io/instance":   workloadInstName,
	}

	if err := unstructured.SetNestedMap(
		kubeObject.Object,
		labels,
		"metadata",
		"labels",
	); err != nil {
		return nil, fmt.Errorf("failed to apply labels to kubernetes object: %w", err)
	}

	return kubeObject, nil
}
