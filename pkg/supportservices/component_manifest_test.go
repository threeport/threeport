package supportservices

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestIngressComponentYaml checks the fields this install sets on the sample.
func TestIngressComponentYaml(t *testing.T) {
	// the sample stays free of threeport values
	if strings.Contains(string(ingressComponent), "threeport") {
		t.Fatal("ingress component sample contains a threeport value")
	}

	// render the component and read the fields this install sets
	rendered, err := ingressComponentYaml()
	if err != nil {
		t.Fatal(err)
	}
	component, err := util.UnstructuredFromYaml([]byte(rendered))
	if err != nil {
		t.Fatal(err)
	}
	name, _, _ := unstructured.NestedString(component.Object, "metadata", "name")
	collection, _, _ := unstructured.NestedString(component.Object, "spec", "collection", "name")
	domain, _, _ := unstructured.NestedString(component.Object, "spec", "domainName")
	version, _, _ := unstructured.NestedString(component.Object, "spec", "nginx", "version")
	include, _, _ := unstructured.NestedBool(component.Object, "spec", "nginx", "include")
	kong, _, _ := unstructured.NestedBool(component.Object, "spec", "kong", "include")
	role, _, _ := unstructured.NestedString(component.Object, "spec", "externalDNS", "iamRoleArn")
	if name != IngressComponentName ||
		collection != "threeport-support-services" ||
		domain != ingressDomainName ||
		version != "5.5.4" ||
		!include ||
		kong ||
		role != "local" {
		t.Fatalf("name=%s collection=%s domain=%s version=%s nginx=%v kong=%v role=%s", name, collection, domain, version, include, kong, role)
	}
}

// TestDatabaseComponentYaml checks the fields this install sets on the sample.
func TestDatabaseComponentYaml(t *testing.T) {
	// the sample stays free of threeport values
	if strings.Contains(string(databaseComponent), "threeport") {
		t.Fatal("database component sample contains a threeport value")
	}

	// render the component and read the operator fields
	rendered, err := databaseComponentYaml()
	if err != nil {
		t.Fatal(err)
	}
	component, err := util.UnstructuredFromYaml([]byte(rendered))
	if err != nil {
		t.Fatal(err)
	}
	name, _, _ := unstructured.NestedString(component.Object, "metadata", "name")
	collection, _, _ := unstructured.NestedString(component.Object, "spec", "collection", "name")
	provider, _, _ := unstructured.NestedString(component.Object, "spec", "provider")
	namespace, _, _ := unstructured.NestedString(component.Object, "spec", "namespace")
	image, _, _ := unstructured.NestedString(component.Object, "spec", "cloudNativePg", "image")
	version, _, _ := unstructured.NestedString(component.Object, "spec", "cloudNativePg", "version")
	replicas, _, _ := unstructured.NestedInt64(component.Object, "spec", "cloudNativePg", "replicas")
	if component.GetKind() != "DatabaseComponent" ||
		name != DatabaseComponentName ||
		collection != "threeport-support-services" ||
		provider != "cloudnative-pg" ||
		namespace != "nukleros-database-system" ||
		image != "ghcr.io/cloudnative-pg/cloudnative-pg" ||
		version != "1.30.1" ||
		replicas != 2 {
		t.Fatalf("kind=%s name=%s collection=%s provider=%s namespace=%s image=%s version=%s replicas=%d", component.GetKind(), name, collection, provider, namespace, image, version, replicas)
	}
}

// TestCertificatesComponentYaml checks the fields this install sets on the sample.
func TestCertificatesComponentYaml(t *testing.T) {
	// the sample stays free of threeport values
	if strings.Contains(string(certificatesComponent), "threeport") {
		t.Fatal("certificates component sample contains a threeport value")
	}

	// render the component and read the operator fields
	rendered, err := certificatesComponentYaml()
	if err != nil {
		t.Fatal(err)
	}
	component, err := util.UnstructuredFromYaml([]byte(rendered))
	if err != nil {
		t.Fatal(err)
	}
	name, _, _ := unstructured.NestedString(component.Object, "metadata", "name")
	collection, _, _ := unstructured.NestedString(component.Object, "spec", "collection", "name")
	namespace, _, _ := unstructured.NestedString(component.Object, "spec", "namespace")
	version, _, _ := unstructured.NestedString(component.Object, "spec", "certManager", "version")
	email, _, _ := unstructured.NestedString(component.Object, "spec", "certManager", "contactEmail")
	if component.GetKind() != "CertificatesComponent" ||
		name != CertificatesComponentName ||
		collection != "threeport-support-services" ||
		namespace != "nukleros-certs-system" ||
		version != "v1.9.1" ||
		email != "admin@nukleros.io" ||
		component.GetNamespace() != "" {
		t.Fatalf("kind=%s name=%s collection=%s namespace=%s version=%s email=%s objectNamespace=%s", component.GetKind(), name, collection, namespace, version, email, component.GetNamespace())
	}
}

// TestSecretsComponentYaml checks the fields this install sets on the sample.
func TestSecretsComponentYaml(t *testing.T) {
	// the sample stays free of threeport values
	if strings.Contains(string(secretsComponent), "threeport") {
		t.Fatal("secrets component sample contains a threeport value")
	}

	// render the component and read the operator fields
	rendered, err := secretsComponentYaml()
	if err != nil {
		t.Fatal(err)
	}
	component, err := util.UnstructuredFromYaml([]byte(rendered))
	if err != nil {
		t.Fatal(err)
	}
	name, _, _ := unstructured.NestedString(component.Object, "metadata", "name")
	collection, _, _ := unstructured.NestedString(component.Object, "spec", "collection", "name")
	namespace, _, _ := unstructured.NestedString(component.Object, "spec", "namespace")
	image, _, _ := unstructured.NestedString(component.Object, "spec", "externalSecrets", "image")
	version, _, _ := unstructured.NestedString(component.Object, "spec", "externalSecrets", "version")
	_, replicasFound, _ := unstructured.NestedFieldNoCopy(component.Object, "spec", "externalSecrets", "replicas")
	if component.GetKind() != "SecretsComponent" ||
		name != SecretsComponentName ||
		collection != "threeport-support-services" ||
		namespace != "nukleros-secrets-system" ||
		image != "ghcr.io/external-secrets/external-secrets" ||
		version != "v0.9.11" ||
		replicasFound ||
		component.GetNamespace() != "" {
		t.Fatalf("kind=%s name=%s collection=%s namespace=%s image=%s version=%s replicas=%v objectNamespace=%s", component.GetKind(), name, collection, namespace, image, version, replicasFound, component.GetNamespace())
	}
}

// TestExternalDnsYamlOmitsProviderNone checks that provider none drops external-dns.
// The component is named externaldns and kong stays out.
func TestExternalDnsYamlOmitsProviderNone(t *testing.T) {
	// render with no provider
	rendered, err := ExternalDnsYaml("example.com", "none", "", "", "1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "externalDNS") {
		t.Fatalf("provider none still has externalDNS:\n%s", rendered)
	}
	if !strings.Contains(rendered, "example.com") || !strings.Contains(rendered, "name: "+externalDNSName) || strings.Contains(rendered, "include: true") {
		t.Fatalf("component fields:\n%s", rendered)
	}
}

// TestExternalDnsYamlSetsRoute53 checks the provider, role, and owner args.
func TestExternalDnsYamlSetsRoute53(t *testing.T) {
	// render the external dns component and check the route53 fields
	rendered, err := ExternalDnsYaml("example.com", "route53", "arn:role", "", "7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "provider: route53") || !strings.Contains(rendered, "arn:role") || !strings.Contains(rendered, "txt-owner-id=7-") {
		t.Fatalf("route53 fields missing:\n%s", rendered)
	}
}
