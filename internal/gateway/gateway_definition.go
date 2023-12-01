package gateway

import (
	"fmt"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// gatewayDefinitionCreated performs reconciliation when a gateway definition
// has been created.
func gatewayDefinitionCreated(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) (int64, error) {

	// create gateway kubernetes manifests
	yamlDocument, err := createYAMLDocument(r, gatewayDefinition)
	if err != nil {
		return 0, fmt.Errorf("failed to create yaml document: %w", err)
	}

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: util.StringPtr(fmt.Sprintf("%s-gateway", *gatewayDefinition.Name)),
		},
		YAMLDocument: &yamlDocument,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &workloadDefinition)
	if err != nil {
		return 0, fmt.Errorf("failed to create workload definition in threeport API: %w", err)
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
		return 0, fmt.Errorf("failed to update gateway definition in threeport API: %w", err)
	}

	log.V(1).Info(
		"gateway definition created",
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return 0, nil
}

// gatewayDefinitionUpdated performs reconciliation when a gateway definition
// has been updated.
func gatewayDefinitionUpdated(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) (int64, error) {

	// create gateway kubernetes manifests
	yamlDocument, err := createYAMLDocument(r, gatewayDefinition)
	if err != nil {
		return 0, fmt.Errorf("failed to create yaml document: %w", err)
	}

	// get workload definition
	if gatewayDefinition.WorkloadDefinitionID == nil {
		return 0, fmt.Errorf("failed to update workload definition, workload definition ID is nil")
	}
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.WorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}

	// update workload definition
	workloadDefinition.YAMLDocument = &yamlDocument
	_, err = client.UpdateWorkloadDefinition(r.APIClient, r.APIServer, workloadDefinition)
	if err != nil {
		return 0, fmt.Errorf("failed to update workload definition in threeport API: %w", err)
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
		return 0, fmt.Errorf("failed to update gateway definition in threeport API: %w", err)
	}

	log.V(1).Info(
		"gateway definition created",
		"gatewayDefinitionID", workloadDefinition.ID,
	)

	return 0, nil
}

// gatewayDefinitionDeleted performs reconciliation when a gateway definition
// has been deleted.
func gatewayDefinitionDeleted(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if gatewayDefinition.DeletionScheduled == nil {
		return 0, nil
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if gatewayDefinition.DeletionConfirmed != nil {
		return 0, nil
	}

	// delete workload definition
	if gatewayDefinition.WorkloadDefinitionID == nil {
		return 0, nil
	}
	_, err := client.DeleteWorkloadDefinition(r.APIClient, r.APIServer, *gatewayDefinition.WorkloadDefinitionID)
	if err != nil {
		return 0, nil
	}

	log.V(1).Info(
		"gateway definition deleted",
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return 0, nil
}

// createYAMLDocument creates a YAML document containing the Kubernetes
// manifests for a gateway definition.
func createYAMLDocument(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) (string, error) {
	// create Gloo virtual service definition

	manifests := []string{}

	domain := ""
	adminEmail := ""

	if gatewayDefinition.DomainNameDefinitionID != nil {

		domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *gatewayDefinition.DomainNameDefinitionID)
		if err != nil {
			return "", fmt.Errorf("failed to get domain name definition by ID: %w", err)
		}

		// construct domain based on subdomain, if provided
		if gatewayDefinition.SubDomain != nil {
			domain = fmt.Sprintf("%s.%s", *gatewayDefinition.SubDomain, *domainNameDefinition.Domain)
		} else {
			domain = *domainNameDefinition.Domain
		}

		adminEmail = *domainNameDefinition.AdminEmail
	}

	// create Gloo virtual service definition
	virtualService, err := createVirtualService(gatewayDefinition, domain)
	if err != nil {
		return "", fmt.Errorf("failed to create virtual service: %w", err)
	}
	manifests = append(manifests, virtualService)

	if *gatewayDefinition.TLSEnabled {

		if domain == "" {
			return "", fmt.Errorf("failed to create issuer and certificate, domain is empty")
		}

		// create cert manager issuer definition
		issuer, err := createIssuer(gatewayDefinition, domain, adminEmail)
		if err != nil {
			return "", fmt.Errorf("failed to create issuer: %w", err)
		}
		manifests = append(manifests, issuer)

		// create cert manager certificate definition
		certificate, err := createCertificate(gatewayDefinition, domain)
		if err != nil {
			return "", fmt.Errorf("failed to create certificate: %w", err)
		}
		manifests = append(manifests, certificate)
	}

	// concatenate manifests into a single YAML document
	yamlDocument := ""
	for _, manifest := range manifests {
		yamlDocument = fmt.Sprintf("%s---\n%s\n", yamlDocument, manifest)
	}

	return yamlDocument, nil
}
