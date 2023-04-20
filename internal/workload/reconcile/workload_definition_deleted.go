package reconcile

import (
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
)

// WorkloadDefinitionDeleted performs reconciliation when a workload definition
// has been deleted.
func WorkloadDefinitionDeleted(
	r *controller.Reconciler,
	workloadDefinition *v0.WorkloadDefinition,
) error {
	// get related workload resource definitions
	workloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
		*workloadDefinition.ID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource definitions by workload definition ID: %w", err)
	}

	// delete each related workload resource definition
	for _, wrd := range *workloadResourceDefinitions {
		_, err := client.DeleteWorkloadResourceDefinition(*wrd.ID, r.APIServer, "")
		if err != nil {
			return fmt.Errorf("failed to delete workload resource definition with ID %d: %w", wrd.ID, err)
		}
	}

	return nil
}
