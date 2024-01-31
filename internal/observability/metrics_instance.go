package observability

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// metricsInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func metricsInstanceCreated(
	r *controller.Reconciler,
	metricsInstance *v0.MetricsInstance,
	log *logr.Logger,
) (int64, error) {

	var err error

	// get grafana helm workload definition
	grafanaHelmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByName(
		r.APIClient,
		r.APIServer,
		GrafanaChartName(*metricsInstance.Name),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get grafana helm workload definition: %w", err)
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

	// ensure grafana helm workload instance is deployed
	_, err = client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			KubernetesRuntimeInstanceID: metricsInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    grafanaHelmWorkloadDefinition.ID,
			HelmValuesDocument:          &grafanaHelmWorkloadInstanceValues,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return 0, fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	}

	// get kube-prometheus-stack helm workload definition
	kubePrometheusStackHelmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByName(
		r.APIClient,
		r.APIServer,
		KubePrometheusStackChartName(*metricsInstance.Name),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get kube-prometheus-stack helm workload definition: %w", err)
	}

	// merge grafana helm values if they are provided
	kubePrometheusStackHelmWorkloadInstanceValues := grafanaValues
	if metricsInstance.GrafanaHelmValues != nil {
		kubePrometheusStackHelmWorkloadInstanceValues, err = MergeHelmValues(
			grafanaValues,
			*metricsInstance.GrafanaHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
		}
	}

	// create kube-prometheus-stack helm workload instance
	_, err = client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			KubernetesRuntimeInstanceID: metricsInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    kubePrometheusStackHelmWorkloadDefinition.ID,
			HelmValuesDocument:          &kubePrometheusStackHelmWorkloadInstanceValues,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
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
	// delete grafana helm workload instance
	_, err := client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*metricsInstance.GrafanaHelmWorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
	}

	// delete kube-prometheus-stack helm workload instance
	_, err = client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*metricsInstance.KubePrometheusStackHelmWorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload instance: %w", err)
	}

	return 0, nil
}
