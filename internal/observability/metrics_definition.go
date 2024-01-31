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

// metricsDefinitionCreated reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionCreated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {

	// ensure grafana helm workload definition exists
	_, err := client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(GrafanaChartName(*metricsDefinition.Name)),
			},
			HelmRepo:  util.StringPtr("https://grafana.github.io/helm-charts"),
			HelmChart: util.StringPtr("grafana"),
		})
	if err != nil && !errors.Is(err, client.ErrConflict) {
		// only return error if it isn't a conflict, since we
		// expect both MetricsInstance and LoggingInstance to depend
		// on the same HelmWorkloadDefinition for Grafana
		return 0, nil
	}

	// create kube-prometheus-stack helm workload definition
	_, err = client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(KubePrometheusStackChartName(*metricsDefinition.Name)),
			},
			HelmRepo:  util.StringPtr("https://prometheus-community.github.io/helm-charts"),
			HelmChart: util.StringPtr("kube-prometheus-stack"),
		})
	if err != nil {
		return 0, nil
	}
	return 0, nil
}

// metricsDefinitionUpdated reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionUpdated(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// metricsDefinitionDeleted reconciles state for a new kubernetes
// runtime instance.
func metricsDefinitionDeleted(
	r *controller.Reconciler,
	metricsDefinition *v0.MetricsDefinition,
	log *logr.Logger,
) (int64, error) {
	// delete grafana helm workload definition
	_, err := client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*metricsDefinition.GrafanaHelmWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete grafana helm workload definition: %w", err)
	}

	// delete kube-prometheus-stack helm workload definition
	_, err = client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*metricsDefinition.KubePrometheusStackHelmWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete kube-prometheus-stack helm workload definition: %w", err)
	}

	return 0, nil
}
