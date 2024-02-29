package controlplane

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
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

	return 0, nil
}
