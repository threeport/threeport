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

// setIndexedField writes one field through maps and list indexes.
// unstructured.SetNestedField only accepts map keys.
func setIndexedField(object map[string]interface{}, value interface{}, path ...interface{}) error {
	// walk each path entry, then write the last one
	if len(path) == 0 {
		return fmt.Errorf("failed to set field: empty path")
	}
	var current interface{} = object
	for i, entry := range path {
		last := i == len(path)-1
		switch key := entry.(type) {
		case string:
			fields, ok := current.(map[string]interface{})
			if !ok {
				return fmt.Errorf("failed to set field %v: expected a map", path)
			}
			if last {
				fields[key] = value
				return nil
			}
			next, ok := fields[key]
			if !ok {
				return fmt.Errorf("failed to set field %v: missing %s", path, key)
			}
			current = next
		case int:
			list, ok := current.([]interface{})
			if !ok {
				return fmt.Errorf("failed to set field %v: expected a list", path)
			}
			if key < 0 || key >= len(list) {
				return fmt.Errorf("failed to set field %v: index %d out of range", path, key)
			}
			if last {
				list[key] = value
				return nil
			}
			current = list[key]
		default:
			return fmt.Errorf("failed to set field %v: path entry is not a string or an int", path)
		}
	}
	return nil
}

// setIngressBackend sets the service and port an ingress sends traffic to.
func setIngressBackend(object map[string]interface{}, service string, port int64) error {
	// set the service name and port on the first path
	if err := setIndexedField(object, service, "spec", "rules", 0, "http", "paths", 0, "backend", "service", "name"); err != nil {
		return fmt.Errorf("ingress has no rules: %w", err)
	}
	return setIndexedField(object, port, "spec", "rules", 0, "http", "paths", 0, "backend", "service", "port", "number")
}

// setTransportBackend sets the service and port a transport server sends traffic to.
func setTransportBackend(object map[string]interface{}, service string, port int64) error {
	// set the upstream, then name the listener for this port
	if err := setIndexedField(object, service, "spec", "upstreams", 0, "service"); err != nil {
		return fmt.Errorf("transport server has no upstreams: %w", err)
	}
	if err := setIndexedField(object, port, "spec", "upstreams", 0, "port"); err != nil {
		return err
	}
	return unstructured.SetNestedField(object, fmt.Sprintf("tcp-%d", port), "spec", "listener", "name")
}

// getIngressComponentYaml returns the nginx operator component.
// The tcp listener object waits until that operator has installed its CRDs.
func getIngressComponentYaml() (string, error) {
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
	if err := unstructured.SetNestedField(component.Object, domain, "spec", "domainName"); err != nil {
		return "", err
	}
	if provider == "" || provider == "none" {
		unstructured.RemoveNestedField(component.Object, "spec", "externalDNS")
		return util.UnstructuredToYaml(component)
	}

	// set the provider and the ownership args
	if err := unstructured.SetNestedField(component.Object, provider, "spec", "externalDNS", "provider"); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(component.Object, iamRoleArn, "spec", "externalDNS", "iamRoleArn"); err != nil {
		return "", err
	}
	extraArgs := []interface{}{
		"--txt-owner-id=" + txtOwnerID + "-",
		"--txt-prefix=" + txtOwnerID + "-",
	}
	if provider == "google" && gcpProject != "" {
		extraArgs = append(extraArgs, "--google-project="+gcpProject)
	}
	if err := unstructured.SetNestedSlice(component.Object, extraArgs, "spec", "externalDNS", "extraArgs"); err != nil {
		return "", err
	}

	return util.UnstructuredToYaml(component)
}

// getCertificatesComponentYaml creates the cert-manager component.
func getCertificatesComponentYaml() (string, error) {
	component, err := util.UnstructuredFromYaml(assets.Certificates)
	if err != nil {
		return "", err
	}
	return util.UnstructuredToYaml(component)
}

// getCleanedDomain returns a cleaned domain.
func getCleanedDomain(domain string) string {
	cleanedDomain := strings.TrimPrefix(domain, "http://")
	cleanedDomain = strings.TrimPrefix(cleanedDomain, "https://")
	return cleanedDomain
}

// getIngressesYaml creates an ingress for each HTTP port.
func getIngressesYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition, domain string) ([]string, error) {
	var manifests []string

	domain = getCleanedDomain(domain)
	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}
	for _, httpPort := range *gatewayHttpPorts {
		host := domain

		// copy the embedded ingress and set this port
		ingress, err := util.UnstructuredFromYaml(assets.Ingress)
		if err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(ingress.Object, getIngressName(gatewayDefinition, domain, *httpPort.Port), "metadata", "name"); err != nil {
			return nil, err
		}
		rules, _, err := unstructured.NestedSlice(ingress.Object, "spec", "rules")
		if err != nil || len(rules) == 0 {
			return nil, fmt.Errorf("ingress has no rules: %w", err)
		}
		rule := rules[0].(map[string]interface{})
		if host == "" {
			delete(rule, "host")
		} else {
			rule["host"] = host
		}
		paths := rule["http"].(map[string]interface{})["paths"].([]interface{})
		paths[0].(map[string]interface{})["path"] = *httpPort.Path
		if err := unstructured.SetNestedSlice(ingress.Object, rules, "spec", "rules"); err != nil {
			return nil, err
		}
		if httpPort.HTTPSRedirect != nil && *httpPort.HTTPSRedirect {
			if err := unstructured.SetNestedField(ingress.Object, "true", "metadata", "annotations", "nginx.org/redirect-to-https"); err != nil {
				return nil, fmt.Errorf("failed to set https redirect on ingress: %w", err)
			}
		}
		if httpPort.TLSEnabled != nil && *httpPort.TLSEnabled {
			tls := []interface{}{
				map[string]interface{}{
					"hosts":      []interface{}{host},
					"secretName": strcase.ToKebab(domain) + "-tls",
				},
			}
			if err := unstructured.SetNestedSlice(ingress.Object, tls, "spec", "tls"); err != nil {
				return nil, fmt.Errorf("failed to set tls on ingress: %w", err)
			}
		}

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

	gatewayTcpPorts, err := client.GetGatewayTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway tcp ports: %w", err)
	}
	for _, tcpPort := range *gatewayTcpPorts {
		server, err := util.UnstructuredFromYaml(assets.TransportServer)
		if err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(server.Object, fmt.Sprintf("%s-%d", *gatewayDefinition.Name, *tcpPort.Port), "metadata", "name"); err != nil {
			return nil, err
		}
		listener, _, err := unstructured.NestedMap(server.Object, "spec", "listener")
		if err != nil {
			return nil, fmt.Errorf("transport server has no listener: %w", err)
		}
		listener["name"] = fmt.Sprintf("tcp-%d", *tcpPort.Port)
		if err := unstructured.SetNestedMap(server.Object, listener, "spec", "listener"); err != nil {
			return nil, err
		}
		if err := setTransportBackend(server.Object, "placeholder", int64(*tcpPort.Port)); err != nil {
			return nil, err
		}

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
	domain = getCleanedDomain(domain)
	if domain == "" {
		return fmt.Sprintf("%s-%d", *gatewayDefinition.Name, port)
	}
	return fmt.Sprintf("%s-%d", strcase.ToKebab(domain), port)
}

// setSelfSignedIssuer writes a self-signed cert-manager issuer.
func setSelfSignedIssuer(object map[string]interface{}) error {
	// write the self-signed spec
	unstructured.RemoveNestedField(object, "spec", "acme")
	return unstructured.SetNestedField(object, map[string]interface{}{}, "spec", "selfSigned")
}

// getIssuerYaml creates an issuer for the given domain.
func getIssuerYaml(gatewayDefinition *v0.GatewayDefinition, domain, adminEmail string) (string, error) {
	issuer, err := util.UnstructuredFromYaml(assets.Issuer)
	if err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(issuer.Object, strcase.ToKebab(domain), "metadata", "name"); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(issuer.Object, adminEmail, "spec", "acme", "email"); err != nil {
		return "", err
	}
	solvers, _, err := unstructured.NestedSlice(issuer.Object, "spec", "acme", "solvers")
	if err != nil || len(solvers) == 0 {
		return "", fmt.Errorf("issuer has no solvers: %w", err)
	}
	selector := solvers[0].(map[string]interface{})["selector"].(map[string]interface{})
	selector["dnsZones"] = []interface{}{domain}
	if err := unstructured.SetNestedSlice(issuer.Object, solvers, "spec", "acme", "solvers"); err != nil {
		return "", err
	}
	return util.UnstructuredToYaml(issuer)
}

// getCertificateYaml creates a certificate for the given domain.
func getCertificateYaml(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {
	// set the domain on the embedded certificate
	certificate, err := util.UnstructuredFromYaml(assets.Certificate)
	if err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(certificate.Object, strcase.ToKebab(domain), "metadata", "name"); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(certificate.Object, strcase.ToKebab(domain)+"-tls", "spec", "secretName"); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedSlice(certificate.Object, []interface{}{domain}, "spec", "dnsNames"); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(certificate.Object, strcase.ToKebab(domain), "spec", "issuerRef", "name"); err != nil {
		return "", err
	}
	return util.UnstructuredToYaml(certificate)
}
