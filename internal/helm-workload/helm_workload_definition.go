package helmworkload

import (
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// helmWorkloadDefinitionCreated reconciles state for a new helm workload
// definition.
func helmWorkloadDefinitionCreated(
	r *controller.Reconciler,
	helmWorkloadDefinition *v0.HelmWorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// helmWorkloadDefinitionCreated reconciles state for a helm workload
// definition when it is changed.
func helmWorkloadDefinitionUpdated(
	r *controller.Reconciler,
	helmWorkloadDefinition *v0.HelmWorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// helmWorkloadDefinitionCreated reconciles state for a helm workload
// definition when it is removed.
func helmWorkloadDefinitionDeleted(
	r *controller.Reconciler,
	helmWorkloadDefinition *v0.HelmWorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
