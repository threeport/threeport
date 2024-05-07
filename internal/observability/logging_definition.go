package observability

import (
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

	// execute logging definition create operations
	if err := c.getLoggingDefinitionOperations().Create(); err != nil {
		return 0, fmt.Errorf("failed to execute logging definition create operations: %w", err)
	}

	// update logging definition
	loggingDefinition.Reconciled = util.Ptr(true)
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

	// execute logging definition delete operations
	if err := c.getLoggingDefinitionOperations().Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging definition delete operations: %w", err)
	}

	return 0, nil
}
