package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/agent"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const KubeManagedByLabel = "threeport"

// AddLabels sets the labels used by threeport to identify managed resources in
// Kubernetes.
func AddLabels(
	kubeObject *unstructured.Unstructured,
	workloadDefName string,
	workloadInst *v0.WorkloadInstance,
) (*unstructured.Unstructured, error) {
	newLabels := map[string]string{
		"app.kubernetes.io/managed-by": KubeManagedByLabel,
		"app.kubernetes.io/name":       workloadDefName,
		"app.kubernetes.io/instance":   *workloadInst.Name,
		agent.WorkloadInstanceLabelKey: fmt.Sprintf("%d", *workloadInst.ID),
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

	for _, kind := range getPodAbstractionKinds() {
		if kubeObject.GetKind() == kind {
			obj, err := setPodTemplateLabel(kubeObject, *workloadInst.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to set pod template label: %w", err)
			}
			kubeObject = obj
			break
		}
	}

	return kubeObject, nil
}

func getPodAbstractionKinds() []string {
	return []string{
		"Job",
		"ReplicaSet",
		"Deployment",
		"StatefulSet",
		"DaemonSet",
	}
}

// setPodTemplateLabel sets a label on the pod template for a Deployment,
// StatefulSet or DaemonSet.
func setPodTemplateLabel(kubeObject *unstructured.Unstructured, workloadInstID uint) (*unstructured.Unstructured, error) {
	podLabels, _, err := unstructured.NestedStringMap(kubeObject.Object, "spec", "template", "metadata", "labels")
	if err != nil {
		return nil, err
	}
	podLabels[agent.WorkloadInstanceLabelKey] = fmt.Sprintf("%d", workloadInstID)

	if err := unstructured.SetNestedStringMap(kubeObject.Object, podLabels, "spec", "template", "metadata", "labels"); err != nil {
		return nil, err
	}

	return kubeObject, nil
}
