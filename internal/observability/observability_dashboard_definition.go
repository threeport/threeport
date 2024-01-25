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

// observabilityDashboardDefinitionCreated reconciles state for a new kubernetes
// observability dashboard definition.
func observabilityDashboardDefinitionCreated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	// create observability dashboard helm workload definition
	grafanaHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(KubePrometheusStackChartName(*observabilityDashboardDefinition.Name)),
			},
			HelmRepo:           util.StringPtr(GrafanaHelmRepo),
			HelmChart:          util.StringPtr("grafana"),
			HelmChartVersion:   observabilityDashboardDefinition.GrafanaHelmChartVersion,
			HelmValuesDocument: observabilityDashboardDefinition.GrafanaHelmValuesDocument,
		})
	if err != nil {
		return 0, fmt.Errorf("failed to create kube-prometheus-stack helm workload definition: %w", err)
	}

	// update metrics definition with helm workload definition id
	observabilityDashboardDefinition.GrafanaHelmWorkloadDefinitionID = grafanaHelmWorkloadDefinition.ID

	// update metrics instance reconciled field
	observabilityDashboardDefinition.Reconciled = util.BoolPtr(true)
	if _, err := client.UpdateObservabilityDashboardDefinition(
		r.APIClient,
		r.APIServer,
		observabilityDashboardDefinition,
	); err != nil {
		return 0, fmt.Errorf("failed to update metrics definition reconciled field: %w", err)
	}

	return 0, nil
}

// observabilityDashboardDefinitiondUpdated reconciles state for an updated kubernetes
// observability dashboard definition.
func observabilityDashboardDefinitionUpdated(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityDashboardDefinitiondDeleted reconciles state for a deleted kubernetes
// observability dashboard definition.
func observabilityDashboardDefinitionDeleted(
	r *controller.Reconciler,
	observabilityDashboardDefinition *v0.ObservabilityDashboardDefinition,
	log *logr.Logger,
) (int64, error) {
	// delete kube-prometheus-stack helm workload definition
	if _, err := client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*observabilityDashboardDefinition.GrafanaHelmWorkloadDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload definition: %w", err)
	}

	return 0, nil
}
