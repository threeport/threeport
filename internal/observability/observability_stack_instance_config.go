package observability

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	helmworkload "github.com/threeport/threeport/internal/helm-workload"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ObservabilityStackInstanceConfig contains the configuration for
// an observability stack instance reconcile function.
type ObservabilityStackInstanceConfig struct {
	r                                     *controller.Reconciler
	observabilityStackInstance            *v0.ObservabilityStackInstance
	observabilityStackDefinition          *v0.ObservabilityStackDefinition
	log                                   *logr.Logger
	grafanaHelmValuesDocument             string
	kubePrometheusStackHelmValuesDocument string
	lokiHelmValuesDocument                string
	promtailHelmValuesDocument            string
}

// getObservabilityStackInstanceOperations returns the operations
// for an observabiblity stack instance
func (c *ObservabilityStackInstanceConfig) getObservabilityStackInstanceOperations() *util.Operations {
	operations := util.Operations{}

	// append observability dashboard operations
	operations.AppendOperation(util.Operation{
		Name:   "observability dashboard",
		Create: c.createObservabilityDashboardInstance,
		Delete: c.deleteObservabilityDashboardInstance,
	})

	if *c.observabilityStackInstance.LoggingEnabled {
		// append logging operations
		operations.AppendOperation(util.Operation{
			Name:   "logging",
			Create: c.createLoggingInstance,
			Delete: c.deleteLoggingInstance,
		})
	}

	if *c.observabilityStackInstance.MetricsEnabled {
		// append metrics operations
		operations.AppendOperation(util.Operation{
			Name:   "metrics",
			Create: c.createMetricsInstance,
			Delete: c.deleteMetricsInstance,
		})
	}

	return &operations
}

// createObservabilityDashboardInstance creates an observability dashboard instance
func (c *ObservabilityStackInstanceConfig) createObservabilityDashboardInstance() error {
	// create observability dashboard instance
	observabilityDashboardInstance, err := client.CreateObservabilityDashboardInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.ObservabilityDashboardInstance{
			Instance: v0.Instance{
				Name: util.Ptr(ObservabilityDashboardName(*c.observabilityStackInstance.Name)),
			},
			KubernetesRuntimeInstanceID:        c.observabilityStackInstance.KubernetesRuntimeInstanceID,
			ObservabilityDashboardDefinitionID: c.observabilityStackDefinition.ObservabilityDashboardDefinitionID,
			GrafanaHelmValuesDocument:          &c.grafanaHelmValuesDocument,
		})
	if err != nil {
		return fmt.Errorf("failed to create observability dashboard instance: %w", err)
	}

	// update observability dashboard instance id
	c.observabilityStackInstance.ObservabilityDashboardInstanceID = observabilityDashboardInstance.ID

	return nil
}

// deleteObservabilityDashboardInstance deletes an observability dashboard instance
func (c *ObservabilityStackInstanceConfig) deleteObservabilityDashboardInstance() error {
	// delete observability dashboard instance
	if _, err := client.DeleteObservabilityDashboardInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackInstance.ObservabilityDashboardInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete observability dashboard instance: %w", err)
	}

	return nil
}

// createMetricsInstance creates a metrics instance
func (c *ObservabilityStackInstanceConfig) createMetricsInstance() error {
	// create metrics instance
	metricsInstance, err := client.CreateMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.MetricsInstance{
			Instance: v0.Instance{
				Name: util.Ptr(MetricsName(*c.observabilityStackInstance.Name)),
			},
			KubernetesRuntimeInstanceID:           c.observabilityStackInstance.KubernetesRuntimeInstanceID,
			MetricsDefinitionID:                   c.observabilityStackDefinition.MetricsDefinitionID,
			KubePrometheusStackHelmValuesDocument: &c.kubePrometheusStackHelmValuesDocument,
		})
	if err != nil {
		return fmt.Errorf("failed to create metrics instance: %w", err)
	}

	// update metrics instance id
	c.observabilityStackInstance.MetricsInstanceID = metricsInstance.ID

	return nil
}

// deleteMetricsInstance deletes a metrics instance
func (c *ObservabilityStackInstanceConfig) deleteMetricsInstance() error {
	// delete metrics instance
	if _, err := client.DeleteMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackInstance.MetricsInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics instance: %w", err)
	}

	return nil
}

// createLoggingInstance creates a logging instance
func (c *ObservabilityStackInstanceConfig) createLoggingInstance() error {
	// create logging instance
	loggingInstance, err := client.CreateLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.LoggingInstance{
			Instance: v0.Instance{
				Name: util.Ptr(LoggingName(*c.observabilityStackInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.observabilityStackInstance.KubernetesRuntimeInstanceID,
			LoggingDefinitionID:         c.observabilityStackDefinition.LoggingDefinitionID,
			LokiHelmValuesDocument:      &c.lokiHelmValuesDocument,
			PromtailHelmValuesDocument:  &c.promtailHelmValuesDocument,
		})
	if err != nil {
		return fmt.Errorf("failed to create logging instance: %w", err)
	}

	// update logging instance id
	c.observabilityStackInstance.LoggingInstanceID = loggingInstance.ID

	return nil
}

// deleteLoggingInstance deletes a logging instance
func (c *ObservabilityStackInstanceConfig) deleteLoggingInstance() error {
	// delete logging instance
	if _, err := client.DeleteLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackInstance.LoggingInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging instance: %w", err)
	}

	return nil
}

// setMergedObservabilityStackInstanceValues sets the merged values for
// an observability stack instance
func (c *ObservabilityStackInstanceConfig) setMergedObservabilityStackInstanceValues() error {
	var err error

	// merge grafana values
	c.grafanaHelmValuesDocument, err = helmworkload.MergeHelmValuesPtrs(
		c.observabilityStackInstance.GrafanaHelmValuesDocument,
		c.observabilityStackDefinition.GrafanaHelmValuesDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to merge grafana helm values: %w", err)
	}

	// Only configure grafana service monitor if metrics are enabled, as
	// this depends on the ServiceMonitor CRD being installed in the cluster
	// and the kube-prometheus-stack being installed to scrape its metrics.
	if *c.observabilityStackInstance.MetricsEnabled {
		// merge grafana prometheus service monitor
		c.grafanaHelmValuesDocument, err = helmworkload.MergeHelmValuesString(
			c.grafanaHelmValuesDocument,
			grafanaPrometheusServiceMonitor,
		)
		if err != nil {
			return fmt.Errorf("failed to merge grafana prometheus service monitor: %w", err)
		}
	}

	// merge kube-prometheus-stack values
	c.kubePrometheusStackHelmValuesDocument, err = helmworkload.MergeHelmValuesPtrs(
		c.observabilityStackInstance.KubePrometheusStackHelmValuesDocument,
		c.observabilityStackDefinition.KubePrometheusStackHelmValuesDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to merge kube-prometheus-stack helm values: %w", err)
	}

	// merge loki values
	c.lokiHelmValuesDocument, err = helmworkload.MergeHelmValuesPtrs(
		c.observabilityStackInstance.LokiHelmValuesDocument,
		c.observabilityStackDefinition.LokiHelmValuesDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// merge promtail values
	c.promtailHelmValuesDocument, err = helmworkload.MergeHelmValuesPtrs(
		c.observabilityStackInstance.PromtailHelmValuesDocument,
		c.observabilityStackDefinition.PromtailHelmValuesDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to merge promtail helm values: %w", err)
	}

	return nil
}
