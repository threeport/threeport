package agent

import (
	"fmt"
)

const (
	// The Kubernetes finalizer applied to ThreeportWorkload resources
	ThreeportWorkloadFinalizer = "control-plane.threeport.io/threeport-workload-finalizer"

	// The workload type applied to the `.spec.workloadType` field in a
	// `ThreeportWorkload` kubernetes resource to indicate to the Threeport
	// Agent what Threeport type is managing workload resources in Kubernetes.
	KubernetesWorkloadInstanceType = "KubernetesWorkloadInstance"
	HelmWorkloadInstanceType       = "HelmWorkloadInstance"

	// The label keys applied to workloads managed by Threeport
	KubernetesWorkloadInstanceLabelKey = "control-plane.threeport.io/kubernetes-workload-instance"
	HelmWorkloadInstanceLabelKey       = "control-plane.threeport.io/helm-workload-instance"
)

// ThreeportWorkloadName returns a standardized name for a ThreeportWorkload
// Kubernetes custom resource based on the kubernetes workload instance ID.
func ThreeportWorkloadName(
	workloadInstanceID uint,
	workloadType string,
) (string, error) {
	switch workloadType {
	case KubernetesWorkloadInstanceType:
		return fmt.Sprintf("kubernetes-workload-instance-%d", workloadInstanceID), nil
	case HelmWorkloadInstanceType:
		return fmt.Sprintf("helm-workload-instance-%d", workloadInstanceID), nil
	default:
		return "", fmt.Errorf(
			"unrecognized workload type - recoginzed types: [%s,%s]",
			KubernetesWorkloadInstanceType,
			HelmWorkloadInstanceType,
		)
	}
}
