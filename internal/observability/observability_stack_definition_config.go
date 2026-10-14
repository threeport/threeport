package observability

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ObservabilityStackDefinitionConfig contains the configuration for an observability dashboard
// reconcile function.
type ObservabilityStackDefinitionConfig struct {
	r                            *controller.Reconciler
	observabilityStackDefinition *v0.ObservabilityStackDefinition
	log                          *logr.Logger
}

// getObservabilityStackDefinitionOperations returns the operations for an observability
// stack definition
func (c *ObservabilityStackDefinitionConfig) getObservabilityStackDefinitionOperations() *util.Operations {
	operations := util.Operations{}

	// append observability dashboard definition operations
	operations.AppendOperation(util.Operation{
		Name:   "observability dashboard",
		Create: c.createObservabilityDashboardDefinition,
		Delete: c.deleteObservabilityDashboardDefinition,
	})

	// append logging definition operations
	operations.AppendOperation(util.Operation{
		Name:   "logging",
		Create: c.createLoggingDefinition,
		Delete: c.deleteLoggingDefinition,
	})

	// append metrics definition operations
	operations.AppendOperation(util.Operation{
		Name:   "metrics",
		Create: c.createMetricsDefinition,
		Delete: c.deleteMetricsDefinition,
	})

	return &operations
}

// createObservabilityDashboardDefinition creates an observability dashboard definition.
func (c *ObservabilityStackDefinitionConfig) createObservabilityDashboardDefinition() error {
	// create observability dashboard definition
	observabilityDashboardDefinition := &v0.ObservabilityDashboardDefinition{
		Common: v0.Common{
			Owner: c.observabilityStackDefinition.OwnObservabilityDashboardDefinition(),
		},
		Definition: v0.Definition{
			Name: util.Ptr(ObservabilityDashboardName(*c.observabilityStackDefinition.Name)),
		},
	}

	// set grafana helm chart version
	observabilityDashboardDefinition.GrafanaHelmChartVersion = c.observabilityStackDefinition.GrafanaHelmChartVersion

	// set grafana helm chart values
	observabilityDashboardDefinition.GrafanaHelmValuesDocument = c.observabilityStackDefinition.GrafanaHelmValuesDocument

	// create observability dashboard definition
	_, err := client.CreateObservabilityDashboardDefinition(
		c.r.APIClient,
		c.r.APIServer,
		observabilityDashboardDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create observability dashboard definition: %w", err)
	}

	return nil
}

// deleteObservabilityDashboardDefinition deletes an observability dashboard definition.
func (c *ObservabilityStackDefinitionConfig) deleteObservabilityDashboardDefinition() error {
	if c.observabilityStackDefinition.ObservabilityDashboardDefinitionID == nil {
		current, err := client.GetObservabilityStackDefinitionByID(c.r.APIClient, c.r.APIServer, *c.observabilityStackDefinition.ID)
		if err != nil {
			return fmt.Errorf("failed to get observability stack definition: %w", err)
		}
		c.observabilityStackDefinition.ObservabilityDashboardDefinitionID = current.ObservabilityDashboardDefinitionID
	}

	// delete observability dashboard definition
	if _, err := client.DeleteObservabilityDashboardDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.ObservabilityDashboardDefinitionID,
	); err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete observability dashboard definition: %w", err)
	}

	return nil
}

// createLoggingDefinition creates a logging definition.
func (c *ObservabilityStackDefinitionConfig) createLoggingDefinition() error {
	// create logging definition
	loggingDefinition := &v0.LoggingDefinition{
		Common: v0.Common{
			Owner: c.observabilityStackDefinition.OwnLoggingDefinition(),
		},
		Definition: v0.Definition{
			Name: util.Ptr(LoggingName(*c.observabilityStackDefinition.Name)),
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
	_, err := client.CreateLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		loggingDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create logging definition: %w", err)
	}

	return nil
}

// deleteLoggingDefinition deletes a logging definition.
func (c *ObservabilityStackDefinitionConfig) deleteLoggingDefinition() error {
	if c.observabilityStackDefinition.LoggingDefinitionID == nil {
		current, err := client.GetObservabilityStackDefinitionByID(c.r.APIClient, c.r.APIServer, *c.observabilityStackDefinition.ID)
		if err != nil {
			return fmt.Errorf("failed to get observability stack definition: %w", err)
		}
		c.observabilityStackDefinition.LoggingDefinitionID = current.LoggingDefinitionID
	}

	// delete logging definition
	if _, err := client.DeleteLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.LoggingDefinitionID,
	); err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging definition: %w", err)
	}

	return nil
}

// createMetricsDefinition creates a metrics definition.
func (c *ObservabilityStackDefinitionConfig) createMetricsDefinition() error {
	// create metrics definition
	metricsDefinition := &v0.MetricsDefinition{
		Common: v0.Common{
			Owner: c.observabilityStackDefinition.OwnMetricsDefinition(),
		},
		Definition: v0.Definition{
			Name: util.Ptr(MetricsName(*c.observabilityStackDefinition.Name)),
		},
	}

	// set kube-prometheus-stack helm chart version
	metricsDefinition.KubePrometheusStackHelmChartVersion = c.observabilityStackDefinition.KubePrometheusStackHelmChartVersion

	// set kube-prometheus-stack helm chart values
	metricsDefinition.KubePrometheusStackHelmValuesDocument = c.observabilityStackDefinition.KubePrometheusStackHelmValuesDocument

	// create metrics definition
	_, err := client.CreateMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		metricsDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create metrics definition: %w", err)
	}

	return nil
}

// deleteMetricsDefinition deletes a metrics definition.
func (c *ObservabilityStackDefinitionConfig) deleteMetricsDefinition() error {
	if c.observabilityStackDefinition.MetricsDefinitionID == nil {
		current, err := client.GetObservabilityStackDefinitionByID(c.r.APIClient, c.r.APIServer, *c.observabilityStackDefinition.ID)
		if err != nil {
			return fmt.Errorf("failed to get observability stack definition: %w", err)
		}
		c.observabilityStackDefinition.MetricsDefinitionID = current.MetricsDefinitionID
	}

	// delete metrics definition
	if _, err := client.DeleteMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackDefinition.MetricsDefinitionID,
	); err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics definition: %w", err)
	}

	return nil
}
