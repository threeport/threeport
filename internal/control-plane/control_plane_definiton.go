package controlplane

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// controlPlaneDefinitionCreated reconciles state for a new control plane
// definition.
func controlPlaneDefinitionCreated(
	r *controller.Reconciler,
	controlPlaneDefintion *v0.ControlPlaneDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// controlPlaneDefinitionUpdated reconciles state for a updated control plane
// definition.
func controlPlaneDefinitionUpdated(
	r *controller.Reconciler,
	controlPlaneDefintion *v0.ControlPlaneDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// controlPlaneDefinitionDeleted reconciles state for a updated control plane
// definition.
func controlPlaneDefinitionDeleted(
	r *controller.Reconciler,
	controlPlaneDefintion *v0.ControlPlaneDefinition,
	log *logr.Logger,
) (int64, error) {

	// delete the workload definition that was scheduled for deletion
	deletionReconciled := true
	deletionTimestamp := time.Now().UTC()
	deletedControlPlaneDefinition := v0.ControlPlaneDefinition{
		Common: v0.Common{
			ID: controlPlaneDefintion.ID,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled:           &deletionReconciled,
			DeletionAcknowledged: &deletionTimestamp,
			DeletionConfirmed:    &deletionTimestamp,
		},
	}
	_, err := client.UpdateControlPlaneDefinition(
		r.APIClient,
		r.APIServer,
		&deletedControlPlaneDefinition,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to confirm deletion of control plane definition in threeport API: %w", err)
	}
	_, err = client.DeleteControlPlaneDefinition(
		r.APIClient,
		r.APIServer,
		*controlPlaneDefintion.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete control plane definition in threeport API: %w", err)
	}

	return 0, nil
}
