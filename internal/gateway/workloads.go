package gateway

import (
	"fmt"

	client_v1 "github.com/threeport/threeport/pkg/client/v1"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// confirmWorkloadInstanceReconciled confirms whether a workload instance
// is reconciled.
func confirmWorkloadInstanceReconciled(
	r *controller.Reconciler,
	instanceID uint,
) (bool, error) {

	// get workload instance id
	workloadInstance, err := client_v1.GetWorkloadInstanceByID(r.APIClient, r.APIServer, instanceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workload instance by workload instance ID: %w", err)
	}

	// if the workload instance is not reconciled, return false
	if workloadInstance.Reconciled != nil && !*workloadInstance.Reconciled {
		return false, nil
	}

	return true, nil
}
