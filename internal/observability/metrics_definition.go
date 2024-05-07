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

// kubePrometheusStackValues contains the default values for the
// kube-prometheus-stack helm chart.
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

// metricsDefinitionCreated reconciles state for a
// created metrics definition.
func metricsDefinitionCreated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	// merge kube-prometheus-stack helm values if they are provided
	kubePrometheusStackHelmWorkloadDefinitionValues, err := helmworkload.MergeHelmValuesString(
		kubePrometheusStackValues,
		util.StringPtrToString(metricsDefinition.KubePrometheusStackHelmValuesDocument),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
	}

	// create kube-prometheus-stack helm workload definition
	kubePrometheusStackHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.Ptr(KubePrometheusStackChartName(*metricsDefinition.Name)),
			},
			Repo:           util.Ptr(PrometheusCommunityHelmRepo),
			Chart:          util.Ptr("kube-prometheus-stack"),
			ChartVersion:   metricsDefinition.KubePrometheusStackHelmChartVersion,
			ValuesDocument: &kubePrometheusStackHelmWorkloadDefinitionValues,
		})
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload definition: %w", err)
	}

	// update metrics definition with helm workload definition id
	metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID = kubePrometheusStackHelmWorkloadDefinition.ID

	// update metrics definition
	metricsDefinition.Reconciled = util.Ptr(true)
	if _, err := client.UpdateMetricsDefinition(
		r.APIClient,
		r.APIServer,
		metricsDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics definition: %w", err)
	}

	return 0, nil
}

// metricsDefinitionUpdated reconciles state for an
// uptaded metrics definition.
func metricsDefinitionUpdated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// metricsDefinitionDeleted reconciles state for a
// deleted metrics definition
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
