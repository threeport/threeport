package gateway

import (
	"fmt"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// gatewayDefinitionCreated performs reconciliation when a gateway definition
// has been created.
func gatewayDefinitionCreated(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) error {

	// create Gloo virtual service definition
	virtualService, err := createVirtualService(gatewayDefinition)
	if err != nil {
		return fmt.Errorf("failed to create virtual service: %w", err)
	}

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: gatewayDefinition.Name,
		},
		YAMLDocument: &virtualService,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &workloadDefinition)
	if err != nil {
		return fmt.Errorf("failed to create workload definition in threeport API: %w", err)
	}

	// update gateway definition
	gatewayDefinitionReconciled := true
	gatewayDefinition.Reconciled = &gatewayDefinitionReconciled
	gatewayDefinition.WorkloadDefinitionID = createdWorkloadDefinition.ID
	_, err = client.UpdateGatewayDefinition(
		r.APIClient,
		r.APIServer,
		gatewayDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to update gateway definition in threeport API: %w", err)
	}

	log.V(1).Info(
		"gateway definition created",
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return nil
}

// gatewayDefinitionUpdated performs reconciliation when a gateway definition
// has been updated.
func gatewayDefinitionUpdated(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) error {

	// create Gloo virtual service definition
	virtualService, err := createVirtualService(gatewayDefinition)
	if err != nil {
		return fmt.Errorf("failed to create virtual service: %w", err)
	}

	// get workload definition
	if gatewayDefinition.WorkloadDefinitionID == nil {
		return fmt.Errorf("failed to update workload definition, workload definition ID is nil")
	}
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.WorkloadDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}

	// update workload definition
	workloadDefinition.YAMLDocument = &virtualService
	_, err = client.UpdateWorkloadDefinition(r.APIClient, r.APIServer, workloadDefinition)
	if err != nil {
		return fmt.Errorf("failed to update workload definition in threeport API: %w", err)
	}

	// update gateway definition
	gatewayDefinitionReconciled := true
	gatewayDefinition.WorkloadDefinitionID = workloadDefinition.ID
	gatewayDefinition.Reconciled = &gatewayDefinitionReconciled
	_, err = client.UpdateGatewayDefinition(
		r.APIClient,
		r.APIServer,
		gatewayDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to update gateway definition in threeport API: %w", err)
	}

	log.V(1).Info(
		"gateway definition created",
		"gatewayDefinitionID", workloadDefinition.ID,
	)

	return nil
}

// gatewayDefinitionDeleted performs reconciliation when a gateway definition
// has been deleted.
func gatewayDefinitionDeleted(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) error {

	// delete workload definition
	if gatewayDefinition.WorkloadDefinitionID == nil {
		return fmt.Errorf("failed to delete workload definition, workload definition ID is nil")
	}
	_, err := client.DeleteWorkloadDefinition(r.APIClient, r.APIServer, *gatewayDefinition.WorkloadDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to delete workload definition with ID %d: %w", *gatewayDefinition.WorkloadDefinitionID, err)
	}

	log.V(1).Info(
		"gateway definition deleted",
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return nil
}
