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

// observabilityDashboardInstanceCreated reconciles state for a new kubernetes
// observability dashboard definition.
func observabilityDashboardInstanceCreated(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	// get observability dashboard definition
	observabilityDashboardDefinition, err := client.GetObservabilityDashboardDefinitionByID(
		r.APIClient,
		r.APIServer,
		*observabilityDashboardInstance.ObservabilityDashboardDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get metrics definition: %w", err)
	}
	if !*observabilityDashboardDefinition.Reconciled {
		return 0, fmt.Errorf("metrics definition is not reconciled")
	}

	// merge grafana helm values
	grafanaHelmValuesDocument, err := MergeHelmValues(
		util.StringPtrToString(observabilityDashboardDefinition.GrafanaHelmValuesDocument),
		util.StringPtrToString(observabilityDashboardInstance.GrafanaHelmValues),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
	}

	// create grafana helm workload instance
	grafanaHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.StringPtr(KubePrometheusStackChartName(*observabilityDashboardInstance.Name)),
			},
			KubernetesRuntimeInstanceID: observabilityDashboardInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    observabilityDashboardDefinition.GrafanaHelmWorkloadDefinitionID,
			HelmValuesDocument:          &grafanaHelmValuesDocument,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	}

	// update grafana helm workload instance
	observabilityDashboardInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID

	// update metrics instance reconciled field
	observabilityDashboardInstance.Reconciled = util.BoolPtr(true)
	if _, err = client.UpdateObservabilityDashboardInstance(
		r.APIClient,
		r.APIServer,
		observabilityDashboardInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics instance reconciled field: %w", err)
	}

	return 0, nil
}

// observabilityDashboardInstanceUpdated reconciles state for an updated kubernetes
// observability dashboard definition.
func observabilityDashboardInstanceUpdated(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardInstanceDeleted reconciles state for a deleted kubernetes
// observability dashboard definition.
func observabilityDashboardInstanceDeleted(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	// delete kube-prometheus-stack helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*observabilityDashboardInstance.GrafanaHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload instance: %w", err)
	}

	return 0, nil
}
