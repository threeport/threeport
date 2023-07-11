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

	// create Gloo vertual service definition
	virtualService := CreateVirtualService()

	// marshal virtual service definition into YAML
	virtualServiceBytes, err := yaml.Marshal(virtualService)
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

	log.V(1).Info(
		"gateway definition created",
		"gatewayDefinitionID", createdWorkloadDefinition.ID,
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
	return nil
}

// gatewayDefinitionDeleted performs reconciliation when a gateway definition
// has been deleted.
func gatewayDefinitionDeleted(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) error {

	// get related gateway definitions
	gatewayDefinition, err := client.GetGatewayDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway definition by gateway definition ID: %w", err)
	}

	// related gateway definition
	_, err = client.DeleteGatewayDefinition(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return fmt.Errorf("failed to delete gateway definition with ID %d: %w", gatewayDefinition.ID, err)
	}
	log.V(1).Info(
		"gateway definition deleted",
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return nil
}
