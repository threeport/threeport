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

	// The label key applied to ThreeportWorkload resources to identify the
	// control plane that owns them.  ThreeportWorkload is cluster-scoped, so
	// when more than one control plane has an agent on a cluster this is what
	// keeps each agent to its own control plane's workloads.
	ControlPlaneNameLabelKey = "control-plane.threeport.io/control-plane-name"

	// The environment variable that carries the control plane's name to its
	// controllers.
	ControlPlaneNameEnvVar = "CONTROL_PLANE_NAME"
)

// ThreeportWorkloadLabels returns the labels for a ThreeportWorkload resource
// created by the named control plane.  A control plane with no name set
// returns no labels.
func ThreeportWorkloadLabels(controlPlaneName string) map[string]string {
	if controlPlaneName == "" {
		return nil
	}
	return map[string]string{ControlPlaneNameLabelKey: controlPlaneName}
}

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
