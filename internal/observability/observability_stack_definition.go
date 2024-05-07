package observability

import (
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// observabilityStackDefinitionCreated reconciles state for a created
// observability stack definition.
func observabilityStackDefinitionCreated(
	r *controller.Reconciler,
	observabilityStackDefinition *v0.ObservabilityStackDefinition,
	log *logr.Logger,
) (int64, error) {
	// create observability stack definition config
	c := &ObservabilityStackDefinitionConfig{
		r:                            r,
		observabilityStackDefinition: observabilityStackDefinition,
		log:                          log,
	}

	// execute observability stack definition create operations
	if err := c.getObservabilityStackDefinitionOperations().Create(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack create operations: %w", err)
	}

	// update observability stack definition
	observabilityStackDefinition.Reconciled = util.Ptr(true)
	if _, err := client.UpdateObservabilityStackDefinition(
		r.APIClient,
		r.APIServer,
		observabilityStackDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update observability stack definition: %w", err)
	}

	return 0, nil
}

// observabilityStackDefinitionUpdated reconciles state for an updated
// observability stack definition.
func observabilityStackDefinitionUpdated(
	r *controller.Reconciler,
	observabilityStackDefinition *v0.ObservabilityStackDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackDefinitionDeleted reconciles state for a deleted
// observability stack definition.
func observabilityStackDefinitionDeleted(
	r *controller.Reconciler,
	observabilityStackDefinition *v0.ObservabilityStackDefinition,
	log *logr.Logger,
) (int64, error) {
	// create observability stack config
	c := &ObservabilityStackDefinitionConfig{
		r:                            r,
		observabilityStackDefinition: observabilityStackDefinition,
		log:                          log,
	}

	// execute observability stack definition delete operations
	if err := c.getObservabilityStackDefinitionOperations().Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack delete operations: %w", err)
	}

	return 0, nil
}
