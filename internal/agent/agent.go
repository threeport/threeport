package agent

import "fmt"

const WorkloadInstanceLabelKey = "control-plane.threeport.io/workload-instance"

// ThreeportWorkloadName returns a standardized name for a ThreeportWorkload
// Kubernetes custom resource based on the workload instance ID.
func ThreeportWorkloadName(workloadInstanceID uint) string {
	return fmt.Sprintf("workload-instance-%d", workloadInstanceID)
}
