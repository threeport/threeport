package gateway

import (
	"fmt"
	"sort"
	"strings"

	"github.com/iancoleman/strcase"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/gateway/assets"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	// ingressComponentName is the IngressComponent object this install applies.
	ingressComponentName = "ingress"

	// externalDNSComponentName is the IngressComponent object that installs external-dns.
	externalDNSComponentName = "externaldns"

	// ingressDomainName is the domain the nginx component uses with no public zone.
	ingressDomainName = "threeport.local"

	// nginxIngressVersion is the nginx ingress controller tag this install pins.
	nginxIngressVersion = "5.7.0"

	// certificatesComponentName is the CertificatesComponent object this install applies.
	certificatesComponentName = "certificates"

	// externalDNSImage is the external-dns image repository this install pins.
	externalDNSImage = "registry.k8s.io/external-dns/external-dns"

	// externalDNSVersion is the external-dns tag this install pins.
	externalDNSVersion = "v0.13.5"

	// externalDNSZoneType is the hosted zone type external-dns manages.
	externalDNSZoneType = "public"
)

// ingressRule returns the first rule, its http object, and that object's paths.
// A missing paths list returns an empty slice.
func ingressRule(object map[string]interface{}) (
	map[string]interface{},
	map[string]interface{},
	[]interface{},
	error,
) {
	// read the first rule
	ruleValue, err := util.FieldAt(
		object,
		"spec",
		"rules",
		0,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	rule, ok := ruleValue.(map[string]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("failed to read field [spec rules 0]: expected a map")
	}

	// read its http object
	httpValue, err := util.FieldAt(
		object,
		"spec",
		"rules",
		0,
		"http",
	)
	if err != nil {
		return nil, nil, nil, err
	}
	http, ok := httpValue.(map[string]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("failed to read field [spec rules 0 http]: expected a map")
	}

	// a missing paths list is an empty slice
	pathsValue, err := util.FieldAt(
		object,
		"spec",
		"rules",
		0,
		"http",
		"paths",
	)
	if err != nil {
		return rule, http, []interface{}{}, nil
	}
	paths, ok := pathsValue.([]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("failed to read field [spec rules 0 http paths]: expected a list")
	}
	return rule, http, paths, nil
}

// setIngressBackend sets the service and port an ingress sends traffic to.
func setIngressBackend(object map[string]interface{}, service string, port int64) error {
	// read the path list
	_, _, paths, err := ingressRule(object)
	if err != nil || len(paths) == 0 {
		return fmt.Errorf("ingress has no rules")
	}

	// set the service name and port on every path
	for i := range paths {
		if err := util.SetIndexedField(
			object,
			service,
			"spec",
			"rules",
			0,
			"http",
			"paths",
			i,
			"backend",
			"service",
			"name",
		); err != nil {
			return fmt.Errorf("ingress has no rules: %w", err)
		}
		if err := util.SetIndexedField(
			object,
			port,
			"spec",
			"rules",
			0,
			"http",
			"paths",
			i,
			"backend",
			"service",
			"port",
			"number",
		); err != nil {
			return fmt.Errorf("ingress has no rules: %w", err)
		}
	}

	return nil
}

// setIngressPaths writes one path entry for each distinct path.
func setIngressPaths(object map[string]interface{}, paths []string) error {
	// read the first rule
	rule, http, _, err := ingressRule(object)
	if err != nil {
		return fmt.Errorf("ingress has no rules")
	}
	rules, found, err := unstructured.NestedSlice(object, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return fmt.Errorf("ingress has no rules")
	}

	// write one path entry for each distinct path
	entries := make([]interface{}, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, map[string]interface{}{
			"path":     path,
			"pathType": "Prefix",
			"backend": map[string]interface{}{
				"service": map[string]interface{}{
					"name": "placeholder",
					"port": map[string]interface{}{
						"number": int64(80),
					},
				},
			},
		})
	}
	http["paths"] = entries
	rule["http"] = http
	rules[0] = rule

	return unstructured.SetNestedSlice(object, rules, "spec", "rules")
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
func getIngressComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(assets.IngressComponent)
	if err != nil {
		return "", err
	}

	// name the component and join it to the SupportServices object
	if err := unstructured.SetNestedField(
		component.Object,
		ingressComponentName,
		"metadata",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the ingress component name: %w", err)
	}
	if err := unstructured.SetNestedField(
		component.Object,
		installer.SupportServicesName,
		"spec",
		"collection",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the support services name: %w", err)
	}
	if err := unstructured.SetNestedField(
		component.Object,
		ingressDomainName,
		"spec",
		"domainName",
	); err != nil {
		return "", fmt.Errorf("failed to set the ingress domain: %w", err)
	}

	// install nginx at the pinned version and leave kong out
	if err := unstructured.SetNestedField(
		component.Object,
		true,
		"spec",
		"nginx",
		"include",
	); err != nil {
		return "", fmt.Errorf("failed to set nginx include: %w", err)
	}
	if err := unstructured.SetNestedField(
		component.Object,
		nginxIngressVersion,
		"spec",
		"nginx",
		"version",
	); err != nil {
		return "", fmt.Errorf("failed to set the nginx version: %w", err)
	}
	if err := unstructured.SetNestedField(
		component.Object,
		false,
		"spec",
		"kong",
		"include",
	); err != nil {
		return "", fmt.Errorf("failed to set kong include: %w", err)
	}

	// an empty iam role is omitted, and the custom resource requires the field
	if err := unstructured.SetNestedField(
		component.Object,
		installer.SupportServicesCloudLocal,
		"spec",
		"externalDNS",
		"iamRoleArn",
	); err != nil {
		return "", fmt.Errorf("failed to set the external dns role: %w", err)
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
	component, err := util.UnstructuredFromYaml(assets.IngressComponent)
	if err != nil {
		return "", err
	}

	// name this component and leave kong out
	if err := unstructured.SetNestedField(
		component.Object,
		externalDNSComponentName,
		"metadata",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the external dns component name: %w", err)
	}
	if err := unstructured.SetNestedField(
		component.Object,
		false,
		"spec",
		"kong",
		"include",
	); err != nil {
		return "", fmt.Errorf("failed to set kong include: %w", err)
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

	// pin the image and the public zone, then set the provider
	if err := unstructured.SetNestedField(
		component.Object,
		externalDNSZoneType,
		"spec",
		"externalDNS",
		"zoneType",
	); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(
		component.Object,
		externalDNSImage,
		"spec",
		"externalDNS",
		"image",
	); err != nil {
		return "", err
	}
	if err := unstructured.SetNestedField(
		component.Object,
		externalDNSVersion,
		"spec",
		"externalDNS",
		"version",
	); err != nil {
		return "", err
	}
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
func getCertificatesComponentYaml(roleArn string) (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(assets.Certificates)
	if err != nil {
		return "", err
	}

	// name the component
	if err := unstructured.SetNestedField(
		component.Object,
		certificatesComponentName,
		"metadata",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the certificates component name: %w", err)
	}
	rendered, err := util.UnstructuredToYaml(component)
	if err != nil {
		return "", err
	}
	if roleArn == "" {
		return rendered, nil
	}

	// emit the cert-manager account before the component
	account, err := installer.IrsaServiceAccountYaml(
		installer.DNS01ChallengeServiceAccountName,
		installer.DNS01ChallengeServiceAccountNamepace,
		roleArn,
	)
	if err != nil {
		return "", err
	}
	return account + "\n---\n" + rendered, nil
}

// getCleanedDomain returns a cleaned domain.
func getCleanedDomain(domain string) string {
	// strip a scheme prefix
	cleanedDomain := strings.TrimPrefix(domain, "http://")
	cleanedDomain = strings.TrimPrefix(cleanedDomain, "https://")
	return cleanedDomain
}

// getIngressesYaml creates one ingress for the host.
// Nginx rejects a second ingress that repeats the host.
func getIngressesYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition, domain string) ([]string, error) {
	// clean the domain and load the http ports
	domain = getCleanedDomain(domain)
	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}
	if len(*gatewayHttpPorts) == 0 {
		return nil, nil
	}

	// reject a port nginx does not publish
	var paths []string
	seen := map[string]struct{}{}
	tlsEnabled := false
	redirect := false
	for _, httpPort := range *gatewayHttpPorts {
		enabled := httpPort.TLSEnabled != nil && *httpPort.TLSEnabled
		if err := supportedHTTPPort(*httpPort.Port, enabled); err != nil {
			return nil, err
		}
		if enabled {
			tlsEnabled = true
		}
		if httpPort.HTTPSRedirect != nil && *httpPort.HTTPSRedirect {
			redirect = true
		}
		if httpPort.Path == nil || *httpPort.Path == "" {
			continue
		}
		if _, ok := seen[*httpPort.Path]; ok {
			continue
		}
		seen[*httpPort.Path] = struct{}{}
		paths = append(paths, *httpPort.Path)
	}

	// copy the embedded ingress and name it for the host
	ingress, err := util.UnstructuredFromYaml(assets.Ingress)
	if err != nil {
		return nil, err
	}
	if err := unstructured.SetNestedField(
		ingress.Object,
		getIngressName(gatewayDefinition, domain),
		"metadata",
		"name",
	); err != nil {
		return nil, err
	}
	// drop the host when the domain is empty
	if domain == "" {
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
		domain,
		"spec",
		"rules",
		0,
		"host",
	); err != nil {
		return nil, fmt.Errorf("ingress has no rules: %w", err)
	}
	// set each distinct path on the one host rule
	if len(paths) > 0 {
		if err := setIngressPaths(ingress.Object, paths); err != nil {
			return nil, err
		}
	}
	// redirect http when a port asks for it
	if redirect {
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
	// terminate tls at the ingress when a port enables it
	if tlsEnabled {
		tls := []interface{}{
			map[string]interface{}{
				"hosts":      []interface{}{domain},
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
		// nginx redirects http once tls is set, unless this annotation is false
		if !redirect {
			if err := unstructured.SetNestedField(
				ingress.Object,
				"false",
				"metadata",
				"annotations",
				"nginx.org/ssl-redirect",
			); err != nil {
				return nil, fmt.Errorf("failed to set ssl redirect on ingress: %w", err)
			}
		}
	}

	// render the ingress
	rendered, err := util.UnstructuredToYaml(ingress)
	if err != nil {
		return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
	}
	return []string{rendered}, nil
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

// getIngressName returns the name of the ingress for a host.
func getIngressName(gatewayDefinition *v0.GatewayDefinition, domain string) string {
	// use the domain, or the gateway name when the domain is empty
	domain = getCleanedDomain(domain)
	if domain == "" {
		return *gatewayDefinition.Name
	}
	return strcase.ToKebab(domain)
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

// supportedHTTPPort reports whether nginx publishes the port.
func supportedHTTPPort(port int, tls bool) error {
	if port == 80 || (port == 443 && tls) {
		return nil
	}
	return fmt.Errorf("http port %d is not served; use 80, or 443 with tls", port)
}

// orderedPorts returns the ports sorted and without duplicates.
func orderedPorts(ports []int) []int {
	// sort, then drop a port that matches the one before it
	ordered := append([]int(nil), ports...)
	sort.Ints(ordered)
	unique := make([]int, 0, len(ordered))
	var previous int
	for i, port := range ordered {
		if i > 0 && port == previous {
			continue
		}
		unique = append(unique, port)
		previous = port
	}
	return unique
}

// tcpListeners returns one listener per port, in port order.
func tcpListeners(ports []int) []interface{} {
	// build one listener for each port
	ordered := orderedPorts(ports)
	listeners := make([]interface{}, 0, len(ordered))
	for _, port := range ordered {
		listeners = append(listeners, map[string]interface{}{
			"name":     fmt.Sprintf("tcp-%d", port),
			"port":     int64(port),
			"protocol": "TCP",
		})
	}
	return listeners
}

// tcpServicePorts returns one service port per listener.
func tcpServicePorts(ports []int) []interface{} {
	// build one service port for each listener
	ordered := orderedPorts(ports)
	servicePorts := make([]interface{}, 0, len(ordered))
	for _, port := range ordered {
		servicePorts = append(servicePorts, map[string]interface{}{
			"name":       fmt.Sprintf("tcp-%d", port),
			"port":       int64(port),
			"targetPort": int64(port),
			"protocol":   "TCP",
		})
	}
	return servicePorts
}

// tcpContainerPorts returns container ports for the shared TCP controller.
func tcpContainerPorts(ports []int) []interface{} {
	// name each port and set containerPort
	ordered := orderedPorts(ports)
	containerPorts := make([]interface{}, 0, len(ordered))
	for _, port := range ordered {
		containerPorts = append(containerPorts, map[string]interface{}{
			"name":          fmt.Sprintf("tcp-%d", port),
			"containerPort": int64(port),
			"protocol":      "TCP",
		})
	}

	return containerPorts
}

// setTcpControllerPorts writes the TCP ports onto the controller container.
func setTcpControllerPorts(object *unstructured.Unstructured, ports []int) error {
	containers, _, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
	if err != nil || len(containers) == 0 {
		return fmt.Errorf("tcp controller has no containers")
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("tcp controller container is not an object")
	}
	container["ports"] = tcpContainerPorts(ports)
	containers[0] = container
	return unstructured.SetNestedSlice(object.Object, containers, "spec", "template", "spec", "containers")
}

// tcpControllerYaml returns the TCP controller deployment.
func tcpControllerYaml(ports []int) ([]byte, error) {
	// load the embedded deployment and set its container ports
	object, err := util.UnstructuredFromYaml(assets.NginxTcpController)
	if err != nil {
		return nil, err
	}
	if err := setTcpControllerPorts(object, ports); err != nil {
		return nil, err
	}
	rendered, err := util.UnstructuredToYaml(object)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}
