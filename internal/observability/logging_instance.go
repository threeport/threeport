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

// loggingInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func loggingInstanceCreated(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	log *logr.Logger,
) (int64, error) {
	// get logging definition
	loggingDefinition, err := client.GetLoggingDefinitionByID(
		r.APIClient,
		r.APIServer,
		*loggingInstance.LoggingDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get logging definition: %w", err)
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
	grafanaHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(GrafanaChartName(*loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    loggingDefinition.GrafanaHelmWorkloadDefinitionID,
			HelmValuesDocument:          &grafanaHelmWorkloadInstanceValues,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return 0, fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	} else {
		grafanaHelmWorkloadInstance, err = client.GetHelmWorkloadInstanceByName(
			r.APIClient,
			r.APIServer,
			GrafanaChartName(*loggingInstance.Name),
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get grafana helm workload instance: %w", err)
		}
	}

	// merge loki helm values if they are provided
	lokiHelmWorkloadInstanceValues := lokiValues
	if loggingInstance.GrafanaHelmValues != nil {
		lokiHelmWorkloadInstanceValues, err = MergeHelmValues(
			lokiValues,
			*loggingInstance.LokiHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
		}
	}

	// create loki helm workload instance
	lokiHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(LokiHelmChartName(*loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    loggingDefinition.LokiHelmWorkloadDefinitionID,
			HelmValuesDocument:          &lokiHelmWorkloadInstanceValues,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
	}

	// create promtail helm workload instance
	promtailHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(PromtailHelmChartName(*loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    loggingDefinition.PromtailHelmWorkloadDefinitionID,
			HelmValuesDocument:          loggingInstance.PromtailHelmValues,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload instance: %w", err)
	}

	// update logging instance reconciled field
	loggingInstance.Reconciled = util.BoolPtr(true)
	loggingInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID
	loggingInstance.LokiHelmWorkloadInstanceID = lokiHelmWorkloadInstance.ID
	loggingInstance.PromtailHelmWorkloadInstanceID = promtailHelmWorkloadInstance.ID
	_, err = client.UpdateLoggingInstance(
		r.APIClient,
		r.APIServer,
		loggingInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update logging instance reconciled field: %w", err)
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
