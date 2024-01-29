package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	KubeManagedByLabelValue      = "threeport"
	ThreeportManagedByLabelKey   = "control-plane.threeport.io/managed-by"
	ThreeportManagedByLabelValue = "threeport"
)

// AddLabels sets the labels used by threeport to identify managed resources in
// Kubernetes.
func AddLabels(
	kubeObject *unstructured.Unstructured,
	definitionName string,
	instanceName string,
	instanceId uint,
	instanceLabelKey string,
) (*unstructured.Unstructured, error) {
	newLabels := map[string]string{
		"app.kubernetes.io/managed-by": KubeManagedByLabelValue,
		"app.kubernetes.io/name":       definitionName,
		"app.kubernetes.io/instance":   instanceName,
		instanceLabelKey:               fmt.Sprintf("%d", instanceId),
	}

	for key, value := range newLabels {
		labels := kubeObject.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		if _, exists := labels[key]; !exists {
			labels[key] = value
		}
		kubeObject.SetLabels(labels)
	}

	// ensure the threeport managed-by label is always present
	kubeObject.SetLabels(map[string]string{ThreeportManagedByLabelKey: ThreeportManagedByLabelValue})

	for _, kind := range GetPodAbstractionKinds() {
		if kubeObject.GetKind() == kind {
			obj, err := setPodTemplateLabels(kubeObject, instanceId, instanceLabelKey)
			if err != nil {
				return nil, fmt.Errorf("failed to set pod template label: %w", err)
			}
			kubeObject = obj
			break
		}
	}

	return kubeObject, nil
}

// GetPodAbstractionKinds returns the Kuberentes kinds that manaage pods with
// templates.
func GetPodAbstractionKinds() []string {
	return []string{
		"Job",
		"ReplicaSet",
		"Deployment",
		"StatefulSet",
		"DaemonSet",
	}
}

// setPodTemplateLabels sets required labels on the pod template for a Deployment,
// StatefulSet, DaemonSet, ReplicaSet or Job.
func setPodTemplateLabels(
	kubeObject *unstructured.Unstructured,
	instanceId uint,
	instanceLabelKey string,
) (*unstructured.Unstructured, error) {
	podLabels, found, err := unstructured.NestedStringMap(kubeObject.Object, "spec", "template", "metadata", "labels")
	if err != nil {
		return nil, err
	}

	if !found {
		podLabels = make(map[string]string)
	}

	podLabels[instanceLabelKey] = fmt.Sprintf("%d", instanceId)
	podLabels[ThreeportManagedByLabelKey] = ThreeportManagedByLabelValue

	if err := unstructured.SetNestedStringMap(kubeObject.Object, podLabels, "spec", "template", "metadata", "labels"); err != nil {
		return nil, err
	}

	return kubeObject, nil
}
