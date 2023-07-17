package gateway

import (
	"fmt"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"

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
	virtualService := CreateVirtualService(gatewayDefinition)

	// marshal virtual service definition into YAML
	virtualServiceBytes, err := yaml.Marshal(virtualService.Object)
	if err != nil {
		return fmt.Errorf("error marshaling YAML: %w", err)
	}
	virtualServiceManifest := string(virtualServiceBytes)

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: gatewayDefinition.Name,
		},
		YAMLDocument: &virtualServiceManifest,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &workloadDefinition)
	if err != nil {
		return fmt.Errorf("failed to create workload definition in threeport API: %w", err)
	}

	// update gateway definition
	gatewayDefinitionReconciled := true
	gatewayDefinition.WorkloadDefinitionID = createdWorkloadDefinition.ID
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
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return nil
}

// gatewayDefinitionDeleted performs reconciliation when a gateway definition
// has been deleted.
func gatewayDefinitionUpdated(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) error {

	// create Gloo virtual service definition
	virtualService := CreateVirtualService(gatewayDefinition)

	// marshal virtual service definition into YAML
	virtualServiceBytes, err := yaml.Marshal(virtualService)
	if err != nil {
		return fmt.Errorf("error marshaling YAML: %w", err)
	}
	virtualServiceManifest := string(virtualServiceBytes)

	// get workload definition
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.WorkloadDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}

	// update workload definition
	workloadDefinition.YAMLDocument = &virtualServiceManifest
	_, err = client.UpdateWorkloadDefinition(r.APIClient, r.APIServer, workloadDefinition)
	if err != nil {
		return fmt.Errorf("failed to create workload definition in threeport API: %w", err)
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

	if gatewayDefinition.WorkloadDefinitionID == nil {
		return fmt.Errorf("failed to delete workload definition, workload definition ID is nil")
	}

	// delete workload definition
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
