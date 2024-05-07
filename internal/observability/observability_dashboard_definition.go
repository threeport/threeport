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

// grafanaValues contains the default helm values for
// a grafana helm chart.
const grafanaValues = `
persistence:
  enabled: true

adminPassword: password

sidecar:
  dashboards:
    enabled: true
    label: grafana_dashboard
    labelValue: "1"
    # Allow discovery in all namespaces for dashboards
    searchNamespace: ALL

  datasources:
    enabled: true
    label: grafana_datasource
    labelValue: "1"
    # Allow discovery in all namespaces for dashboards
    searchNamespace: ALL
`

// observabilityDashboardDefinitionCreated reconciles state
// for a created observability dashboard definition.
func observabilityDashboardDefinitionCreated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	// merge grafana helm values
	grafanaHelmValuesDocument, err := helmworkload.MergeHelmValuesString(
		grafanaValues,
		util.StringPtrToString(observabilityDashboardDefinition.GrafanaHelmValuesDocument),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge grafana helm values: %w", err)
	}

	// create observability dashboard helm workload definition
	grafanaHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.Ptr(GrafanaChartName(*observabilityDashboardDefinition.Name)),
			},
			Repo:           util.Ptr(GrafanaHelmRepo),
			Chart:          util.Ptr("grafana"),
			ChartVersion:   observabilityDashboardDefinition.GrafanaHelmChartVersion,
			ValuesDocument: &grafanaHelmValuesDocument,
		})
	if err != nil {
		return 0, fmt.Errorf("failed to create grafana helm workload definition: %w", err)
	}

	// update observability dashboard definition with helm workload definition id
	observabilityDashboardDefinition.GrafanaHelmWorkloadDefinitionID = grafanaHelmWorkloadDefinition.ID

	// update observability dashboard definition
	observabilityDashboardDefinition.Reconciled = util.Ptr(true)
	if _, err := client.UpdateObservabilityDashboardDefinition(
		r.APIClient,
		r.APIServer,
		observabilityDashboardDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update observibility dashboard definition: %w", err)
	}

	return 0, nil
}

// observabilityDashboardDefinitiondUpdated reconciles state for an updated
// observability dashboard definition.
func observabilityDashboardDefinitionUpdated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardDefinitiondDeleted reconciles state for a deleted
// observability dashboard definition.
func observabilityDashboardDefinitionDeleted(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	// delete observability dashboard definition
	if _, err := client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*observabilityDashboardDefinition.GrafanaHelmWorkloadDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to delete grafana helm workload definition: %w", err)
	}

	return 0, nil
}
