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

// Helm configuration to configure Grafana
// for prometheus metrics scraping. This is
// passed in to the observability dashboard definition
// when metrics are enabled.
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

// ObservabilityStackDefinitionConfig contains the configuration for an observability dashboard
// reconcile function.
type ObservabilityStackDefinitionConfig struct {
	r                            *controller.Reconciler
	observabilityStackDefinition *v0.ObservabilityStackDefinition
	log                          *logr.Logger
}

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

	// get observability stack operations
	operations := c.getObservabilityStackDefinitionOperations()

	// execute observability stack operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack operations: %w", err)
	}

	// update metrics instance
	observabilityStackDefinition.Reconciled = util.BoolPtr(true)
	if _, err := client.UpdateObservabilityStackDefinition(
		r.APIClient,
		r.APIServer,
		observabilityStackDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update observability stack definition: %w", err)
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
	operations := c.getObservabilityStackDefinitionOperations()

	// execute observability stack operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack operations: %w", err)
	}

	return 0, nil
}

// getObservabilityStackDefinitionOperations returns the operations for an observability
// stack definition
func (c *ObservabilityStackDefinitionConfig) getObservabilityStackDefinitionOperations() *util.Operations {
	operations := util.Operations{}

	// append observability dashboard operations
	operations.AppendOperation(util.Operation{
		Name:   "observability dashboard",
		Create: func() error { return c.createObservabilityDashboardDefinition() },
		Delete: func() error { return c.deleteObservabilityDashboardDefinition() },
	})

	// append logging operations
	operations.AppendOperation(util.Operation{
		Name:   "logging",
		Create: func() error { return c.createLoggingDefinition() },
		Delete: func() error { return c.deleteLoggingDefinition() },
	})

	// append metrics operations
	operations.AppendOperation(util.Operation{
		Name:   "metrics",
		Create: func() error { return c.createMetricsDefinition() },
		Delete: func() error { return c.deleteMetricsDefinition() },
	})

	return &operations
}

// createObservabilityDashboardDefinition creates an observability dashboard definition.
func (c *ObservabilityStackDefinitionConfig) createObservabilityDashboardDefinition() error {
	// create observability dashboard definition
	observabilityDashboardDefinition := &v0.ObservabilityDashboardDefinition{
		Definition: v0.Definition{
			Name: util.StringPtr(ObservabilityDashboardName(*c.observabilityStackDefinition.Name)),
		},
	}

	// set grafana helm chart version
	observabilityDashboardDefinition.GrafanaHelmChartVersion = c.observabilityStackDefinition.GrafanaHelmChartVersion

	// set grafana helm chart values
	observabilityDashboardDefinition.GrafanaHelmValuesDocument = c.observabilityStackDefinition.GrafanaHelmValuesDocument

	// create observability dashboard definition
	createdObservabilityDashboardDefinition, err := client.CreateObservabilityDashboardDefinition(
		c.r.APIClient,
		c.r.APIServer,
		observabilityDashboardDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create observability dashboard definition: %w", err)
	}

	// update observability stack definition with observability dashboard definition id
	c.observabilityStackDefinition.ObservabilityDashboardDefinitionID = createdObservabilityDashboardDefinition.ID

	return nil
}

// deleteObservabilityDashboardDefinition deletes an observability dashboard definition.
func (c *ObservabilityStackDefinitionConfig) deleteObservabilityDashboardDefinition() error {
	// delete observability dashboard definition
	if _, err := client.DeleteObservabilityDashboardDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.ObservabilityDashboardDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete observability dashboard definition: %w", err)
	}

	return nil
}

// createLoggingDefinition creates a logging definition.
func (c *ObservabilityStackDefinitionConfig) createLoggingDefinition() error {
	// create logging definition
	loggingDefinition := &v0.LoggingDefinition{
		Definition: v0.Definition{
			Name: util.StringPtr(LoggingName(*c.observabilityStackDefinition.Name)),
		},
	}

	// set promtail helm chart version
	loggingDefinition.PromtailHelmChartVersion = c.observabilityStackDefinition.PromtailHelmChartVersion

	// set promtail helm chart values
	loggingDefinition.PromtailHelmValuesDocument = c.observabilityStackDefinition.PromtailHelmValuesDocument

	// set loki helm chart version
	loggingDefinition.LokiHelmChartVersion = c.observabilityStackDefinition.LokiHelmChartVersion

	// set loki helm chart values
	loggingDefinition.LokiHelmValuesDocument = c.observabilityStackDefinition.LokiHelmValuesDocument

	// create logging definition
	createdLoggingDefinition, err := client.CreateLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		loggingDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create logging definition: %w", err)
	}

	// update observability stack definition with logging definition id
	c.observabilityStackDefinition.LoggingDefinitionID = createdLoggingDefinition.ID

	return nil
}

// deleteLoggingDefinition deletes a logging definition.
func (c *ObservabilityStackDefinitionConfig) deleteLoggingDefinition() error {
	// delete logging definition
	if _, err := client.DeleteLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.LoggingDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging definition: %w", err)
	}

	return nil
}

// createMetricsDefinition creates a metrics definition.
func (c *ObservabilityStackDefinitionConfig) createMetricsDefinition() error {
	// create metrics definition
	metricsDefinition := &v0.MetricsDefinition{
		Definition: v0.Definition{
			Name: util.StringPtr(MetricsName(*c.observabilityStackDefinition.Name)),
		},
	}

	// set kube-prometheus-stack helm chart version
	metricsDefinition.KubePrometheusStackHelmChartVersion = c.observabilityStackDefinition.KubePrometheusStackHelmChartVersion

	// set kube-prometheus-stack helm chart values
	metricsDefinition.KubePrometheusStackHelmValuesDocument = c.observabilityStackDefinition.KubePrometheusStackHelmValuesDocument

	// create metrics definition
	createdMetricsDefinition, err := client.CreateMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		metricsDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create metrics definition: %w", err)
	}

	// update observability stack definition with metrics definition id
	c.observabilityStackDefinition.MetricsDefinitionID = createdMetricsDefinition.ID

	return nil
}

// deleteMetricsDefinition deletes a metrics definition.
func (c *ObservabilityStackDefinitionConfig) deleteMetricsDefinition() error {
	// delete metrics definition
	if _, err := client.DeleteMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.MetricsDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics definition: %w", err)
	}

	return nil
}
