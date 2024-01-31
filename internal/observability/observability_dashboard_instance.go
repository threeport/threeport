package observability

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// observabilityDashboardInstanceCreated reconciles state for a new kubernetes
// observability dashboard definition.
func observabilityDashboardInstanceCreated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardInstanceUpdated reconciles state for an updated kubernetes
// observability dashboard definition.
func observabilityDashboardInstanceUpdated(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardInstanceDeleted reconciles state for a deleted kubernetes
// observability dashboard definition.
func observabilityDashboardInstanceDeleted(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
