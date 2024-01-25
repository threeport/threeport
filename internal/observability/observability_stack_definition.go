package observability

import (
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const grafanaPrometheusServiceMonitor = `
serviceMonitor:
  # If true, a ServiceMonitor CRD is created for a prometheus operator
  # https://github.com/coreos/prometheus-operator
  #
  enabled: true

  # Scrape interval. If not set, the Prometheus default scrape interval is used.
  #
  interval: ""
`

// ObservabilityStackDefinitionConfig contains the configuration for a observability dashboard
// reconciler.
type ObservabilityStackDefinitionConfig struct {
	r                            *controller.Reconciler
	observabilityStackDefinition *v0.ObservabilityStackDefinition
	log                          *logr.Logger
}

// observabilityStackDefinitionCreated reconciles state for a new kubernetes
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

	// get observability stack operations
	operations := getObservabilityStackDefinitionOperations(c)

	// execute observability stack operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack operations: %w", err)
	}

	// update metrics instance reconciled field
	observabilityStackDefinition.Reconciled = util.BoolPtr(true)
	if _, err := client.UpdateObservabilityStackDefinition(
		r.APIClient,
		r.APIServer,
		observabilityStackDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics definition reconciled field: %w", err)
	}

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
	// create observability stack config
	c := &ObservabilityStackDefinitionConfig{
		r:                            r,
		observabilityStackDefinition: observabilityStackDefinition,
		log:                          log,
	}

	// get observability stack operations
	operations := getObservabilityStackDefinitionOperations(c)

	// execute observability stack operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack operations: %w", err)
	}

	return 0, nil
}

// getObservabilityDashboardOperations returns the operations for a observability
// dashboard
func getObservabilityStackDefinitionOperations(c *ObservabilityStackDefinitionConfig) *util.Operations {

	operations := util.Operations{}

	// append observability dashboard operations
	operations.AppendOperation(util.Operation{
		Name: "observability dashboard",
		Create: func() error {
			if err := c.createObservabilityDashboardDefinition(); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteObservabilityDashboardDefinition(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append logging operations
	operations.AppendOperation(util.Operation{
		Name: "logging",
		Create: func() error {
			if err := c.createLoggingDefinition(); err != nil {
				return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteLoggingDefinition(); err != nil {
				return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append metrics operations
	operations.AppendOperation(util.Operation{
		Name: "metrics",
		Create: func() error {
			if err := c.createMetricsDefinition(); err != nil {
				return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteMetricsDefinition(); err != nil {
				return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}

// createObservabilityDashboardDefinition creates an observability dashboard definition.
func (c *ObservabilityStackDefinitionConfig) createObservabilityDashboardDefinition() error {
	// create observability dashboard definition
	observabilityDashboardDefinition, err := client.CreateObservabilityDashboardDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.ObservabilityDashboardDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(ObservabilityDashboardName(*c.observabilityStackDefinition.Name)),
			},
		})
	if err != nil {
		return fmt.Errorf("failed to create observability dashboard definition: %w", err)
	}

	// update observability stack definition with observability dashboard definition id
	c.observabilityStackDefinition.ObservabilityDashboardDefinitionID = observabilityDashboardDefinition.ID

	return nil
}

// deleteObservabilityDashboardDefinition deletes an observability dashboard definition.
func (c *ObservabilityStackDefinitionConfig) deleteObservabilityDashboardDefinition() error {
	// delete observability dashboard definition
	if _, err := client.DeleteObservabilityDashboardDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.ObservabilityDashboardDefinitionID,
	); err != nil {
		return fmt.Errorf("failed to delete observability dashboard definition: %w", err)
	}

	return nil
}

// createLoggingDefinition creates a logging definition.
func (c *ObservabilityStackDefinitionConfig) createLoggingDefinition() error {
	// create logging definition
	loggingDefinition, err := client.CreateLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.LoggingDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(LoggingName(*c.observabilityStackDefinition.Name)),
			},
		})
	if err != nil {
		return fmt.Errorf("failed to create logging definition: %w", err)
	}

	// update observability stack definition with logging definition id
	c.observabilityStackDefinition.LoggingDefinitionID = loggingDefinition.ID

	return nil
}

// deleteLoggingDefinition deletes a logging definition.
func (c *ObservabilityStackDefinitionConfig) deleteLoggingDefinition() error {
	// delete logging definition
	if _, err := client.DeleteLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.LoggingDefinitionID,
	); err != nil {
		return fmt.Errorf("failed to delete logging definition: %w", err)
	}

	return nil
}

// createMetricsDefinition creates a metrics definition.
func (c *ObservabilityStackDefinitionConfig) createMetricsDefinition() error {
	// create metrics definition
	metricsDefinition, err := client.CreateMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.MetricsDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(MetricsName(*c.observabilityStackDefinition.Name)),
			},
		})
	if err != nil {
		return fmt.Errorf("failed to create metrics definition: %w", err)
	}

	// update observability stack definition with metrics definition id
	c.observabilityStackDefinition.MetricsDefinitionID = metricsDefinition.ID

	return nil
}

// deleteMetricsDefinition deletes a metrics definition.
func (c *ObservabilityStackDefinitionConfig) deleteMetricsDefinition() error {
	// delete metrics definition
	if _, err := client.DeleteMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.MetricsDefinitionID,
	); err != nil {
		return fmt.Errorf("failed to delete metrics definition: %w", err)
	}

	return nil
}
