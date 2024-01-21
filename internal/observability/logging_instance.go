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
	if !*loggingDefinition.Reconciled {
		return 0, fmt.Errorf("logging definition is not reconciled")
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

	// generate shared namespace name for loki and promtail
	loggingNamespace := fmt.Sprintf("%s-logging-%s", *loggingInstance.Name, util.RandomAlphaString(10))

	// merge loki helm values if they are provided
	lokiHelmWorkloadInstanceValues := lokiValues
	if loggingInstance.LokiHelmValues != nil {
		lokiHelmWorkloadInstanceValues, err = MergeHelmValues(
			lokiValues,
			*loggingInstance.LokiHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
		}
	}

	// merge loki helm values if they are provided
	promtailHelmWorkloadInstanceValues := promtailValues
	if loggingInstance.PromtailHelmValues != nil {
		promtailHelmWorkloadInstanceValues, err = MergeHelmValues(
			promtailValues,
			*loggingInstance.PromtailHelmValues,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to merge promtail helm values: %w", err)
		}
	}

	// get logging operations
	operations := getLoggingOperations(
		r,
		loggingInstance,
		loggingDefinition,
		loggingNamespace,
		grafanaHelmWorkloadInstanceValues,
		lokiHelmWorkloadInstanceValues,
		promtailHelmWorkloadInstanceValues,
	)

	// execute logging operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute logging create operations: %w", err)
	}

	// update logging instance reconciled field
	loggingInstance.Reconciled = util.BoolPtr(true)
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

	// get logging operations
	operations := getLoggingOperations(
		r,
		loggingInstance,
		nil,
		"",
		"",
		"",
		"",
	)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging delete operations: %w", err)
	}

	return 0, nil
}

func getLoggingOperations(
	r *controller.Reconciler,
	loggingInstance *v0.LoggingInstance,
	loggingDefinition *v0.LoggingDefinition,
	loggingNamespace,
	grafanaHelmWorkloadInstanceValues,
	lokiHelmWorkloadInstanceValues,
	promtailHelmWorkloadInstanceValues string,
) *util.Operations {

	operations := util.Operations{}

	// append grafana operations
	operations.AppendOperation(util.Operation{
		Name: "grafana",
		Create: func() error {
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
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			} else if err != nil && errors.Is(err, client.ErrConflict) {
				grafanaHelmWorkloadInstance, err = client.GetHelmWorkloadInstanceByName(
					r.APIClient,
					r.APIServer,
					GrafanaChartName(*loggingInstance.Name),
				)
				if err != nil {
					return fmt.Errorf("failed to get grafana helm workload instance: %w", err)
				}
				metricsInstance, err := client.GetMetricsInstanceByName(
					r.APIClient,
					r.APIServer,
					*loggingInstance.Name,
				)
				if err != nil {
					return fmt.Errorf("failed to get metrics instance: %w", err)
				}
				if metricsInstance.GrafanaHelmWorkloadInstanceID != nil &&
					*metricsInstance.GrafanaHelmWorkloadInstanceID != *grafanaHelmWorkloadInstance.ID {
					return fmt.Errorf("grafana helm workload instance already exists")
				}
			}
			loggingInstance.GrafanaHelmWorkloadInstanceID = grafanaHelmWorkloadInstance.ID
			return nil
		},
		Delete: func() error {
			// check if metrics is deployed,
			// if it's not then we can clean up grafana chart
			metricsInstance, err := client.GetMetricsInstanceByName(
				r.APIClient,
				r.APIServer,
				*loggingInstance.Name,
			)
			if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
				return fmt.Errorf("failed to get metrics instance: %w", err)
			} else if err != nil && errors.Is(err, client.ErrObjectNotFound) ||
				(metricsInstance != nil &&
					metricsInstance.DeletionScheduled != nil) {
				// delete grafana helm workload instance
				_, err = client.DeleteHelmWorkloadInstance(
					r.APIClient,
					r.APIServer,
					*loggingInstance.GrafanaHelmWorkloadInstanceID,
				)
				if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
					return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
				}
			}
			return nil
		},
	})

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name: "loki",
		Create: func() error {
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
					HelmReleaseNamespace:        &loggingNamespace,
				},
			)
			if err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			loggingInstance.LokiHelmWorkloadInstanceID = lokiHelmWorkloadInstance.ID
			return nil
		},
		Delete: func() error {
			// delete loki helm workload instance
			_, err := client.DeleteHelmWorkloadInstance(
				r.APIClient,
				r.APIServer,
				*loggingInstance.LokiHelmWorkloadInstanceID,
			)
			if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name: "promtail",
		Create: func() error {
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
					HelmValuesDocument:          &promtailHelmWorkloadInstanceValues,
					HelmReleaseNamespace:        &loggingNamespace,
				},
			)
			if err != nil {
				return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
			}
			loggingInstance.PromtailHelmWorkloadInstanceID = promtailHelmWorkloadInstance.ID
			return nil
		},
		Delete: func() error {
			// delete promtail helm workload instance
			_, err := client.DeleteHelmWorkloadInstance(
				r.APIClient,
				r.APIServer,
				*loggingInstance.PromtailHelmWorkloadInstanceID,
			)
			if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
				return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}
