package gateway

import (
	"fmt"

	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// confirmWorkloadInstanceReconciled confirms whether a kubernetes workload instance
// is reconciled.
func confirmWorkloadInstanceReconciled(
	r *controller.Reconciler,
	instanceID uint,
) (bool, error) {

	// get kubernetes workload instance id
	workloadInstance, err := client.GetKubernetesWorkloadInstanceByID(r.APIClient, r.APIServer, instanceID)
	if err != nil {
		return false, fmt.Errorf("failed to get kubernetes workload instance by kubernetes workload instance ID: %w", err)
	}

	// if the kubernetes workload instance is not reconciled, return false
	if workloadInstance.Reconciled != nil && !*workloadInstance.Reconciled {
		return false, nil
	}

	return true, nil
}
