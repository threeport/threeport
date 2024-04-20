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

// LoggingDefinitionConfig contains configuration for a logging
// definition reconcile function.
type LoggingDefinitionConfig struct {
	r                                    *controller.Reconciler
	loggingDefinition                    *v0.LoggingDefinition
	log                                  *logr.Logger
	lokiHelmWorkloadDefinitionValues     string
	promtailHelmWorkloadDefinitionValues string
}

// getLoggingDefinitionOperations returns a list of operations for a logging definition.
func (c *LoggingDefinitionConfig) getLoggingDefinitionOperations() *util.Operations {
	operations := util.Operations{}

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name:   "loki",
		Create: c.createLokiHelmWorkloadDefinition,
		Delete: c.deleteLokiHelmWorkloadDefinition,
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name:   "promtail",
		Create: c.createPromtailHelmWorkloadDefinition,
		Delete: c.deletePromtailHelmWorkloadDefinition,
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
				Name: util.Ptr(LokiHelmChartName(*c.loggingDefinition.Name)),
			},
			Repo:           util.Ptr(GrafanaHelmRepo),
			Chart:          util.Ptr("loki"),
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
				Name: util.Ptr(PromtailHelmChartName(*c.loggingDefinition.Name)),
			},
			Repo:           util.Ptr(GrafanaHelmRepo),
			Chart:          util.Ptr("promtail"),
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
