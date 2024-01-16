package observability

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// metricsInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceCreated(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// metricsInstanceUpdated reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceUpdated(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// metricsInstanceDeleted reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceDeleted(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
