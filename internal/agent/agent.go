package agent

import "fmt"

// ThreeportWorkloadName returns a standardized name for a ThreeportWorkload
// Kubernetes custom resource based on the workload instance ID.
func ThreeportWorkloadName(workloadInstanceID uint) string {
	return fmt.Sprintf("workload-instance-%d", workloadInstanceID)
}
