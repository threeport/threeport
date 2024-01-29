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
	WorkloadInstanceType     = "WorkloadInstance"
	HelmWorkloadInstanceType = "HelmWorkloadInstance"

	// The label keys applied to workloads managed by Threeport
	WorkloadInstanceLabelKey     = "control-plane.threeport.io/workload-instance"
	HelmWorkloadInstanceLabelKey = "control-plane.threeport.io/helm-workload-instance"
)

// ThreeportWorkloadName returns a standardized name for a ThreeportWorkload
// Kubernetes custom resource based on the workload instance ID.
func ThreeportWorkloadName(
	workloadInstanceID uint,
	workloadType string,
) (string, error) {
	switch workloadType {
	case WorkloadInstanceType:
		return fmt.Sprintf("workload-instance-%d", workloadInstanceID), nil
	case HelmWorkloadInstanceType:
		return fmt.Sprintf("helm-workload-instance-%d", workloadInstanceID), nil
	default:
		return "", fmt.Errorf(
			"unrecognized workload type - recoginzed types: [%s,%s]",
			WorkloadInstanceType,
			HelmWorkloadInstanceType,
		)
	}
}
