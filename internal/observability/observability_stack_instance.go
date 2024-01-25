package observability

import (
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

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

// observabilityStackInstanceCreated reconciles state for a new kubernetes
// observability stack instance.
func observabilityStackInstanceCreated(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	// get observability dashboard definition
	observabilityStackDefinition, err := client.GetObservabilityStackDefinitionByID(
		r.APIClient,
		r.APIServer,
		*observabilityStackInstance.ObservabilityStackDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get observability dashboard definition: %w", err)
	}

	// get merged observability stack instance values
	grafanaHelmValuesDocument, kubePrometheusStackHelmValuesDocument, lokiHelmValuesDocument, promtailHelmValuesDocument, err := getMergedObservabilityStackInstanceValues(
		observabilityStackInstance,
		observabilityStackDefinition,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get merged observability stack instance values: %w", err)
	}

	// create observability stack instance config
	c := &ObservabilityStackInstanceConfig{
		r:                                     r,
		observabilityStackInstance:            observabilityStackInstance,
		observabilityStackDefinition:          observabilityStackDefinition,
		log:                                   log,
		grafanaHelmValuesDocument:             grafanaHelmValuesDocument,
		kubePrometheusStackHelmValuesDocument: kubePrometheusStackHelmValuesDocument,
		lokiHelmValuesDocument:                lokiHelmValuesDocument,
		promtailHelmValuesDocument:            promtailHelmValuesDocument,
	}

	// get observability stack operations
	operations := c.getObservabilityStackInstanceOperations()

	// execute observability stack create operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack operations: %w", err)
	}

	// update metrics instance reconciled field
	observabilityStackInstance.Reconciled = util.BoolPtr(true)
	if _, err := client.UpdateObservabilityStackInstance(
		r.APIClient,
		r.APIServer,
		observabilityStackInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics definition reconciled field: %w", err)
	}

	return 0, nil
}

// observabilityStackInstanceUpdated reconciles state for an updated kubernetes
// observability stack instance.
func observabilityStackInstanceUpdated(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackInstanceDeleted reconciles state for a deleted kubernetes
// observability stack instance.
func observabilityStackInstanceDeleted(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	// create observability stack instance config
	c := &ObservabilityStackInstanceConfig{
		r:                            r,
		observabilityStackInstance:   observabilityStackInstance,
		observabilityStackDefinition: nil,
		log:                          log,
	}

	// get observability stack operations
	operations := c.getObservabilityStackInstanceOperations()

	// execute observability stack delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack operations: %w", err)
	}

	return 0, nil
}

// getObservabilityInstanceOperations returns the operations for a observability
// dashboard
func (c *ObservabilityStackInstanceConfig) getObservabilityStackInstanceOperations() *util.Operations {

	operations := util.Operations{}

	// append observability dashboard operations
	operations.AppendOperation(util.Operation{
		Name: "observability dashboard",
		Create: func() error {
			if err := c.createObservabilityDashboardInstance(); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteObservabilityDashboardInstance(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	if *c.observabilityStackInstance.LoggingEnabled {
		// append logging operations
		operations.AppendOperation(util.Operation{
			Name: "logging",
			Create: func() error {
				if err := c.createLoggingInstance(); err != nil {
					return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
				}
				return nil
			},
			Delete: func() error {
				if err := c.deleteLoggingInstance(); err != nil {
					return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
				}
				return nil
			},
		})
	}

	if *c.observabilityStackInstance.MetricsEnabled {
		// append metrics operations
		operations.AppendOperation(util.Operation{
			Name: "metrics",
			Create: func() error {
				if err := c.createMetricsInstance(); err != nil {
					return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
				}
				return nil
			},
			Delete: func() error {
				if err := c.deleteMetricsInstance(); err != nil {
					return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
				}
				return nil
			},
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
				Name: util.StringPtr(ObservabilityDashboardName(*c.observabilityStackInstance.Name)),
			},
			ObservabilityDashboardDefinitionID: c.observabilityStackDefinition.ObservabilityDashboardDefinitionID,
			KubernetesRuntimeInstanceID:        c.observabilityStackInstance.KubernetesRuntimeInstanceID,
		})
	if err != nil {
		return fmt.Errorf("failed to create observability dashboard instance: %w", err)
	}

	// update observability dashboard instance id
	c.observabilityStackInstance.ObservabilityDashboardInstanceID = observabilityDashboardInstance.ID

	return nil
}

// deleteObservabilityDashboardInstance creates an observability dashboard instance
func (c *ObservabilityStackInstanceConfig) deleteObservabilityDashboardInstance() error {
	// delete observability dashboard instance
	_, err := client.DeleteObservabilityDashboardInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackInstance.ObservabilityDashboardInstanceID,
	)
	if err != nil {
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
				Name: util.StringPtr(MetricsName(*c.observabilityStackInstance.Name)),
			},
			MetricsDefinitionID:         c.observabilityStackDefinition.MetricsDefinitionID,
			KubernetesRuntimeInstanceID: c.observabilityStackInstance.KubernetesRuntimeInstanceID,
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
	_, err := client.DeleteMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackInstance.MetricsInstanceID,
	)
	if err != nil {
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
				Name: util.StringPtr(LoggingName(*c.observabilityStackInstance.Name)),
			},
			LoggingDefinitionID:         c.observabilityStackDefinition.LoggingDefinitionID,
			KubernetesRuntimeInstanceID: c.observabilityStackInstance.KubernetesRuntimeInstanceID,
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
	_, err := client.DeleteLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.observabilityStackInstance.LoggingInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete logging instance: %w", err)
	}
	return nil
}

// getMergedObservabilityStackInstanceValues returns the merged values for a observability stack instance
func getMergedObservabilityStackInstanceValues(osi *v0.ObservabilityStackInstance, osd *v0.ObservabilityStackDefinition) (string, string, string, string, error) {

	grafanaHelmValuesDocument := ""
	kubePrometheusStackHelmValuesDocument := ""
	lokiHelmValuesDocument := ""
	promtailHelmValuesDocument := ""

	// merge grafana values
	grafanaHelmValuesDocument, err := MergeHelmValues(
		util.StringPtrToString(osi.GrafanaHelmValuesDocument),
		util.StringPtrToString(osd.GrafanaHelmValuesDocument),
	)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to merge grafana helm values: %w", err)
	}

	// merge kube-prometheus-stack values
	kubePrometheusStackHelmValuesDocument, err = MergeHelmValues(
		util.StringPtrToString(osi.KubePrometheusStackHelmValuesDocument),
		util.StringPtrToString(osd.KubePrometheusStackHelmValuesDocument),
	)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to merge kube-prometheus-stack helm values: %w", err)
	}

	// merge loki values
	lokiHelmValuesDocument, err = MergeHelmValues(
		util.StringPtrToString(osi.LokiHelmValuesDocument),
		util.StringPtrToString(osd.LokiHelmValuesDocument),
	)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// merge promtail values
	promtailHelmValuesDocument, err = MergeHelmValues(
		util.StringPtrToString(osi.PromtailHelmValuesDocument),
		util.StringPtrToString(osd.PromtailHelmValuesDocument),
	)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to merge promtail helm values: %w", err)
	}

	return grafanaHelmValuesDocument, kubePrometheusStackHelmValuesDocument, lokiHelmValuesDocument, promtailHelmValuesDocument, nil
}
