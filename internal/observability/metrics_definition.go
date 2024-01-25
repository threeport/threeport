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

const kubePrometheusStackValues = `
grafana:
  enabled: false

  ## ForceDeployDatasources Create datasource configmap even if grafana deployment has been disabled
  ##
  forceDeployDatasources: true

  ## ForceDeployDashboard Create dashboard configmap even if grafana deployment has been disabled
  ##
  forceDeployDashboards: true
`

// metricsDefinitionCreated reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionCreated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {

	// create kube-prometheus-stack helm workload definition
	kubePrometheusStackHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(KubePrometheusStackChartName(*metricsDefinition.Name)),
			},
			Repo:               util.StringPtr(PrometheusCommunityHelmRepo),
			Chart:              util.StringPtr("kube-prometheus-stack"),
			HelmChartVersion:   metricsDefinition.KubePrometheusStackHelmChartVersion,
			HelmValuesDocument: metricsDefinition.KubePrometheusStackHelmValuesDocument,
		})
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload definition: %w", err)
	}

	// update metrics definition with helm workload definition id
	metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID = kubePrometheusStackHelmWorkloadDefinition.ID

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
	// delete kube-prometheus-stack helm workload definition
	if _, err := client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload definition: %w", err)
	}

	return 0, nil
}
