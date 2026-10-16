package gateway

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/gateway/assets"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// setIngressBackend sets the service and port an ingress sends traffic to.
func setIngressBackend(object map[string]interface{}, service string, port int64) error {
	// set the service name and port on the first path
	if err := util.SetIndexedField(
		object,
		service,
		"spec",
		"rules",
		0,
		"http",
		"paths",
		0,
		"backend",
		"service",
		"name",
	); err != nil {
		return fmt.Errorf("ingress has no rules: %w", err)
	}

	// set the service port on that same path
	return util.SetIndexedField(
		object,
		port,
		"spec",
		"rules",
		0,
		"http",
		"paths",
		0,
		"backend",
		"service",
		"port",
		"number",
	)
}

// setTransportBackend sets the service and port a transport server sends traffic to.
func setTransportBackend(object map[string]interface{}, service string, port int64) error {
	// set the upstream service
	if err := util.SetIndexedField(
		object,
		service,
		"spec",
		"upstreams",
		0,
		"service",
	); err != nil {
		return fmt.Errorf("transport server has no upstreams: %w", err)
	}
	if err := util.SetIndexedField(
		object,
		port,
		"spec",
		"upstreams",
		0,
		"port",
	); err != nil {
		return err
	}

	// name the listener for this port
	return unstructured.SetNestedField(
		object,
		fmt.Sprintf("tcp-%d", port),
		"spec",
		"listener",
		"name",
	)
}

// getIngressComponentYaml returns the nginx operator component.
// The tcp listener object waits until that operator has installed its CRDs.
func getIngressComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(assets.IngressComponent)
	if err != nil {
		return "", err
	}
	componentYaml, err := util.UnstructuredToYaml(component)
	if err != nil {
		return "", err
	}
	return componentYaml, nil
}

// getExternalDnsYaml creates an ingress component that installs external-dns.
func getExternalDnsYaml(
	domain,
	provider,
	iamRoleArn,
	gcpProject,
	txtOwnerID string,
) (string, error) {
	component, err := util.UnstructuredFromYaml(assets.ExternalDNS)
	if err != nil {
		return "", err
	}

	// set the zone this install manages
	if err := unstructured.SetNestedField(
		component.Object,
		domain,
		"spec",
		"domainName",
	); err != nil {
		return "", err
	}
	if provider == "" || provider == "none" {
		unstructured.RemoveNestedField(
			component.Object,
			"spec",
			"externalDNS",
		)
		return util.UnstructuredToYaml(component)
	}

	// set the provider and the ownership args
	if err := unstructured.SetNestedField(
		component.Object,
		provider,
		"spec",
		"externalDNS",
		"provider",
	); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(
		component.Object,
		iamRoleArn,
		"spec",
		"externalDNS",
		"iamRoleArn",
	); err != nil {
		return "", err
	}
	extraArgs := []interface{}{
		"--txt-owner-id=" + txtOwnerID + "-",
		"--txt-prefix=" + txtOwnerID + "-",
	}
	if provider == "google" && gcpProject != "" {
		extraArgs = append(extraArgs, "--google-project="+gcpProject)
	}
	if err := unstructured.SetNestedSlice(
		component.Object,
		extraArgs,
		"spec",
		"externalDNS",
		"extraArgs",
	); err != nil {
		return "", err
	}

	return util.UnstructuredToYaml(component)
}

// getCertificatesComponentYaml creates the cert-manager component.
func getCertificatesComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(assets.Certificates)
	if err != nil {
		return "", err
	}
	return util.UnstructuredToYaml(component)
}

// getCleanedDomain returns a cleaned domain.
func getCleanedDomain(domain string) string {
	// strip a scheme prefix
	cleanedDomain := strings.TrimPrefix(domain, "http://")
	cleanedDomain = strings.TrimPrefix(cleanedDomain, "https://")
	return cleanedDomain
}

// getIngressesYaml creates an ingress for each HTTP port.
func getIngressesYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition, domain string) ([]string, error) {
	var manifests []string

	// clean the domain and load the http ports
	domain = getCleanedDomain(domain)
	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}
	for _, httpPort := range *gatewayHttpPorts {
		host := domain

		// copy the embedded ingress and name it for this port
		ingress, err := util.UnstructuredFromYaml(assets.Ingress)
		if err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(
			ingress.Object,
			getIngressName(gatewayDefinition, domain, *httpPort.Port),
			"metadata",
			"name",
		); err != nil {
			return nil, err
		}
		// drop the host when the domain is empty
		if host == "" {
			if err := util.RemoveIndexedField(
				ingress.Object,
				"spec",
				"rules",
				0,
				"host",
			); err != nil {
				return nil, fmt.Errorf("ingress has no rules: %w", err)
			}
		} else if err := util.SetIndexedField(
			ingress.Object,
			host,
			"spec",
			"rules",
			0,
			"host",
		); err != nil {
			return nil, fmt.Errorf("ingress has no rules: %w", err)
		}
		// set the path
		if err := util.SetIndexedField(
			ingress.Object,
			*httpPort.Path,
			"spec",
			"rules",
			0,
			"http",
			"paths",
			0,
			"path",
		); err != nil {
			return nil, fmt.Errorf("ingress has no rules: %w", err)
		}
		// redirect http when the port asks for it
		if httpPort.HTTPSRedirect != nil && *httpPort.HTTPSRedirect {
			if err := unstructured.SetNestedField(
				ingress.Object,
				"true",
				"metadata",
				"annotations",
				"nginx.org/redirect-to-https",
			); err != nil {
				return nil, fmt.Errorf("failed to set https redirect on ingress: %w", err)
			}
		}
		// terminate tls at the ingress when the port enables it
		if httpPort.TLSEnabled != nil && *httpPort.TLSEnabled {
			tls := []interface{}{
				map[string]interface{}{
					"hosts":      []interface{}{host},
					"secretName": strcase.ToKebab(domain) + "-tls",
				},
			}
			if err := unstructured.SetNestedSlice(
				ingress.Object,
				tls,
				"spec",
				"tls",
			); err != nil {
				return nil, fmt.Errorf("failed to set tls on ingress: %w", err)
			}
		}

		// render the ingress
		rendered, err := util.UnstructuredToYaml(ingress)
		if err != nil {
			return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
		}
		manifests = append(manifests, rendered)
	}

	return manifests, nil
}

// getTransportServersYaml creates a transport server for each TCP port.
func getTransportServersYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) ([]string, error) {
	var manifests []string

	// load the tcp ports
	gatewayTcpPorts, err := client.GetGatewayTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway tcp ports: %w", err)
	}
	for _, tcpPort := range *gatewayTcpPorts {
		// copy the embedded server and name it for this port
		server, err := util.UnstructuredFromYaml(assets.TransportServer)
		if err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(
			server.Object,
			fmt.Sprintf("%s-%d", *gatewayDefinition.Name, *tcpPort.Port),
			"metadata",
			"name",
		); err != nil {
			return nil, err
		}
		// name the listener for this port
		listener, _, err := unstructured.NestedMap(
			server.Object,
			"spec",
			"listener",
		)
		if err != nil {
			return nil, fmt.Errorf("transport server has no listener: %w", err)
		}
		listener["name"] = fmt.Sprintf("tcp-%d", *tcpPort.Port)
		if err := unstructured.SetNestedMap(
			server.Object,
			listener,
			"spec",
			"listener",
		); err != nil {
			return nil, err
		}
		// point the upstream at this port
		if err := setTransportBackend(server.Object, "placeholder", int64(*tcpPort.Port)); err != nil {
			return nil, err
		}

		// render the server
		rendered, err := util.UnstructuredToYaml(server)
		if err != nil {
			return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
		}
		manifests = append(manifests, rendered)
	}

	return manifests, nil
}

// getIngressName returns the name of an ingress.
func getIngressName(gatewayDefinition *v0.GatewayDefinition, domain string, port int) string {
	// use the domain, or the gateway name when the domain is empty
	domain = getCleanedDomain(domain)
	if domain == "" {
		return fmt.Sprintf("%s-%d", *gatewayDefinition.Name, port)
	}
	return fmt.Sprintf("%s-%d", strcase.ToKebab(domain), port)
}

// setSelfSignedIssuer writes a self-signed cert-manager issuer.
func setSelfSignedIssuer(object map[string]interface{}) error {
	// write the self-signed spec
	unstructured.RemoveNestedField(
		object,
		"spec",
		"acme",
	)
	return unstructured.SetNestedField(
		object,
		map[string]interface{}{},
		"spec",
		"selfSigned",
	)
}

// getIssuerYaml creates an issuer for the given domain.
func getIssuerYaml(gatewayDefinition *v0.GatewayDefinition, domain, adminEmail string) (string, error) {
	// name the issuer, then set the account email and the zone
	issuer, err := util.UnstructuredFromYaml(assets.Issuer)
	if err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(
		issuer.Object,
		strcase.ToKebab(domain),
		"metadata",
		"name",
	); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(
		issuer.Object,
		adminEmail,
		"spec",
		"acme",
		"email",
	); err != nil {
		return "", err
	}
	if err := util.SetIndexedField(
		issuer.Object,
		[]interface{}{domain},
		"spec",
		"acme",
		"solvers",
		0,
		"selector",
		"dnsZones",
	); err != nil {
		return "", fmt.Errorf("issuer has no solvers: %w", err)
	}
	return util.UnstructuredToYaml(issuer)
}

// getCertificateYaml creates a certificate for the given domain.
func getCertificateYaml(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {
	// decode the embedded certificate
	certificate, err := util.UnstructuredFromYaml(assets.Certificate)
	if err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(
		certificate.Object,
		strcase.ToKebab(domain),
		"metadata",
		"name",
	); err != nil {
		return "", err
	}
	// name the secret the certificate writes
	if err := unstructured.SetNestedField(
		certificate.Object,
		strcase.ToKebab(domain)+"-tls",
		"spec",
		"secretName",
	); err != nil {
		return "", err
	}
	// set the dns name
	if err := unstructured.SetNestedSlice(
		certificate.Object,
		[]interface{}{domain},
		"spec",
		"dnsNames",
	); err != nil {
		return "", err
	}
	// point the certificate at the issuer for this domain
	if err := unstructured.SetNestedField(
		certificate.Object,
		strcase.ToKebab(domain),
		"spec",
		"issuerRef",
		"name",
	); err != nil {
		return "", err
	}
	return util.UnstructuredToYaml(certificate)
}
