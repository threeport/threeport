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

	var grafanaHelmWorkloadInstance *v0.HelmWorkloadInstance
	if metricsDefinition.GrafanaHelmWorkloadDefinitionID != nil {
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
		grafanaHelmWorkloadInstance, err = client.CreateHelmWorkloadInstance(
			r.APIClient,
			r.APIServer,
			&v0.HelmWorkloadInstance{
				Instance: v0.Instance{
					Name: util.StringPtr(GrafanaChartName(*metricsInstance.Name)),
				},
				KubernetesRuntimeInstanceID: metricsInstance.KubernetesRuntimeInstanceID,
				HelmWorkloadDefinitionID:    metricsDefinition.GrafanaHelmWorkloadDefinitionID,
				HelmValuesDocument:          &grafanaHelmWorkloadInstanceValues,
			},
		)
		if err != nil {
			return 0, fmt.Errorf("failed to create grafana helm workload instance: %w", err)
		}
	} else {
		grafanaHelmWorkloadInstance, err = client.GetHelmWorkloadInstanceByName(
			r.APIClient,
			r.APIServer,
			GrafanaChartName(*metricsInstance.Name),
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get grafana helm workload instance: %w", err)
		}
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
	kubePrometheusStackHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(KubePrometheusStackChartName(*metricsInstance.Name)),
			},
			KubernetesRuntimeInstanceID: metricsInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID,
			HelmValuesDocument:          &kubePrometheusStackHelmWorkloadInstanceValues,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
	}

	// update metrics instance reconciled field
	metricsInstance.Reconciled = util.BoolPtr(true)
	metricsInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID
	metricsInstance.KubePrometheusStackHelmWorkloadInstanceID = kubePrometheusStackHelmWorkloadInstance.ID
	_, err = client.UpdateMetricsInstance(
		r.APIClient,
		r.APIServer,
		metricsInstance,
	)
	if err != nil {
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
	// check if logging is deployed,
	// if it's not then we can clean up grafana chart
	_, err := client.GetHelmWorkloadInstanceByName(
		r.APIClient,
		r.APIServer,
		LokiHelmChartName(*metricsInstance.Name),
	)
	if err != nil && errors.Is(err, client.ErrorObjectNotFound) {
		// delete grafana helm workload instance
		_, err = client.DeleteHelmWorkloadInstance(
			r.APIClient,
			r.APIServer,
			*metricsInstance.GrafanaHelmWorkloadInstanceID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
		}
		return 0, nil
	}

	// delete kube-prometheus-stack helm workload instance
	_, err = client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*metricsInstance.KubePrometheusStackHelmWorkloadInstanceID,
	)
	if err != nil && !errors.Is(err, client.ErrorObjectNotFound) {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload instance: %w", err)
	}

	return 0, nil
}
