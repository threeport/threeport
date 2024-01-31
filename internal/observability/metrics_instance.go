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
	if !*metricsDefinition.Reconciled {
		return 0, fmt.Errorf("metrics definition is not reconciled")
	}

	// merge kube-prometheus-stack helm values if they are provided
	kubePrometheusStackHelmWorkloadInstanceValues := kubePrometheusStackValues
	if metricsInstance.KubePrometheusStackHelmValues != nil {
		kubePrometheusStackHelmWorkloadInstanceValues, err = MergeHelmValues(
			kubePrometheusStackHelmWorkloadInstanceValues,
			*metricsInstance.KubePrometheusStackHelmValues,
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

	// update kube-prometheus-stack helm workload instance
	metricsInstance.KubePrometheusStackHelmWorkloadInstanceID = kubePrometheusStackHelmWorkloadInstance.ID

	// update metrics instance reconciled field
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

	// delete kube-prometheus-stack helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*metricsInstance.KubePrometheusStackHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload instance: %w", err)
	}

	return 0, nil
}
