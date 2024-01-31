package kubernetesruntime

import (
	"errors"
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// getMetricsOperations returns a set of operations for configuring metrics
func (c *KubernetesRuntimeInstanceConfig) getLoggingOperations(loggingDefinitionID *uint) *util.Operations {

	operations := util.Operations{}

	var loggingMetricsDefinitionID *uint
	var err error

	// append metrics definition operations
	operations.AppendOperation(util.Operation{
		Name: "logging definition",
		Create: func() error {
			loggingMetricsDefinitionID, err = c.createLoggingDefinition()
			if err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err = c.deleteLoggingDefinition(loggingDefinitionID); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append metrics instance operations
	operations.AppendOperation(util.Operation{
		Name: "logging instance",
		Create: func() error {
			if err := c.createLoggingInstance(loggingMetricsDefinitionID); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteLoggingInstance(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}

// createLoggingDefinition configures a logging definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createLoggingDefinition() (*uint, error) {
	// create logging definition
	createdLoggingDefinition, err := client.CreateLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.LoggingDefinition{
			Definition: v0.Definition{
				Name: c.kubernetesRuntimeInstance.Name,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create logging definition: %w", err)
	}

	return createdLoggingDefinition.ID, nil
}

// deleteLoggingDefinition disables logging instance for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteLoggingDefinition(loggingDefinitionID *uint) error {
	// delete logging definition
	_, err := client.DeleteLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*loggingDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging definition: %w", err)
	}

	return nil
}

// createLoggingInstance configures a logging instance for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createLoggingInstance(loggingDefinitionID *uint) error {
	// create logging instance
	createdLoggingInstance, err := client.CreateLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.LoggingInstance{
			Instance: v0.Instance{
				Name: c.kubernetesRuntimeInstance.Name,
			},
			LoggingDefinitionID:         loggingDefinitionID,
			KubernetesRuntimeInstanceID: c.kubernetesRuntimeInstance.ID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create logging instance: %w", err)
	}

	// update kubernetes runtime instance with logging instance ID
	c.kubernetesRuntimeInstance.LoggingInstanceID = util.SqlNullInt64(createdLoggingInstance.ID)

	return nil
}

// deleteLoggingInstance disables logging instance for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteLoggingInstance() error {
	// delete logging instance
	_, err := client.DeleteLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		uint(c.kubernetesRuntimeInstance.LoggingInstanceID.Int64),
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging instance: %w", err)
	}

	// update kubernetes runtime instance with logging instance ID
	c.kubernetesRuntimeInstance.LoggingInstanceID = util.SqlNullInt64(nil)

	return nil
}
