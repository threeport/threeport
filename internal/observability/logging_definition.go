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

// lokiValues contains the default values for the loki helm chart.
const lokiValues = `
loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1
  storage:
    type: 'filesystem'
singleBinary:
  replicas: 1
test:
  enabled: false
monitoring:
  selfMonitoring:
    enabled: false
    grafanaAgent:
      installOperator: false
extraObjects:
- kind: ConfigMap
  apiVersion: v1
  metadata:
    name: loki-grafana-datasource
    namespace: "{{ $.Release.Namespace }}"
    labels:
      grafana_datasource: "1"
  data:
    loki-datasource.yaml: |-
      apiVersion: 1
      datasources:
      - name: loki
        access: proxy
        editable: false
        isDefault: false
        jsonData:
            tlsSkipVerify: true
        type: loki
        url: http://loki-headless.{{ $.Release.Namespace }}:3100
`

// promtailValues contains the default values for the promtail helm chart.
const promtailValues = ``

// LoggingDefinitionConfig contains configuration for a logging
// definition reconcile function.
type LoggingDefinitionConfig struct {
	r                                    *controller.Reconciler
	loggingDefinition                    *v0.LoggingDefinition
	log                                  *logr.Logger
	lokiHelmWorkloadDefinitionValues     string
	promtailHelmWorkloadDefinitionValues string
}

// loggingDefinitionCreated reconciles state for a
// new logging definition.
func loggingDefinitionCreated(
	r *controller.Reconciler,
	loggingDefinition *v0.LoggingDefinition,
	log *logr.Logger,
) (int64, error) {
	var err error

	// create logging definition config
	c := &LoggingDefinitionConfig{
		r:                 r,
		loggingDefinition: loggingDefinition,
		log:               log,
	}

	// merge loki helm values
	c.lokiHelmWorkloadDefinitionValues, err = helmworkload.MergeHelmValuesString(
		lokiValues,
		util.StringPtrToString(loggingDefinition.LokiHelmValuesDocument),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// merge promtail helm values
	c.promtailHelmWorkloadDefinitionValues, err = helmworkload.MergeHelmValuesString(
		promtailValues,
		util.StringPtrToString(loggingDefinition.PromtailHelmValuesDocument),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to merge loki helm values: %w", err)
	}

	// get logging operations
	operations := c.getLoggingDefinitionOperations()

	// execute logging definition create operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute logging definition create operations: %w", err)
	}

	// update logging definition
	loggingDefinition.Reconciled = util.BoolPtr(true)
	_, err = client.UpdateLoggingDefinition(
		r.APIClient,
		r.APIServer,
		loggingDefinition,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update logging definition: %w", err)
	}

	return 0, nil
}

// loggingDefinitionUpdated reconciles state for an
// updated logging definition.
func loggingDefinitionUpdated(
	r *controller.Reconciler,
	loggingDefinition *v0.LoggingDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// loggingDefinitionDeleted reconciles state for a
// deleted logging definition.
func loggingDefinitionDeleted(
	r *controller.Reconciler,
	loggingDefinition *v0.LoggingDefinition,
	log *logr.Logger,
) (int64, error) {
	// create logging definition config
	c := &LoggingDefinitionConfig{
		r:                 r,
		loggingDefinition: loggingDefinition,
		log:               log,
	}

	// get logging operations
	operations := c.getLoggingDefinitionOperations()

	// execute logging definition delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging definition delete operations: %w", err)
	}

	return 0, nil
}

// getLoggingDefinitionOperations returns a list of operations for a logging definition.
func (c *LoggingDefinitionConfig) getLoggingDefinitionOperations() *util.Operations {
	operations := util.Operations{}

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name:   "loki",
		Create: func() error { return c.createLokiHelmWorkloadDefinition() },
		Delete: func() error { return c.deleteLokiHelmWorkloadDefinition() },
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name:   "promtail",
		Create: func() error { return c.createPromtailHelmWorkloadDefinition() },
		Delete: func() error { return c.deletePromtailHelmWorkloadDefinition() },
	})

	return &operations
}

// createLokiHelmWorkloadDefinition creates a loki helm workload definition.
func (c *LoggingDefinitionConfig) createLokiHelmWorkloadDefinition() error {
	// create loki helm workload definition
	lokiHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(LokiHelmChartName(*c.loggingDefinition.Name)),
			},
			Repo:           util.StringPtr(GrafanaHelmRepo),
			Chart:          util.StringPtr("loki"),
			ChartVersion:   c.loggingDefinition.LokiHelmChartVersion,
			ValuesDocument: &c.lokiHelmWorkloadDefinitionValues,
		})
	if err != nil {
		return fmt.Errorf("failed to create loki helm workload definition: %w", err)
	}

	// update logging definition with loki helm workload definition id
	c.loggingDefinition.LokiHelmWorkloadDefinitionID = lokiHelmWorkloadDefinition.ID

	return nil
}

// deleteLokiHelmWorkloadDefinition deletes a loki helm workload definition.
func (c *LoggingDefinitionConfig) deleteLokiHelmWorkloadDefinition() error {
	// delete loki helm workload definition
	if _, err := client.DeleteHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingDefinition.LokiHelmWorkloadDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete loki helm workload definition: %w", err)
	}

	return nil
}

// createPromtailHelmWorkloadDefinition creates a promtail helm workload definition.
func (c *LoggingDefinitionConfig) createPromtailHelmWorkloadDefinition() error {
	// create promtail helm workload definition
	promtailHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(PromtailHelmChartName(*c.loggingDefinition.Name)),
			},
			Repo:           util.StringPtr(GrafanaHelmRepo),
			Chart:          util.StringPtr("promtail"),
			ChartVersion:   c.loggingDefinition.PromtailHelmChartVersion,
			ValuesDocument: &c.promtailHelmWorkloadDefinitionValues,
		})
	if err != nil {
		return fmt.Errorf("failed to create promtail helm workload definition: %w", err)
	}

	// update logging definition with promtail helm workload definition id
	c.loggingDefinition.PromtailHelmWorkloadDefinitionID = promtailHelmWorkloadDefinition.ID

	return nil
}

// deletePromtailHelmWorkloadDefinition creates a promtail helm workload definition.
func (c *LoggingDefinitionConfig) deletePromtailHelmWorkloadDefinition() error {
	// delete promtail helm workload definition
	if _, err := client.DeleteHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingDefinition.PromtailHelmWorkloadDefinitionID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete promtail helm workload definition: %w", err)
	}

	return nil
}
