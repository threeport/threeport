package kubernetesruntime

import (
	"errors"
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// getMetricsOperations returns a set of operations for configuring metrics
func (c *KubernetesRuntimeInstanceConfig) getMetricsOperations(metricsDefinitionID *uint) *util.Operations {

	operations := util.Operations{}

	var createdMetricsDefinitionID *uint
	var err error

	// append metrics definition operations
	operations.AppendOperation(util.Operation{
		Name: "metrics definition",
		Create: func() error {
			createdMetricsDefinitionID, err = c.createMetricsDefinition()
			if err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err = c.deleteMetricsDefinition(metricsDefinitionID); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append metrics instance operations
	operations.AppendOperation(util.Operation{
		Name: "metrics instance",
		Create: func() error {
			if err := c.createMetricsInstance(createdMetricsDefinitionID); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteMetricsInstance(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}

// createMetricsDefinition configures a metrics definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createMetricsDefinition() (*uint, error) {
	// create metrics definition
	createdMetricsDefinition, err := client.CreateMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.MetricsDefinition{
			Definition: v0.Definition{
				Name: c.kubernetesRuntimeInstance.Name,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics definition: %w", err)
	}

	return createdMetricsDefinition.ID, nil
}

// deleteMetricsDefinition disables a metrics definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteMetricsDefinition(metricsDefinitionID *uint) error {

	// delete metrics definition
	_, err := client.DeleteMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*metricsDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics definition: %w", err)
	}

	return nil
}

// createMetricsInstance configures a metrics for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createMetricsInstance(metricsDefinitionID *uint) error {
	// create metrics instance
	createdMetricsInstance, err := client.CreateMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.MetricsInstance{
			Instance: v0.Instance{
				Name: c.kubernetesRuntimeInstance.Name,
			},
			MetricsDefinitionID:         metricsDefinitionID,
			KubernetesRuntimeInstanceID: c.kubernetesRuntimeInstance.ID,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return fmt.Errorf("failed to create metrics instance: %w", err)
	}

	// update kubernetes runtime instance with metrics instance ID
	c.kubernetesRuntimeInstance.MetricsInstanceID = util.SqlNullInt64(createdMetricsInstance.ID)

	return nil
}

// deleteMetricsInstance disables a metrics definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteMetricsInstance() error {
	// delete metrics instance
	_, err := client.DeleteMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		uint(c.kubernetesRuntimeInstance.MetricsInstanceID.Int64),
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics instance: %w", err)
	}

	// update kubernetes runtime instance with metrics instance ID
	c.kubernetesRuntimeInstance.MetricsInstanceID = util.SqlNullInt64(nil)

	return nil
}
