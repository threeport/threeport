package gateway

import (
	"strings"
	"testing"

	"github.com/threeport/threeport/internal/gateway/manifests"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestSetIngressBackend checks the service and port the setter writes.
func TestSetIngressBackend(t *testing.T) {
	ingress, err := util.UnstructuredFromYaml(manifests.Ingress)
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
	server, err := util.UnstructuredFromYaml(manifests.TransportServer)
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

// TestIngressRule reads the first ingress rule, its http object, and its paths.
func TestIngressRule(t *testing.T) {
	object := map[string]interface{}{
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{"path": "/"},
						},
					},
				},
			},
		},
	}

	// read the first rule
	rule, http, paths, err := ingressRule(object)
	if err != nil || rule == nil || http == nil || len(paths) != 1 {
		t.Fatalf("ingressRule() = (%v, %v, %v, %v)", rule, http, paths, err)
	}
}

// TestSetSelfSignedIssuer writes selfSigned.
func TestSetSelfSignedIssuer(t *testing.T) {
	issuer, err := util.UnstructuredFromYaml(manifests.Issuer)
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

// TestGatewayComponentPresentRequiresID accepts a stored component id.
func TestGatewayComponentPresentRequiresID(t *testing.T) {
	err := gatewayComponentPresent(&v0.KubernetesRuntimeInstance{
		IngressComponentInstanceID: util.Ptr(uint(7)),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = gatewayComponentPresent(&v0.KubernetesRuntimeInstance{})
	if err == nil {
		t.Fatal("expected an error")
	}
}
