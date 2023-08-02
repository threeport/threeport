package gateway

// import (
// 	"fmt"

// 	"github.com/go-logr/logr"
// 	v0 "github.com/threeport/threeport/pkg/api/v0"
// 	client "github.com/threeport/threeport/pkg/client/v0"
// 	controller "github.com/threeport/threeport/pkg/controller/v0"
// )

// // domainNameDefinitionCreated performs reconciliation when a domain name definition
// // has been created.
// func domainNameDefinitionCreated(
// 	r *controller.Reconciler,
// 	domainNameDefinition *v0.DomainNameDefinition,
// 	log *logr.Logger,
// ) error {

// 	// create domain name definition
// 	dnsEndpoint, err := createDnsEndpoint()
// 	if err != nil {
// 		return fmt.Errorf("failed to create domain name: %w", err)
// 	}

// 	// construct workload definition object
// 	workloadDefinition := v0.WorkloadDefinition{
// 		Definition: v0.Definition{
// 			Name: domainNameDefinition.Name,
// 		},
// 		YAMLDocument: &dnsEndpoint,
// 	}

// 	// create workload definition
// 	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &workloadDefinition)
// 	if err != nil {
// 		return fmt.Errorf("failed to create workload definition in threeport API: %w", err)
// 	}

// 	// update gateway definition
// 	domainNameDefinitionReconciled := true
// 	domainNameDefinition.Reconciled = &domainNameDefinitionReconciled
// 	domainNameDefinition.WorkloadDefinitionID = createdWorkloadDefinition.ID
// 	_, err = client.UpdateDomainNameDefinition(
// 		r.APIClient,
// 		r.APIServer,
// 		domainNameDefinition,
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to update gateway definition in threeport API: %w", err)
// 	}

// 	log.V(1).Info(
// 		"domain name definition created",
// 		"domainNameDefinitionID", domainNameDefinition.ID,
// 	)

// 	return nil
// }

// // domainNameDefinitionUpdated performs reconciliation when a domain name definition
// // has been updated.
// func domainNameDefinitionUpdated(
// 	r *controller.Reconciler,
// 	domainNameDefinition *v0.DomainNameDefinition,
// 	log *logr.Logger,
// ) error {

// 	// create domain name definition
// 	dnsEndpoint, err := createDnsEndpoint()
// 	if err != nil {
// 		return fmt.Errorf("failed to create domain name: %w", err)
// 	}

// 	// get workload definition
// 	if domainNameDefinition.WorkloadDefinitionID == nil {
// 		return fmt.Errorf("failed to update workload definition, workload definition ID is nil")
// 	}
// 	workloadDefinition, err := client.GetWorkloadDefinitionByID(
// 		r.APIClient,
// 		r.APIServer,
// 		*domainNameDefinition.WorkloadDefinitionID,
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
// 	}

// 	// update workload definition
// 	workloadDefinition.YAMLDocument = &dnsEndpoint
// 	_, err = client.UpdateWorkloadDefinition(r.APIClient, r.APIServer, workloadDefinition)
// 	if err != nil {
// 		return fmt.Errorf("failed to update workload definition in threeport API: %w", err)
// 	}

// 	// update gateway definition
// 	domainNameDefinitionReconciled := true
// 	domainNameDefinition.WorkloadDefinitionID = workloadDefinition.ID
// 	domainNameDefinition.Reconciled = &domainNameDefinitionReconciled
// 	_, err = client.UpdateDomainNameDefinition(
// 		r.APIClient,
// 		r.APIServer,
// 		domainNameDefinition,
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to update domain name definition in threeport API: %w", err)
// 	}

// 	log.V(1).Info(
// 		"domian name definition created",
// 		"domainNameDefinitionID", workloadDefinition.ID,
// 	)

// 	return nil
// }

// // domainNameDefinitionDeleted performs reconciliation when a domain name definition
// // has been deleted.
// func domainNameDefinitionDeleted(
// 	r *controller.Reconciler,
// 	domainNameDefinition *v0.DomainNameDefinition,
// 	log *logr.Logger,
// ) error {

// 	// delete workload definition
// 	if domainNameDefinition.WorkloadDefinitionID == nil {
// 		return fmt.Errorf("failed to delete workload definition, workload definition ID is nil")
// 	}
// 	_, err := client.DeleteWorkloadDefinition(r.APIClient, r.APIServer, *domainNameDefinition.WorkloadDefinitionID)
// 	if err != nil {
// 		return fmt.Errorf("failed to delete workload definition with ID %d: %w", *domainNameDefinition.WorkloadDefinitionID, err)
// 	}

// 	log.V(1).Info(
// 		"gateway definition deleted",
// 		"gatewayDefinitionID", domainNameDefinition.ID,
// 	)

// 	return nil
// }
