package radiusworkload

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// radiusWorkloadDefinitionCreated reconciles state for a new radius workload
// definition.
func radiusWorkloadDefinitionCreated(
	r *controller.Reconciler,
	radiusWorkloadDefinition *v0.RadiusWorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// radiusWorkloadDefinitionCreated reconciles state for a radius workload
// definition whenever it is changed.
func radiusWorkloadDefinitionUpdated(
	r *controller.Reconciler,
	radiusWorkloadDefinition *v0.RadiusWorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// radiusWorkloadDefinitionCreated reconciles state for a radius workload
// definition whenever it is removed.
func radiusWorkloadDefinitionDeleted(
	r *controller.Reconciler,
	radiusWorkloadDefinition *v0.RadiusWorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
