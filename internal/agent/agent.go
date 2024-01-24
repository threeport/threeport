package agent

import "fmt"

const (
	ThreeportWorkloadFinalizer   = "control-plane.threeport.io/threeport-workload-finalizer"
	WorkloadInstanceLabelKey     = "control-plane.threeport.io/workload-instance"
	HelmWorkloadInstanceLabelKey = "control-plane.threeport.io/helm-workload-instance"
)

// ThreeportWorkloadName returns a standardized name for a ThreeportWorkload
// Kubernetes custom resource based on the workload instance ID.
func ThreeportWorkloadName(workloadInstanceID uint) string {
	return fmt.Sprintf("workload-instance-%d", workloadInstanceID)
}
