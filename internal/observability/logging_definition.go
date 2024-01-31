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

// loggingDefinitionCreated reconciles state for a new kubernetes
// runtime instance.
func loggingDefinitionCreated(
	r *controller.Reconciler,
	loggingDefinition *v0.LoggingDefinition,
	log *logr.Logger,
) (int64, error) {
	// ensure grafana helm workload definition exists
	_, err := client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(GrafanaChartName(*loggingDefinition.Name)),
			},
			HelmRepo:  util.StringPtr("https://grafana.github.io/helm-charts"),
			HelmChart: util.StringPtr("grafana"),
		})
	if err != nil && !errors.Is(err, client.ErrConflict) {
		// only return error if it isn't a conflict, since we
		// expect both MetricsInstance and LoggingInstance to depend
		// on the same HelmWorkloadDefinition for Grafana
		return 0, fmt.Errorf("failed to create grafana helm workload definition: %w", err)
	}

	// create loki helm workload definition
	_, err = client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(LokiHelmChartName(*loggingDefinition.Name)),
			},
			HelmRepo:  util.StringPtr("https://grafana.github.io/helm-charts"),
			HelmChart: util.StringPtr("loki"),
		})
	if err != nil {
		return 0, fmt.Errorf("failed to create loki helm workload definition: %w", err)
	}

	// create promtail helm workload definition
	_, err = client.CreateHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(PromtailHelmChartName(*loggingDefinition.Name)),
			},
			HelmRepo:  util.StringPtr("https://grafana.github.io/helm-charts"),
			HelmChart: util.StringPtr("promtail"),
		})
	if err != nil {
		return 0, fmt.Errorf("failed to create promtail helm workload definition: %w", err)
	}

	return 0, nil
}

// loggingDefinitionUpdated reconciles state for a new kubernetes
// runtime instance.
func loggingDefinitionUpdated(
	r *controller.Reconciler,
	loggingDefinition *v0.LoggingDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// loggingDefinitionDeleted reconciles state for a new kubernetes
// runtime instance.
func loggingDefinitionDeleted(
	r *controller.Reconciler,
	loggingDefinition *v0.LoggingDefinition,
	log *logr.Logger,
) (int64, error) {
	// delete grafana helm workload definition
	_, err := client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*loggingDefinition.GrafanaHelmWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete grafana helm workload definition: %w", err)
	}

	// delete loki helm workload definition
	_, err = client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*loggingDefinition.LokiHelmWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete loki helm workload definition: %w", err)
	}

	// delete promtail helm workload definition
	_, err = client.DeleteHelmWorkloadDefinition(
		r.APIClient,
		r.APIServer,
		*loggingDefinition.PromtailHelmWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete promtail helm workload definition: %w", err)
	}

	return 0, nil
}
