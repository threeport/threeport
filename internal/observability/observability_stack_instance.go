package observability

import (
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Helm configuration to configure Grafana
// for prometheus metrics scraping. This is
// passed in to the observability dashboard definition
// when metrics are enabled.
const grafanaPrometheusServiceMonitor = `
serviceMonitor:
  # If true, a ServiceMonitor CRD is created for a prometheus operator
  # https://github.com/coreos/prometheus-operator
  #
  enabled: true

  # Scrape interval. If not set, the Prometheus default scrape interval is used.
  #
  interval: ""
`

// observabilityStackInstanceCreated reconciles state for a created
// observability stack instance.
func observabilityStackInstanceCreated(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	// get observability stack definition
	observabilityStackDefinition, err := client.GetObservabilityStackDefinitionByID(
		r.APIClient,
		r.APIServer,
		*observabilityStackInstance.ObservabilityStackDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get observability stack definition: %w", err)
	}
	if !*observabilityStackDefinition.Reconciled {
		return 0, fmt.Errorf("observability stack definition not reconciled")
	}

	// create observability stack instance config
	c := &ObservabilityStackInstanceConfig{
		r:                            r,
		observabilityStackInstance:   observabilityStackInstance,
		observabilityStackDefinition: observabilityStackDefinition,
		log:                          log,
	}

	// set observability stack instance values
	if err := c.setMergedObservabilityStackInstanceValues(); err != nil {
		return 0, fmt.Errorf("failed to set observability stack instance values: %w", err)
	}

	// execute observability stack instance create operations
	if err := c.getObservabilityStackInstanceOperations().Create(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack create operations: %w", err)
	}

	// update observability stack instance
	observabilityStackInstance.Reconciled = util.Ptr(true)
	if _, err := client.UpdateObservabilityStackInstance(
		r.APIClient,
		r.APIServer,
		observabilityStackInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update observability stack instance: %w", err)
	}

	return 0, nil
}

// observabilityStackInstanceUpdated reconciles state for an updated
// observability stack instance.
func observabilityStackInstanceUpdated(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// observabilityStackInstanceDeleted reconciles state for a deleted
// observability stack instance.
func observabilityStackInstanceDeleted(
	r *controller.Reconciler,
	observabilityStackInstance *v0.ObservabilityStackInstance,
	log *logr.Logger,
) (int64, error) {
	// create observability stack instance config
	c := &ObservabilityStackInstanceConfig{
		r:                            r,
		observabilityStackInstance:   observabilityStackInstance,
		observabilityStackDefinition: nil,
		log:                          log,
	}

	// execute observability stack delete operations
	if err := c.getObservabilityStackInstanceOperations().Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute observability stack delete operations: %w", err)
	}

	return 0, nil
}
