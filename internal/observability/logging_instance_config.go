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

// LoggingInstanceConfig contains the configuration for a logging instance
// reconcile function.
type LoggingInstanceConfig struct {
	r                                  *controller.Reconciler
	loggingInstance                    *v0.LoggingInstance
	loggingDefinition                  *v0.LoggingDefinition
	log                                *logr.Logger
	loggingNamespace                   string
	lokiHelmWorkloadInstanceValues     string
	promtailHelmWorkloadInstanceValues string
}

// getLoggingInstanceOperations returns a list of operations for a logging instance.
func (c *LoggingInstanceConfig) getLoggingInstanceOperations() *util.Operations {
	operations := util.Operations{}

	// append loki operations
	operations.AppendOperation(util.Operation{
		Name:   "loki",
		Create: c.createLokiHelmWorkloadInstance,
		Delete: c.deleteLokiHelmWorkloadInstance,
	})

	// append promtail operations
	operations.AppendOperation(util.Operation{
		Name:   "promtail",
		Create: c.createPromtailHelmWorkloadInstance,
		Delete: c.deletePromtailHelmWorkloadInstance,
	})

	return &operations
}

// createLokiHelmWorkloadInstance creates loki helm workload instance
func (c *LoggingInstanceConfig) createLokiHelmWorkloadInstance() error {
	// create loki helm workload instance
	lokiHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.Ptr(LokiHelmChartName(*c.loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.loggingDefinition.LokiHelmWorkloadDefinitionID,
			ValuesDocument:              &c.lokiHelmWorkloadInstanceValues,
			ReleaseNamespace:            &c.loggingNamespace,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create loki helm workload instance: %w", err)
	}

	// update logging instance loki helm workload instance id
	c.loggingInstance.LokiHelmWorkloadInstanceID = lokiHelmWorkloadInstance.ID

	return nil
}

// deleteLokiHelmWorkloadInstance deletes loki helm workload instance
func (c *LoggingInstanceConfig) deleteLokiHelmWorkloadInstance() error {

	// delete loki helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.LokiHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
	}

	return nil
}

// createPromtailHelmWorkloadInstance creates promtail helm workload instance
func (c *LoggingInstanceConfig) createPromtailHelmWorkloadInstance() error {
	// create promtail helm workload instance
	promtailHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.HelmWorkloadInstance{
			Instance: v0.Instance{
				Name: util.Ptr(PromtailHelmChartName(*c.loggingInstance.Name)),
			},
			KubernetesRuntimeInstanceID: c.loggingInstance.KubernetesRuntimeInstanceID,
			HelmWorkloadDefinitionID:    c.loggingDefinition.PromtailHelmWorkloadDefinitionID,
			ValuesDocument:              &c.promtailHelmWorkloadInstanceValues,
			ReleaseNamespace:            &c.loggingNamespace,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create promtail helm workload instance: %w", err)
	}

	// update logging instance promtail helm workload instance id
	c.loggingInstance.PromtailHelmWorkloadInstanceID = promtailHelmWorkloadInstance.ID

	return nil
}

// deletePromtailHelmWorkloadInstance creates promtail helm workload instance
func (c *LoggingInstanceConfig) deletePromtailHelmWorkloadInstance() error {
	// delete promtail helm workload instance
	if _, err := client.DeleteHelmWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		*c.loggingInstance.PromtailHelmWorkloadInstanceID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete promtail helm workload instance: %w", err)
	}

	return nil
}
