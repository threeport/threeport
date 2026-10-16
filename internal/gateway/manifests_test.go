package gateway

import (
	"strings"
	"testing"

	"github.com/threeport/threeport/internal/gateway/assets"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestSetIngressBackend checks the service and port the setter writes.
func TestSetIngressBackend(t *testing.T) {
	ingress, err := util.UnstructuredFromYaml(assets.Ingress)
	if err != nil {
		t.Fatal(err)
	}
	if err := setIngressBackend(ingress.Object, "web", 8080); err != nil {
		t.Fatal(err)
	}
	name, err := util.FieldAt(
		ingress.Object,
		"spec",
		"rules",
		0,
		"http",
		"paths",
		0,
		"backend",
		"service",
		"name",
	)
	if err != nil {
		t.Fatal(err)
	}
	port, err := util.FieldAt(
		ingress.Object,
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
	if err != nil {
		t.Fatal(err)
	}
	if name != "web" || port != int64(8080) {
		t.Fatalf("backend name %v port %v", name, port)
	}
}

// TestSetTransportBackend checks the service and port the setter writes.
func TestSetTransportBackend(t *testing.T) {
	server, err := util.UnstructuredFromYaml(assets.TransportServer)
	if err != nil {
		t.Fatal(err)
	}
	if err := setTransportBackend(server.Object, "db", 5432); err != nil {
		t.Fatal(err)
	}
	service, err := util.FieldAt(
		server.Object,
		"spec",
		"upstreams",
		0,
		"service",
	)
	if err != nil {
		t.Fatal(err)
	}
	port, err := util.FieldAt(
		server.Object,
		"spec",
		"upstreams",
		0,
		"port",
	)
	if err != nil {
		t.Fatal(err)
	}
	if service != "db" || port != int64(5432) {
		t.Fatalf("upstream service %v port %v", service, port)
	}
}

// TestGetExternalDnsYamlOmitsProviderNone checks that provider none drops external-dns.
func TestGetExternalDnsYamlOmitsProviderNone(t *testing.T) {
	rendered, err := getExternalDnsYaml("example.com", "none", "", "", "1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "externalDNS") {
		t.Fatalf("provider none still has externalDNS:\n%s", rendered)
	}
	if !strings.Contains(rendered, "example.com") {
		t.Fatalf("domain missing:\n%s", rendered)
	}
}

// TestGetExternalDnsYamlSetsRoute53 checks the provider, role, and owner args.
func TestGetExternalDnsYamlSetsRoute53(t *testing.T) {
	// render the external dns component and check the route53 fields
	rendered, err := getExternalDnsYaml("example.com", "route53", "arn:role", "", "7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "provider: route53") || !strings.Contains(rendered, "arn:role") || !strings.Contains(rendered, "txt-owner-id=7-") {
		t.Fatalf("route53 fields missing:\n%s", rendered)
	}
}

// TestSetSelfSignedIssuer writes selfSigned.
func TestSetSelfSignedIssuer(t *testing.T) {
	issuer, err := util.UnstructuredFromYaml(assets.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := setSelfSignedIssuer(issuer.Object); err != nil {
		t.Fatal(err)
	}
	rendered, err := util.UnstructuredToYaml(issuer)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "acme:") || !strings.Contains(rendered, "selfSigned:") {
		t.Fatalf("issuer:\n%s", rendered)
	}
}

// TestGetIssuerAndCertificate checks the domain fields the setters write.
func TestGetIssuerAndCertificate(t *testing.T) {
	issuer, err := getIssuerYaml(nil, "Example.COM", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issuer, "name: example-com") || !strings.Contains(issuer, "admin@example.com") {
		t.Fatalf("issuer:\n%s", issuer)
	}
	certificate, err := getCertificateYaml(nil, "Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(certificate, "secretName: example-com-tls") {
		t.Fatalf("certificate:\n%s", certificate)
	}
}

// TestConfirmDefinitionsReconciledRequiresDefinition covers the missing definition id.
func TestConfirmDefinitionsReconciledRequiresDefinition(t *testing.T) {
	_, err := confirmDefinitionsReconciled(nil, &v0.GatewayInstance{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestConfirmGatewayControllerDeployedAlreadySet returns when the controller id is set.
func TestConfirmGatewayControllerDeployedAlreadySet(t *testing.T) {
	// return before any API call when the controller id is already stored
	err := confirmGatewayControllerDeployed(nil, &v0.KubernetesRuntimeInstance{
		GatewayControllerInstanceID: util.Ptr(uint(1)),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// TestGatewayControllerPresentRequiresID accepts a stored controller id.
func TestGatewayControllerPresentRequiresID(t *testing.T) {
	err := gatewayControllerPresent(&v0.KubernetesRuntimeInstance{
		GatewayControllerInstanceID: util.Ptr(uint(7)),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = gatewayControllerPresent(&v0.KubernetesRuntimeInstance{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestConfirmGatewayPortsExposedRequiresController covers the missing controller id.
func TestConfirmGatewayPortsExposedRequiresController(t *testing.T) {
	err := confirmGatewayPortsExposed(nil, &v0.GatewayInstance{}, &v0.KubernetesRuntimeInstance{}, &v0.GatewayDefinition{}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestConfirmDnsControllerDeployedAlreadySet returns when the controller id is set.
func TestConfirmDnsControllerDeployedAlreadySet(t *testing.T) {
	// return before any API call when the controller id is already stored
	err := confirmDnsControllerDeployed(nil, &v0.DomainNameInstance{}, &v0.KubernetesRuntimeInstance{
		DnsControllerInstanceID: util.Ptr(uint(1)),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
