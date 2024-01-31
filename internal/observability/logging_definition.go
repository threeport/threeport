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

type LoggingDefinitionConfig struct {
	r                 *controller.Reconciler
	loggingDefinition *v0.LoggingDefinition
	log               *logr.Logger
}

// loggingDefinitionCreated reconciles state for a new kubernetes
// runtime instance.
func loggingDefinitionCreated(
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
	operations := getLoggingDefinitionOperations(c)

	// execute logging definition create operations
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute logging definition create operations: %w", err)
	}

	// wait for helm workload definitions to be reconciled
	for _, id := range []*uint{
		loggingDefinition.GrafanaHelmWorkloadDefinitionID,
		loggingDefinition.LokiHelmWorkloadDefinitionID,
		loggingDefinition.PromtailHelmWorkloadDefinitionID,
	} {
		current := id
		if err := util.Retry(60, 1, func() error {
			if hwrd, err := client.GetHelmWorkloadDefinitionByID(
				r.APIClient,
				r.APIServer,
				*current,
			); err != nil {
				return fmt.Errorf("failed to get helm workload definition: %w", err)
			} else if !*hwrd.Reconciled {
				return fmt.Errorf("helm workload definition is not reconciled")
			}
			return nil
		}); err != nil {
			return 0, fmt.Errorf("failed to wait for helm workload definition to be reconciled: %w", err)
		}
	}

	// update logging definition reconciled field
	loggingDefinition.Reconciled = util.BoolPtr(true)
	_, err := client.UpdateLoggingDefinition(
		r.APIClient,
		r.APIServer,
		loggingDefinition,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update logging definition reconciled field: %w", err)
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
	// create logging definition config
	c := &LoggingDefinitionConfig{
		r:                 r,
		loggingDefinition: loggingDefinition,
		log:               log,
	}

	// get logging operations
	operations := getLoggingDefinitionOperations(c)

	// execute logging definition delete operations
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute logging definition delete operations: %w", err)
	}

	// wait for helm workload definitions to be deleted
	for _, id := range []*uint{
		loggingDefinition.GrafanaHelmWorkloadDefinitionID,
		loggingDefinition.LokiHelmWorkloadDefinitionID,
		loggingDefinition.PromtailHelmWorkloadDefinitionID,
	} {
		current := id
		if err := util.Retry(60, 1, func() error {
			if _, err := client.GetHelmWorkloadDefinitionByID(
				r.APIClient,
				r.APIServer,
				*current,
			); err == nil {
				return fmt.Errorf("helm workload definition still present: %w", err)
			}
			return nil
		}); err != nil {
			return 0, fmt.Errorf("failed to wait for helm workload definition to be deleted: %w", err)
		}
	}

	return 0, nil
}

// createGrafanaHelmWorkloadDefinition creates a grafana helm workload definition.
func (c *LoggingDefinitionConfig) createGrafanaHelmWorkloadDefinition() error {
	var err error
	// ensure grafana helm workload definition exists
	grafanaHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr(GrafanaChartName(*c.loggingDefinition.Name)),
			},
			Repo:           util.StringPtr(GrafanaHelmRepo),
			Chart:          util.StringPtr("grafana"),
			HelmChartVersion:   c.loggingDefinition.GrafanaHelmChartVersion,
			HelmValuesDocument: c.loggingDefinition.GrafanaHelmValuesDocument,
		})
	if err != nil && !errors.Is(err, client.ErrConflict) {
		// only return error if it isn't a conflict, since we
		// expect both MetricsInstance and LoggingInstance to depend
		// on the same HelmWorkloadDefinition for Grafana
		return fmt.Errorf("failed to create grafana helm workload definition: %w", err)
	} else if err != nil && errors.Is(err, client.ErrConflict) {
		grafanaHelmWorkloadDefinition, err = client.GetHelmWorkloadDefinitionByName(
			c.r.APIClient,
			c.r.APIServer,
			GrafanaChartName(*c.loggingDefinition.Name),
		)
	}
	c.loggingDefinition.GrafanaHelmWorkloadDefinitionID = grafanaHelmWorkloadDefinition.ID
	return nil
}

// deleteGrafanaHelmWorkloadDefinition creates a grafana helm workload definition.
func (c *LoggingDefinitionConfig) deleteGrafanaHelmWorkloadDefinition() error {
	// delete grafana helm workload definition
	_, err := client.DeleteHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingDefinition.GrafanaHelmWorkloadDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete grafana helm workload definition: %w", err)
	}

	return nil
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
			HelmChartVersion:   c.loggingDefinition.LokiHelmChartVersion,
			HelmValuesDocument: c.loggingDefinition.LokiHelmValuesDocument,
		})
	if err != nil {
		return fmt.Errorf("failed to create loki helm workload definition: %w", err)
	}
	c.loggingDefinition.LokiHelmWorkloadDefinitionID = lokiHelmWorkloadDefinition.ID
	return nil
}

// deleteLokiHelmWorkloadDefinition creates a loki helm workload definition.
func (c *LoggingDefinitionConfig) deleteLokiHelmWorkloadDefinition() error {
	// delete loki helm workload definition
	_, err := client.DeleteHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingDefinition.LokiHelmWorkloadDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
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
			HelmChartVersion:   c.loggingDefinition.PromtailHelmChartVersion,
			HelmValuesDocument: c.loggingDefinition.PromtailHelmValuesDocument,
		})
	if err != nil {
		return fmt.Errorf("failed to create promtail helm workload definition: %w", err)
	}
	c.loggingDefinition.PromtailHelmWorkloadDefinitionID = promtailHelmWorkloadDefinition.ID
	return nil
}

// deletePromtailHelmWorkloadDefinition creates a promtail helm workload definition.
func (c *LoggingDefinitionConfig) deletePromtailHelmWorkloadDefinition() error {
	// delete promtail helm workload definition
	_, err := client.DeleteHelmWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingDefinition.PromtailHelmWorkloadDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete promtail helm workload definition: %w", err)
	}
	return nil
}

// getLoggingDefinitionOperations returns a list of operations for a logging definition.
func getLoggingDefinitionOperations(c *LoggingDefinitionConfig) *util.Operations {
	operations := util.Operations{}

	// append grafana operations
	operations.AppendOperation(util.Operation{
		Name: "grafana",
		Create: func() error {
			if err := c.createGrafanaHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteGrafanaHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name: "loki",
		Create: func() error {
			if err := c.createLokiHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteLokiHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name: "promtail",
		Create: func() error {
			if err := c.createPromtailHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deletePromtailHelmWorkloadDefinition(); err != nil {
				return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}
