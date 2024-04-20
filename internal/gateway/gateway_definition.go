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
	yamlDocument, err := createGatewayDefinitionYamlDocument(r, gatewayDefinition)
	if err != nil {
		return 0, fmt.Errorf("failed to create yaml document: %w", err)
	}

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: util.Ptr(fmt.Sprintf("%s-gateway", *gatewayDefinition.Name)),
		},
		YAMLDocument: &yamlDocument,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &workloadDefinition)
	if err != nil {
		return 0, fmt.Errorf("failed to create workload definition in threeport API: %w", err)
	}

	// update gateway definition
	gatewayDefinition.Reconciled = util.Ptr(true)
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
	yamlDocument, err := createGatewayDefinitionYamlDocument(r, gatewayDefinition)
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
	gatewayDefinition.WorkloadDefinitionID = workloadDefinition.ID
	gatewayDefinition.Reconciled = util.Ptr(true)
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

	// get gateway http and tcp ports
	gatewayHttpPorts, gatewayTcpPorts, err := client.GetGatewayHttpAndTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to get gateway http and tcp ports by gateway definition ID: %w", err)
	}

	// delete gateway http ports
	for _, httpPort := range *gatewayHttpPorts {
		_, err := client.DeleteGatewayHttpPort(r.APIClient, r.APIServer, *httpPort.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to delete gateway http port: %w", err)
		}
	}

	// delete gateway tcp ports
	for _, tcpPort := range *gatewayTcpPorts {
		_, err := client.DeleteGatewayTcpPort(r.APIClient, r.APIServer, *tcpPort.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to delete gateway tcp port: %w", err)
		}
	}

	log.V(1).Info(
		"gateway definition deleted",
		"gatewayDefinitionID", gatewayDefinition.ID,
	)

	return 0, nil
}

// createGatewayDefinitionYamlDocument creates a YAML document containing the Kubernetes
// manifests for a gateway definition.
func createGatewayDefinitionYamlDocument(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) (string, error) {
	// create Gloo virtual service definition

	manifests := []string{}

	domain, adminEmail, err := getDomainInfo(r, gatewayDefinition)
	if err != nil {
		return "", fmt.Errorf("failed to get domain info: %w", err)
	}

	// create Gloo virtual service definitions
	virtualServices, err := getVirtualServicesYaml(r, gatewayDefinition, domain)
	if err != nil {
		return "", fmt.Errorf("failed to create virtual service: %w", err)
	}
	manifests = append(manifests, virtualServices...)

	// create Gloo tcp gateway definitions
	tcpGateways, err := getTcpGatewaysYaml(r, gatewayDefinition)
	if err != nil {
		return "", fmt.Errorf("failed to create tcp gateway: %w", err)
	}
	manifests = append(manifests, tcpGateways...)

	tlsEnabled, err := getTlsEnabled(r, gatewayDefinition)
	if err != nil {
		return "", fmt.Errorf("failed to get tls enabled: %w", err)
	}
	if tlsEnabled {

		if domain == "" {
			return "", fmt.Errorf("failed to create issuer and certificate, domain is empty")
		}

		// create cert manager issuer definition
		issuer, err := getIssuerYaml(gatewayDefinition, domain, adminEmail)
		if err != nil {
			return "", fmt.Errorf("failed to create issuer: %w", err)
		}
		manifests = append(manifests, issuer)

		// create cert manager certificate definition
		certificate, err := getCertificateYaml(gatewayDefinition, domain)
		if err != nil {
			return "", fmt.Errorf("failed to create certificate: %w", err)
		}
		manifests = append(manifests, certificate)
	}

	// concatenate manifests into a single YAML document and return
	return util.HyphenDelimitedString(manifests), nil
}

// getTlsEnabled returns true if any of the HTTP or TCP ports in a gateway
// definition have TLS enabled.
func getTlsEnabled(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) (bool, error) {
	gatewayHttpPorts, gatewayTcpPorts, err := client.GetGatewayHttpAndTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return false, fmt.Errorf("failed to get gateway http and tcp ports by gateway definition ID: %w", err)
	}

	for _, httpPort := range *gatewayHttpPorts {
		if httpPort.TLSEnabled != nil && *httpPort.TLSEnabled {
			return true, nil
		}
	}

	for _, tcpPort := range *gatewayTcpPorts {
		if tcpPort.TLSEnabled != nil && *tcpPort.TLSEnabled {
			return true, nil
		}
	}

	return false, nil
}

// getDomainInfo returns the domain and admin email for a gateway definition.
func getDomainInfo(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) (string, string, error) {
	domain := ""
	adminEmail := ""

	if gatewayDefinition.DomainNameDefinitionID != nil {

		domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *gatewayDefinition.DomainNameDefinitionID)
		if err != nil {
			return "", "", fmt.Errorf("failed to get domain name definition by ID: %w", err)
		}

		// construct domain based on subdomain, if provided
		switch {
		case gatewayDefinition.SubDomain != nil && *gatewayDefinition.SubDomain != "":
			domain = getSubDomain(gatewayDefinition, domainNameDefinition)
		case domainNameDefinition != nil:
			domain = *domainNameDefinition.Domain
		default:
			return "", "", fmt.Errorf("failed to create domain, domain name definition is nil")
		}

		adminEmail = *domainNameDefinition.AdminEmail
	}

	return domain, adminEmail, nil
}
