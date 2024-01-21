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

// LoggingInstanceConfig contains the configuration for a logging instance.
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

	// merge grafana helm values if they are provided
	grafanaHelmWorkloadInstanceValues := grafanaValues
	if loggingInstance.GrafanaHelmValues != nil {
		if grafanaHelmWorkloadInstanceValues, err = MergeHelmValues(
			grafanaValues,
			*loggingInstance.GrafanaHelmValues,
		); err != nil {
			return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
		}
	}

	// merge loki helm values if they are provided
	lokiHelmWorkloadInstanceValues := lokiValues
	if loggingInstance.LokiHelmValues != nil {
		if lokiHelmWorkloadInstanceValues, err = MergeHelmValues(
			lokiValues,
			*loggingInstance.LokiHelmValues,
		); err != nil {
			return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
		}
	}

	// merge loki helm values if they are provided
	promtailHelmWorkloadInstanceValues := promtailValues
	if loggingInstance.PromtailHelmValues != nil {
		if promtailHelmWorkloadInstanceValues, err = MergeHelmValues(
			promtailValues,
			*loggingInstance.PromtailHelmValues,
		); err != nil {
			return 0, fmt.Errorf("failed to merge promtail helm values: %w", err)
		}
	}

	// create logging instance config
	c := &LoggingInstanceConfig{
		r:                                  r,
		loggingInstance:                    loggingInstance,
		loggingDefinition:                  loggingDefinition,
		log:                                log,
		loggingNamespace:                   loggingNamespace,
		grafanaHelmWorkloadInstanceValues:  grafanaHelmWorkloadInstanceValues,
		lokiHelmWorkloadInstanceValues:     lokiHelmWorkloadInstanceValues,
		promtailHelmWorkloadInstanceValues: promtailHelmWorkloadInstanceValues,
	}

	// get logging operations
	operations := getLoggingOperations(c)

	// execute logging operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute logging create operations: %w", err)
	}

	// wait for helm workload instances to be reconciled
	var hwri *v0.HelmWorkloadInstance
	for _, id := range []*uint{
		loggingInstance.GrafanaHelmWorkloadInstanceID,
		loggingInstance.LokiHelmWorkloadInstanceID,
		loggingInstance.PromtailHelmWorkloadInstanceID,
	} {
		current := id
		if err = util.Retry(60, 1, func() error {
			if hwri, err = client.GetHelmWorkloadInstanceByID(
				r.APIClient,
				r.APIServer,
				*current,
			); err != nil {
				return fmt.Errorf("failed to get helm workload instance: %w", err)
			} else if !*hwri.Reconciled {
				return fmt.Errorf("helm workload instance is not reconciled")
			}
			return nil
		}); err != nil {
			return 0, fmt.Errorf("failed to wait for helm workload instance to be reconciled: %w", err)
		}
	}

	// update logging instance reconciled field
	loggingInstance.Reconciled = util.BoolPtr(true)
	_, err = client.UpdateLoggingInstance(
		r.APIClient,
		r.APIServer,
		loggingInstance,
	)
	if err != nil {
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
	operations := getLoggingOperations(c)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging delete operations: %w", err)
	}

	return 0, nil
}

// createGrafana creates grafana helm workload instance if metrics
// instance is not deployed.
func (c *LoggingInstanceConfig) createGrafana() error {
	// ensure grafana helm workload instance is deployed
	grafanaHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(GrafanaChartName(*c.loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.loggingDefinition.GrafanaHelmWorkloadDefinitionID,
			HelmValuesDocument:          &c.grafanaHelmWorkloadInstanceValues,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	} else if err != nil && errors.Is(err, client.ErrConflict) {
		grafanaHelmWorkloadInstance, err = client.GetHelmWorkloadInstanceByName(
			c.r.APIClient,
			c.r.APIServer,
			GrafanaChartName(*c.loggingInstance.Name),
		)
		if err != nil {
			return fmt.Errorf("failed to get grafana helm workload instance: %w", err)
		}
		metricsInstance, err := client.GetMetricsInstanceByName(
			c.r.APIClient,
			c.r.APIServer,
			*c.loggingInstance.Name,
		)
		if err != nil {
			return fmt.Errorf("failed to get metrics instance: %w", err)
		}
		if metricsInstance.GrafanaHelmWorkloadInstanceID != nil &&
			*metricsInstance.GrafanaHelmWorkloadInstanceID != *grafanaHelmWorkloadInstance.ID {
			return fmt.Errorf("grafana helm workload instance already exists")
		}
	}
	c.loggingInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID
	return nil
}

// deleteGrafana deletes grafana helm workload instance if metrics
// instance is not deployed.
func (c *LoggingInstanceConfig) deleteGrafana() error {
	// check if metrics is deployed,
	// if it's not then we can clean up grafana chart
	metricsInstance, err := client.GetMetricsInstanceByName(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.Name,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to get metrics instance: %w", err)
	} else if err != nil && errors.Is(err, client.ErrObjectNotFound) ||
		(metricsInstance != nil &&
			metricsInstance.DeletionScheduled != nil) {
		// delete grafana helm workload instance
		_, err = client.DeleteHelmWorkloadInstance(
			c.r.APIClient,
			c.r.APIServer,
			*c.loggingInstance.GrafanaHelmWorkloadInstanceID,
		)
		if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
			return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
		}
	}
	return nil
}

// createLoki creates loki helm workload instance
func (c *LoggingInstanceConfig) createLoki() error {
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
	c.loggingInstance.LokiHelmWorkloadInstanceID = lokiHelmWorkloadInstance.ID
	return nil
}

// deleteLoki deletes loki helm workload instance
func (c *LoggingInstanceConfig) deleteLoki() error {

	// delete loki helm workload instance
	_, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.LokiHelmWorkloadInstanceID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
	}

	return nil
}

// createPromtail creates promtail helm workload instance
func (c *LoggingInstanceConfig) createPromtail() error {
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
	c.loggingInstance.PromtailHelmWorkloadInstanceID = promtailHelmWorkloadInstance.ID
	return nil
}

// deletePromtail creates promtail helm workload instance
func (c *LoggingInstanceConfig) deletePromtail() error {
	// delete promtail helm workload instance
	_, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.PromtailHelmWorkloadInstanceID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
	}
	return nil
}

// getLoggingOperations returns a list of operations for a logging instance.
func getLoggingOperations(c *LoggingInstanceConfig) *util.Operations {

	operations := util.Operations{}

	// append grafana operations
	operations.AppendOperation(util.Operation{
		Name: "grafana",
		Create: func() error {
			if err := c.createGrafana(); err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteGrafana(); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name: "loki",
		Create: func() error {
			if err := c.createLoki(); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteLoki(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name: "promtail",
		Create: func() error {
			if err := c.createPromtail(); err != nil {
				return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deletePromtail(); err != nil {
				return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}
