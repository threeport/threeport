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

// MetricsInstanceConfig is the configuration for a metrics instance
// reconciler.
type MetricsInstanceConfig struct {
	r                                             *controller.Reconciler
	metricsInstance                               *v0.MetricsInstance
	metricsDefinition                             *v0.MetricsDefinition
	log                                           *logr.Logger
	grafanaHelmWorkloadInstanceValues             string
	kubePrometheusStackHelmWorkloadInstanceValues string
}

// metricsInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceCreated(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {
	// get metrics definition
	metricsDefinition, err := client.GetMetricsDefinitionByID(
		r.APIClient,
		r.APIServer,
		*metricsInstance.MetricsDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get metrics definition: %w", err)
	}
	if !*metricsDefinition.Reconciled {
		return 0, fmt.Errorf("metrics definition is not reconciled")
	}

	// merge grafana helm values if they are provided
	grafanaHelmWorkloadInstanceValues := grafanaValues
	if metricsInstance.GrafanaHelmValues != nil {
		grafanaHelmWorkloadInstanceValues, err = MergeHelmValues(
			grafanaValues,
			*metricsInstance.GrafanaHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
		}
	}

	// merge kube-prometheus-stack helm values if they are provided
	kubePrometheusStackHelmWorkloadInstanceValues := grafanaValues
	if metricsInstance.KubePrometheusStackHelmValues != nil {
		kubePrometheusStackHelmWorkloadInstanceValues, err = MergeHelmValues(
			kubePrometheusStackHelmWorkloadInstanceValues,
			*metricsInstance.KubePrometheusStackHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
		}
	}

	// configure metrics instance config
	c := &MetricsInstanceConfig{
		r:                                 r,
		metricsInstance:                   metricsInstance,
		metricsDefinition:                 metricsDefinition,
		log:                               log,
		grafanaHelmWorkloadInstanceValues: grafanaHelmWorkloadInstanceValues,
		kubePrometheusStackHelmWorkloadInstanceValues: kubePrometheusStackHelmWorkloadInstanceValues,
	}

	// get metrics instance operations
	operations := getMetricsInstanceOperations(c)

	// execute create metrics instance operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute create metrics instance operations: %w", err)
	}

	// update metrics instance reconciled field
	metricsInstance.Reconciled = util.BoolPtr(true)
	if _, err = client.UpdateMetricsInstance(
		r.APIClient,
		r.APIServer,
		metricsInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics instance reconciled field: %w", err)
	}

	return 0, nil
}

// metricsInstanceUpdated reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceUpdated(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// metricsInstanceDeleted reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceDeleted(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {

	// create logging instance config
	c := &MetricsInstanceConfig{
		r:                 r,
		metricsInstance:   metricsInstance,
		metricsDefinition: nil,
		log:               log,
	}

	// get logging operations
	operations := getMetricsInstanceOperations(c)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging delete operations: %w", err)
	}

	return 0, nil
}

// createGrafanaHelmWorkloadInstance creates a grafana helm workload
// instance.
func (c *MetricsInstanceConfig) createGrafanaHelmWorkloadInstance() error {
	// ensure grafana helm workload instance is deployed
	grafanaHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(GrafanaChartName(*c.metricsInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.metricsInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.metricsDefinition.GrafanaHelmWorkloadDefinitionID,
			HelmValuesDocument:          &c.grafanaHelmWorkloadInstanceValues,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	} else {
		grafanaHelmWorkloadInstance, err = client.GetHelmWorkloadInstanceByName(
			c.r.APIClient,
			c.r.APIServer,
			GrafanaChartName(*c.metricsInstance.Name),
		)
		if err != nil {
			return fmt.Errorf("failed to get grafana helm workload instance: %w", err)
		}
		loggingInstance, err := client.GetLoggingInstanceByName(
			c.r.APIClient,
			c.r.APIServer,
			*c.metricsInstance.Name,
		)
		if err != nil {
			return fmt.Errorf("failed to get metrics instance: %w", err)
		}
		if loggingInstance.GrafanaHelmWorkloadInstanceID != nil &&
			*loggingInstance.GrafanaHelmWorkloadInstanceID != *grafanaHelmWorkloadInstance.ID {
			return fmt.Errorf("grafana helm workload instance already exists")
		}
	}

	// update grafana helm workload instance
	c.metricsInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID

	// wait for grafana helm workload instance to be reconciled
	if err := helmworkload.WaitForHelmWorkloadInstanceReconciled(
		c.r,
		*grafanaHelmWorkloadInstance.ID,
	); err != nil {
		return fmt.Errorf("failed to wait for grafana helm workload instance to be reconciled: %w", err)
	}

	return nil
}

// deleteGrafanaHelmWorkloadInstance deletes a grafana helm workload
// instance.
func (c *MetricsInstanceConfig) deleteGrafanaHelmWorkloadInstance() error {
	// check if logging is deployed,
	// if it's not then we can clean up grafana chart
	loggingInstance, err := client.GetLoggingInstanceByName(
		c.r.APIClient,
		c.r.APIServer,
		*c.metricsInstance.Name,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to get logging instance: %w", err)
	} else if err != nil && errors.Is(err, client.ErrObjectNotFound) ||
		(loggingInstance != nil && loggingInstance.DeletionScheduled != nil) {
		// delete grafana helm workload instance
		if _, err := client.DeleteHelmWorkloadInstance(
			c.r.APIClient,
			c.r.APIServer,
			*c.metricsInstance.GrafanaHelmWorkloadInstanceID,
		); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
			return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
		}

		// wait for grafana helm workload instance to be deleted
		if err := helmworkload.WaitForHelmWorkloadInstanceDeleted(
			c.r,
			*c.metricsInstance.GrafanaHelmWorkloadInstanceID,
		); err != nil {
			return fmt.Errorf("failed to wait for grafana helm workload instance to be deleted: %w", err)
		}
	}

	return nil
}

// createKubePrometheusStackHelmWorkloadInstance creates a kube-prometheus-stack helm
// workload instance.
func (c *MetricsInstanceConfig) createKubePrometheusStackHelmWorkloadInstance() error {
	// create kube-prometheus-stack helm workload instance
	kubePrometheusStackHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(KubePrometheusStackChartName(*c.metricsInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.metricsInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID,
			HelmValuesDocument:          &c.kubePrometheusStackHelmWorkloadInstanceValues,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
	}

	// update kube-prometheus-stack helm workload instance
	c.metricsInstance.KubePrometheusStackHelmWorkloadInstanceID = kubePrometheusStackHelmWorkloadInstance.ID

	// wait for kube-prometheus-stack helm workload instance to be reconciled
	if err := helmworkload.WaitForHelmWorkloadInstanceReconciled(
		c.r,
		*kubePrometheusStackHelmWorkloadInstance.ID,
	); err != nil {
		return fmt.Errorf("failed to wait for kube-prometheus-stack helm workload instance to be reconciled: %w", err)
	}

	return nil
}

// deleteKubePrometheusStackHelmWorkloadInstance deletes a kube-prometheus-stack helm
// workload instance.
func (c *MetricsInstanceConfig) deleteKubePrometheusStackHelmWorkloadInstance() error {
	// delete kube-prometheus-stack helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.metricsInstance.KubePrometheusStackHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete kube-prometheus-stack helm workload instance: %w", err)
	}

	// wait for kube-prometheus-stack helm workload instance to be deleted
	if err := helmworkload.WaitForHelmWorkloadInstanceDeleted(
		c.r,
		*c.metricsInstance.KubePrometheusStackHelmWorkloadInstanceID,
	); err != nil {
		return fmt.Errorf("failed to wait for kube-prometheus-stack helm workload instance to be deleted: %w", err)
	}

	return nil
}

// getMetricsInstanceOperations returns a list of operations for a
// metrics instance.
func getMetricsInstanceOperations(c *MetricsInstanceConfig) *util.Operations {
	operations := util.Operations{}

	// append grafana operations
	operations.AppendOperation(util.Operation{
		Name: "grafana",
		Create: func() error {
			if err := c.createGrafanaHelmWorkloadInstance(); err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteGrafanaHelmWorkloadInstance(); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append kube-prometheus-stack operations
	operations.AppendOperation(util.Operation{
		Name: "kube-prometheus-stack",
		Create: func() error {
			if err := c.createKubePrometheusStackHelmWorkloadInstance(); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteKubePrometheusStackHelmWorkloadInstance(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}
