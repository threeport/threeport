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

// manifest is an embedded Kubernetes object changed through setters.
type manifest struct {
	object *unstructured.Unstructured
}

// newManifest decodes an embedded manifest.
func newManifest(asset []byte) (*manifest, error) {
	// decode a fresh object so setters cannot share one decode
	object, err := util.UnstructuredFromYaml(asset)
	if err != nil {
		return nil, err
	}
	return &manifest{object: object}, nil
}

// setField sets one field on the embedded object.
func (m *manifest) setField(value interface{}, path ...string) error {
	return util.SetNestedField(m.object.Object, value, path...)
}

// setSlice sets one list field on the embedded object.
func (m *manifest) setSlice(value []interface{}, path ...string) error {
	return util.SetNestedSlice(m.object.Object, value, path...)
}

// removeField drops one field from the embedded object.
func (m *manifest) removeField(path ...string) {
	util.RemoveNestedField(m.object.Object, path...)
}

// setIngressBackend sets the service and port an ingress sends traffic to.
func (m *manifest) setIngressBackend(service string, port int64) error {
	rules, _, err := unstructured.NestedSlice(m.object.Object, "spec", "rules")
	if err != nil || len(rules) == 0 {
		return fmt.Errorf("ingress has no rules: %w", err)
	}
	rule := rules[0].(map[string]interface{})
	paths := rule["http"].(map[string]interface{})["paths"].([]interface{})
	backend := paths[0].(map[string]interface{})["backend"].(map[string]interface{})
	svc := backend["service"].(map[string]interface{})
	svc["name"] = service
	svc["port"].(map[string]interface{})["number"] = port
	return m.setSlice(rules, "spec", "rules")
}

// setTransportBackend sets the service and port a transport server sends traffic to.
func (m *manifest) setTransportBackend(service string, port int64) error {
	upstreams, _, err := unstructured.NestedSlice(m.object.Object, "spec", "upstreams")
	if err != nil || len(upstreams) == 0 {
		return fmt.Errorf("transport server has no upstreams: %w", err)
	}
	upstream := upstreams[0].(map[string]interface{})
	upstream["service"] = service
	upstream["port"] = port
	if err := m.setSlice(upstreams, "spec", "upstreams"); err != nil {
		return err
	}
	return unstructured.SetNestedField(m.object.Object, fmt.Sprintf("tcp-%d", port), "spec", "listener", "name")
}

// manifestFromObject wraps an existing object so setters can change it.
func manifestFromObject(object map[string]interface{}) *manifest {
	return &manifest{object: &unstructured.Unstructured{Object: object}}
}

// yaml renders the object.
func (m *manifest) yaml() (string, error) {
	return util.UnstructuredToYaml(m.object)
}

// getIngressComponentYaml returns the nginx operator component.
// The tcp listener object waits until that operator has installed its CRDs.
func getIngressComponentYaml() (string, error) {
	component, err := newManifest(assets.IngressComponent)
	if err != nil {
		return "", err
	}
	componentYaml, err := component.yaml()
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
	component, err := newManifest(assets.ExternalDNS)
	if err != nil {
		return "", err
	}

	// set the zone this install manages
	if err := component.setField(domain, "spec", "domainName"); err != nil {
		return "", err
	}
	if provider == "" || provider == "none" {
		component.removeField("spec", "externalDNS")
		return component.yaml()
	}

	// set the provider and the ownership args
	if err := component.setField(provider, "spec", "externalDNS", "provider"); err != nil {
		return "", err
	}
	if err := component.setField(iamRoleArn, "spec", "externalDNS", "iamRoleArn"); err != nil {
		return "", err
	}
	extraArgs := []interface{}{
		"--txt-owner-id=" + txtOwnerID + "-",
		"--txt-prefix=" + txtOwnerID + "-",
	}
	if provider == "google" && gcpProject != "" {
		extraArgs = append(extraArgs, "--google-project="+gcpProject)
	}
	if err := component.setSlice(extraArgs, "spec", "externalDNS", "extraArgs"); err != nil {
		return "", err
	}

	return component.yaml()
}

// getCertificatesComponentYaml creates the cert-manager component.
func getCertificatesComponentYaml() (string, error) {
	component, err := newManifest(assets.Certificates)
	if err != nil {
		return "", err
	}
	return component.yaml()
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
		ingress, err := newManifest(assets.Ingress)
		if err != nil {
			return nil, err
		}
		if err := ingress.setField(getIngressName(gatewayDefinition, domain, *httpPort.Port), "metadata", "name"); err != nil {
			return nil, err
		}
		rules, _, err := unstructured.NestedSlice(ingress.object.Object, "spec", "rules")
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
		if err := ingress.setSlice(rules, "spec", "rules"); err != nil {
			return nil, err
		}
		if httpPort.HTTPSRedirect != nil && *httpPort.HTTPSRedirect {
			if err := ingress.setField("true", "metadata", "annotations", "nginx.org/redirect-to-https"); err != nil {
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
			if err := ingress.setSlice(tls, "spec", "tls"); err != nil {
				return nil, fmt.Errorf("failed to set tls on ingress: %w", err)
			}
		}

		rendered, err := ingress.yaml()
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
		server, err := newManifest(assets.TransportServer)
		if err != nil {
			return nil, err
		}
		if err := server.setField(fmt.Sprintf("%s-%d", *gatewayDefinition.Name, *tcpPort.Port), "metadata", "name"); err != nil {
			return nil, err
		}
		listener, _, err := unstructured.NestedMap(server.object.Object, "spec", "listener")
		if err != nil {
			return nil, fmt.Errorf("transport server has no listener: %w", err)
		}
		listener["name"] = fmt.Sprintf("tcp-%d", *tcpPort.Port)
		if err := unstructured.SetNestedMap(server.object.Object, listener, "spec", "listener"); err != nil {
			return nil, err
		}
		if err := server.setTransportBackend("placeholder", int64(*tcpPort.Port)); err != nil {
			return nil, err
		}

		rendered, err := server.yaml()
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
	issuer, err := newManifest(assets.Issuer)
	if err != nil {
		return "", err
	}
	if err := issuer.setField(strcase.ToKebab(domain), "metadata", "name"); err != nil {
		return "", err
	}
	if err := issuer.setField(adminEmail, "spec", "acme", "email"); err != nil {
		return "", err
	}
	solvers, _, err := unstructured.NestedSlice(issuer.object.Object, "spec", "acme", "solvers")
	if err != nil || len(solvers) == 0 {
		return "", fmt.Errorf("issuer has no solvers: %w", err)
	}
	selector := solvers[0].(map[string]interface{})["selector"].(map[string]interface{})
	selector["dnsZones"] = []interface{}{domain}
	if err := issuer.setSlice(solvers, "spec", "acme", "solvers"); err != nil {
		return "", err
	}
	return issuer.yaml()
}

// getCertificateYaml creates a certificate for the given domain.
func getCertificateYaml(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {
	// set the domain on the embedded certificate
	certificate, err := newManifest(assets.Certificate)
	if err != nil {
		return "", err
	}
	if err := certificate.setField(strcase.ToKebab(domain), "metadata", "name"); err != nil {
		return "", err
	}
	if err := certificate.setField(strcase.ToKebab(domain)+"-tls", "spec", "secretName"); err != nil {
		return "", err
	}
	if err := certificate.setSlice([]interface{}{domain}, "spec", "dnsNames"); err != nil {
		return "", err
	}
	if err := certificate.setField(strcase.ToKebab(domain), "spec", "issuerRef", "name"); err != nil {
		return "", err
	}
	return certificate.yaml()
}
