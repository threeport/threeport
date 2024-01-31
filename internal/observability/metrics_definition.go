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

	return nil
}
