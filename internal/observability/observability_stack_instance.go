package observability

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// observabilityStackInstanceCreated reconciles state for a new kubernetes
// observability stack instance.
func observabilityStackInstanceCreated(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackInstanceUpdated reconciles state for an updated kubernetes
// observability stack instance.
func observabilityStackInstanceUpdated(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackInstanceDeleted reconciles state for a deleted kubernetes
// observability stack instance.
func observabilityStackInstanceDeleted(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
