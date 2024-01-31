package observability

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// observabilityDashboardDefinitionCreated reconciles state for a new kubernetes
// observability dashboard definition.
func observabilityDashboardDefinitionCreated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardDefinitiondUpdated reconciles state for an updated kubernetes
// observability dashboard definition.
func observabilityDashboardDefinitionUpdated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardDefinitiondDeleted reconciles state for a deleted kubernetes
// observability dashboard definition.
func observabilityDashboardDefinitionDeleted(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
