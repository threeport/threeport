package observability

import (
	"fmt"

	"github.com/go-logr/logr"
	helmworkload "github.com/threeport/threeport/internal/helm-workload"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// loggingInstanceCreated reconciles state for a created
// logging instance.
func loggingInstanceCreated(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	// get logging definition
	loggingDefinition, err := client.GetLoggingDefinitionByID(
		r.APIClient,
		r.APIServer,
		*loggingInstance.LoggingDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get logging definition: %w", err)
	}
	if !*loggingDefinition.Reconciled {
		return 0, fmt.Errorf("logging definition is not reconciled")
	}

	// generate shared namespace name for loki and promtail
	loggingNamespace := fmt.Sprintf("%s-logging-%s", *loggingInstance.Name, util.RandomAlphaString(10))

	// create logging instance config
	c := &LoggingInstanceConfig{
		r:                 r,
		loggingInstance:   loggingInstance,
		loggingDefinition: loggingDefinition,
		log:               log,
		loggingNamespace:  loggingNamespace,
	}

	// merge loki helm values
	c.lokiHelmWorkloadInstanceValues, err = helmworkload.MergeHelmValuesPtrs(
		loggingDefinition.LokiHelmValuesDocument,
		loggingInstance.LokiHelmValuesDocument,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// merge promtail helm values
	c.promtailHelmWorkloadInstanceValues, err = helmworkload.MergeHelmValuesPtrs(
		loggingDefinition.PromtailHelmValuesDocument,
		loggingInstance.PromtailHelmValuesDocument,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// execute logging instance create operations
	if err := c.getLoggingInstanceOperations().Create(); err != nil {
		return 0, fmt.Errorf("failed to execute logging instance create operations: %w", err)
	}

	// update logging instance
	loggingInstance.Reconciled = util.Ptr(true)
	if _, err = client.UpdateLoggingInstance(
		r.APIClient,
		r.APIServer,
		loggingInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update logging instance: %w", err)
	}

	return 0, nil
}

// loggingInstanceUpdated reconciles state for an
// updated logging instance.
func loggingInstanceUpdated(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// loggingInstanceDeleted reconciles state for a deleted
// logging instance.
func loggingInstanceDeleted(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	// create logging instance config
	c := &LoggingInstanceConfig{
		r:                 r,
		loggingInstance:   loggingInstance,
		loggingDefinition: nil,
		log:               log,
	}

	// execute delete logging instance operations
	if err := c.getLoggingInstanceOperations().Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging delete operations: %w", err)
	}

	return 0, nil
}
