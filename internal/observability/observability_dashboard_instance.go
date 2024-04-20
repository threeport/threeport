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

// observabilityDashboardInstanceCreated reconciles state for a
// created observability dashboard instance.
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
		return 0, fmt.Errorf("failed to get observability dashboard definition: %w", err)
	}
	if !*observabilityDashboardDefinition.Reconciled {
		return 0, fmt.Errorf("observability dashboard definition is not reconciled")
	}

	// merge grafana helm values
	grafanaHelmValuesDocument, err := helmworkload.MergeHelmValuesPtrs(
		observabilityDashboardDefinition.GrafanaHelmValuesDocument,
		observabilityDashboardInstance.GrafanaHelmValuesDocument,
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
				Name: util.Ptr(GrafanaChartName(*observabilityDashboardInstance.Name)),
			},
			KubernetesRuntimeInstanceID: observabilityDashboardInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    observabilityDashboardDefinition.GrafanaHelmWorkloadDefinitionID,
			ValuesDocument:              &grafanaHelmValuesDocument,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create grafana helm workload instance: %w", err)
	}

	// update grafana helm workload instance
	observabilityDashboardInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID

	// update observability dashboard instance
	observabilityDashboardInstance.Reconciled = util.Ptr(true)
	if _, err = client.UpdateObservabilityDashboardInstance(
		r.APIClient,
		r.APIServer,
		observabilityDashboardInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update observability dashboard instance: %w", err)
	}

	return 0, nil
}

// observabilityDashboardInstanceUpdated reconciles state for an updated
// observability dashboard instance.
func observabilityDashboardInstanceUpdated(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardInstanceDeleted reconciles state for a deleted
// observability dashboard instance.
func observabilityDashboardInstanceDeleted(
	r *controller.Reconciler,
	observabilityDashboardInstance *v0.ObservabilityDashboardInstance,
	log *logr.Logger,
) (int64, error) {
	// delete grafana helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		r.APIClient,
		r.APIServer,
		*observabilityDashboardInstance.GrafanaHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
	}

	return 0, nil
}
