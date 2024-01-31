package observability

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// loggingInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func loggingInstanceCreated(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	var err error

	// get grafana helm workload definition
	grafanaHelmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByName(
		r.APIClient,
		r.APIServer,
		GrafanaChartName(*loggingInstance.Name),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get grafana helm workload definition: %w", err)
	}

	// merge grafana helm values if they are provided
	grafanaHelmWorkloadInstanceValues := grafanaValues
	if loggingInstance.GrafanaHelmValues != nil {
		grafanaHelmWorkloadInstanceValues, err = MergeHelmValues(
			grafanaValues,
			*loggingInstance.GrafanaHelmValues,
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
			KubernetesRuntimeInstanceID: loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    grafanaHelmWorkloadDefinition.ID,
			HelmValuesDocument:          &grafanaHelmWorkloadInstanceValues,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return 0, fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	}

	// get kube-prometheus-stack helm workload definition
	lokiHelmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByName(
		r.APIClient,
		r.APIServer,
		LokiHelmChartName(*loggingInstance.Name),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get kube-prometheus-stack helm workload definition: %w", err)
	}

	// merge grafana helm values if they are provided
	lokiWorkloadInstanceValues := lokiValues
	if loggingInstance.GrafanaHelmValues != nil {
		lokiWorkloadInstanceValues, err = MergeHelmValues(
			lokiValues,
			*loggingInstance.LokiHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
		}
	}

	// create loki helm workload instance
	_, err = client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			KubernetesRuntimeInstanceID: loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    lokiHelmWorkloadDefinition.ID,
			HelmValuesDocument:          &lokiWorkloadInstanceValues,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
	}

	// merge grafana helm values if they are provided
	// create loki helm workload instance
	_, err = client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			KubernetesRuntimeInstanceID: loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    lokiHelmWorkloadDefinition.ID,
			HelmValuesDocument:          loggingInstance.PromtailHelmValues,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
	}

	return 0, nil
}

// loggingInstanceUpdated reconciles state for a new kubernetes
// runtime instance.
func loggingInstanceUpdated(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// loggingInstanceDeleted reconciles state for a new kubernetes
// runtime instance.
func loggingInstanceDeleted(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	// delete grafana helm workload instance
	_, err := client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*loggingInstance.GrafanaHelmWorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
	}

	// delete loki helm workload instance
	_, err = client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*loggingInstance.LokiHelmWorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete loki helm workload instance: %w", err)
	}

	// delete promtail helm workload instance
	_, err = client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*loggingInstance.PromtailHelmWorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
	}

	return 0, nil
}
