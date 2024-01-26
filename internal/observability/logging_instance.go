package observability

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// LoggingInstanceConfig contains the configuration for a logging instance
// reconciler.
type LoggingInstanceConfig struct {
	r                                  *controller.Reconciler
	loggingInstance                    *v0.LoggingInstance
	loggingDefinition                  *v0.LoggingDefinition
	log                                *logr.Logger
	loggingNamespace                   string
	grafanaHelmWorkloadInstanceValues  string
	lokiHelmWorkloadInstanceValues     string
	promtailHelmWorkloadInstanceValues string
}

// loggingInstanceCreated reconciles state for a new kubernetes
// runtime instance.
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

	// merge loki helm values
	lokiHelmWorkloadInstanceValues, err := MergeHelmValues(
		util.StringPtrToString(loggingDefinition.LokiHelmValuesDocument),
		util.StringPtrToString(loggingInstance.LokiHelmValues),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// merge promtail helm values
	promtailHelmWorkloadInstanceValues, err := MergeHelmValues(
		util.StringPtrToString(loggingDefinition.PromtailHelmValuesDocument),
		util.StringPtrToString(loggingInstance.PromtailHelmValues),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// create logging instance config
	c := &LoggingInstanceConfig{
		r:                                  r,
		loggingInstance:                    loggingInstance,
		loggingDefinition:                  loggingDefinition,
		log:                                log,
		loggingNamespace:                   loggingNamespace,
		lokiHelmWorkloadInstanceValues:     lokiHelmWorkloadInstanceValues,
		promtailHelmWorkloadInstanceValues: promtailHelmWorkloadInstanceValues,
	}

	// get logging operations
	operations := getLoggingInstanceOperations(c)

	// execute create logging instance operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute create logging instance operations: %w", err)
	}

	// update logging instance
	loggingInstance.Reconciled = util.BoolPtr(true)
	if _, err = client.UpdateLoggingInstance(
		r.APIClient,
		r.APIServer,
		loggingInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update logging instance reconciled field: %w", err)
	}

	return 0, nil
}

// loggingInstanceUpdated reconciles state for a new kubernetes
// runtime instance.
func loggingInstanceUpdated(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// loggingInstanceDeleted reconciles state for a new kubernetes
// runtime instance.
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

	// get logging operations
	operations := getLoggingInstanceOperations(c)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging delete operations: %w", err)
	}

	return 0, nil
}

// getLoggingInstanceOperations returns a list of operations for a logging instance.
func getLoggingInstanceOperations(c *LoggingInstanceConfig) *util.Operations {
	operations := util.Operations{}

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name:   "loki",
		Create: func() error { return c.createLokiHelmWorkloadInstance() },
		Delete: func() error { return c.deleteLokiHelmWorkloadInstance() },
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name:   "promtail",
		Create: func() error { return c.createPromtailHelmWorkloadInstance() },
		Delete: func() error { return c.deletePromtailHelmWorkloadInstance() },
	})

	return &operations
}

// createLokiHelmWorkloadInstance creates loki helm workload instance
func (c *LoggingInstanceConfig) createLokiHelmWorkloadInstance() error {
	// create loki helm workload instance
	lokiHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(LokiHelmChartName(*c.loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.loggingDefinition.LokiHelmWorkloadDefinitionID,
			HelmValuesDocument:          &c.lokiHelmWorkloadInstanceValues,
			HelmReleaseNamespace:        &c.loggingNamespace,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create loki helm workload instance: %w", err)
	}

	// update logging instance loki helm workload instance id
	c.loggingInstance.LokiHelmWorkloadInstanceID = lokiHelmWorkloadInstance.ID

	return nil
}

// deleteLokiHelmWorkloadInstance deletes loki helm workload instance
func (c *LoggingInstanceConfig) deleteLokiHelmWorkloadInstance() error {

	// delete loki helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.LokiHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
	}

	return nil
}

// createPromtailHelmWorkloadInstance creates promtail helm workload instance
func (c *LoggingInstanceConfig) createPromtailHelmWorkloadInstance() error {
	// create promtail helm workload instance
	promtailHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(PromtailHelmChartName(*c.loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.loggingDefinition.PromtailHelmWorkloadDefinitionID,
			HelmValuesDocument:          &c.promtailHelmWorkloadInstanceValues,
			HelmReleaseNamespace:        &c.loggingNamespace,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
	}

	// update logging instance promtail helm workload instance id
	c.loggingInstance.PromtailHelmWorkloadInstanceID = promtailHelmWorkloadInstance.ID

	return nil
}

// deletePromtailHelmWorkloadInstance creates promtail helm workload instance
func (c *LoggingInstanceConfig) deletePromtailHelmWorkloadInstance() error {
	// delete promtail helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.PromtailHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
	}

	return nil
}
