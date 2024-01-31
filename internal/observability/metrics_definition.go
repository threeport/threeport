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

// MetricsDefinitionConfig is the configuration for a metrics definition
// reconciler.
type MetricsDefinitionConfig struct {
	r                 *controller.Reconciler
	metricsDefinition *v0.MetricsDefinition
	log               *logr.Logger
}

// metricsDefinitionCreated reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionCreated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	c := &MetricsDefinitionConfig{
		r:                 r,
		metricsDefinition: metricsDefinition,
		log:               log,
	}

	// get metrics definition operations
	operations := getMetricsDefinitionOperations(c)

	// execute create metrics definition operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute create metrics definition operations: %w", err)
	}

	// update metrics instance reconciled field
	metricsDefinition.Reconciled = util.BoolPtr(true)
	if _, err := client.UpdateMetricsDefinition(
		r.APIClient,
		r.APIServer,
		metricsDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics definition reconciled field: %w", err)
	}

	return 0, nil
}

// metricsDefinitionUpdated reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionUpdated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// metricsDefinitionDeleted reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionDeleted(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	c := &MetricsDefinitionConfig{
		r:                 r,
		metricsDefinition: metricsDefinition,
		log:               log,
	}

	// get metrics definition operations
	operations := getMetricsDefinitionOperations(c)

	// execute delete metrics definition operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute delete metrics definition operations: %w", err)
	}

	return 0, nil
}

func getMetricsDefinitionOperations(c *MetricsDefinitionConfig) *util.Operations {
	operations := util.Operations{}

	// append grafana operations
	operations.AppendOperation(util.Operation{
		Name: "grafana",
		Create: func() error {
			if err := c.createGrafanaHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteGrafanaHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append kube-prometheus-stack operations
	operations.AppendOperation(util.Operation{
		Name: "kube-prometheus-stack",
		Create: func() error {
			if err := c.createKubePrometheusStackHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteKubePrometheusStackHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}

// createGrafanaHelmWorkloadDefinition creates a grafana helm workload
// definition.
func (c *MetricsDefinitionConfig) createGrafanaHelmWorkloadDefinition() error {
	// ensure grafana helm workload definition exists
	grafanaHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(GrafanaChartName(*c.metricsDefinition.Name)),
			},
			Repo:           util.StringPtr(GrafanaHelmRepo),
			Chart:          util.StringPtr("grafana"),
			HelmChartVersion:   c.metricsDefinition.GrafanaHelmChartVersion,
			HelmValuesDocument: c.metricsDefinition.GrafanaHelmValuesDocument,
		})
	if err != nil && !errors.Is(err, client.ErrConflict) {
		// only return error if it isn't a conflict, since we
		// expect both MetricsInstance and LoggingInstance to depend
		// on the same HelmWorkloadDefinition for Grafana
		return fmt.Errorf("failed to create grafana helm workload definition: %w", err)
	} else if err != nil && errors.Is(err, client.ErrConflict) {
		grafanaHelmWorkloadDefinition, err = client.GetHelmWorkloadDefinitionByName(
			c.r.APIClient,
			c.r.APIServer,
			GrafanaChartName(*c.metricsDefinition.Name),
		)
	}

	// update metrics definition with helm workload definition id
	c.metricsDefinition.GrafanaHelmWorkloadDefinitionID = grafanaHelmWorkloadDefinition.ID

	// wait for grafana helm workload definition to be reconciled
	if err := helmworkload.WaitForHelmWorkloadDefinitionReconciled(
		c.r,
		*grafanaHelmWorkloadDefinition.ID,
	); err != nil {
		return fmt.Errorf("failed to wait for grafana helm workload definition to be reconciled: %w", err)
	}

	return nil
}

// deleteGrafanaHelmWorkloadDefinition deletes a grafana helm workload
// definition.
func (c *MetricsDefinitionConfig) deleteGrafanaHelmWorkloadDefinition() error {
	// check if logging is deployed
	loggingDefinition, err := client.GetLoggingDefinitionByName(
		c.r.APIClient,
		c.r.APIServer,
		*c.metricsDefinition.Name,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to get logging definition: %w", err)
	} else if err != nil && errors.Is(err, client.ErrObjectNotFound) ||
		(loggingDefinition != nil && loggingDefinition.DeletionScheduled != nil) {

		// delete grafana helm workload definition
		_, err := client.DeleteHelmWorkloadDefinition(
			c.r.APIClient,
			c.r.APIServer,
			*c.metricsDefinition.GrafanaHelmWorkloadDefinitionID,
		)
		if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
			return fmt.Errorf("failed to delete grafana helm workload definition: %w", err)
		}

		// wait for grafana helm workload definition to be deleted
		if err := helmworkload.WaitForHelmWorkloadDefinitionDeleted(
			c.r,
			*c.metricsDefinition.GrafanaHelmWorkloadDefinitionID,
		); err != nil {
			return fmt.Errorf("failed to wait for grafana helm workload definition to be deleted: %w", err)
		}

	}
	return nil
}

// createKubePrometheusStackHelmWorkloadDefinition creates a kube-prometheus-stack helm
// workload definition.
func (c *MetricsDefinitionConfig) createKubePrometheusStackHelmWorkloadDefinition() error {
	// create kube-prometheus-stack helm workload definition
	kubePrometheusStackHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(KubePrometheusStackChartName(*c.metricsDefinition.Name)),
			},
			Repo:           util.StringPtr(PrometheusCommunityHelmRepo),
			Chart:          util.StringPtr("kube-prometheus-stack"),
			HelmChartVersion:   c.metricsDefinition.KubePrometheusStackHelmChartVersion,
			HelmValuesDocument: c.metricsDefinition.KubePrometheusStackHelmValuesDocument,
		})
	if err != nil {
		return fmt.Errorf("failed to create kube-prometheus-stack helm workload definition: %w", err)
	}

	// update metrics definition with helm workload definition id
	c.metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID = kubePrometheusStackHelmWorkloadDefinition.ID

	// wait for kube-prometheus-stack helm workload definition to be reconciled
	if err := helmworkload.WaitForHelmWorkloadDefinitionReconciled(
		c.r,
		*kubePrometheusStackHelmWorkloadDefinition.ID,
	); err != nil {
		return fmt.Errorf("failed to wait for kube-prometheus-stack helm workload definition to be reconciled: %w", err)
	}

	return nil
}

// deleteKubePrometheusStackHelmWorkloadDefinition deletes a kube-prometheus-stack helm
// workload definition.
func (c *MetricsDefinitionConfig) deleteKubePrometheusStackHelmWorkloadDefinition() error {
	// delete kube-prometheus-stack helm workload definition
	if _, err := client.DeleteHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete kube-prometheus-stack helm workload definition: %w", err)
	}

	// wait for kube-prometheus-stack helm workload definition to be deleted
	if err := helmworkload.WaitForHelmWorkloadDefinitionDeleted(
		c.r,
		*c.metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID,
	); err != nil {
		return fmt.Errorf("failed to wait for kube-prometheus-stack helm workload definition to be deleted: %w", err)
	}

	return nil
}
