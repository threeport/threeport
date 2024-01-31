package observability

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// observabilityStackDefinitionCreated reconciles state for a new kubernetes
// observability stack definition.
func observabilityStackDefinitionCreated(
	r *controller.Reconciler,
	observabilityStackDefinition *v0.ObservabilityStackDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackDefinitionUpdated reconciles state for an updated kubernetes
// observability stack definition.
func observabilityStackDefinitionUpdated(
	r *controller.Reconciler,
	observabilityStackDefinition *v0.ObservabilityStackDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackDefinitionDeleted reconciles state for a deleted kubernetes
// observability stack definition.
func observabilityStackDefinitionDeleted(
	r *controller.Reconciler,
	observabilityStackDefinition *v0.ObservabilityStackDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
