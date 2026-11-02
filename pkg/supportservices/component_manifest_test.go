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
	rendered, err := IngressComponentYaml()
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
		domain != IngressDomainName ||
		version != "5.5.4" ||
		!include ||
		kong ||
		role != "local" {
		t.Fatalf("name=%s collection=%s domain=%s version=%s nginx=%v kong=%v role=%s", name, collection, domain, version, include, kong, role)
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
	if !strings.Contains(rendered, "example.com") || !strings.Contains(rendered, "name: "+ExternalDNSName) || strings.Contains(rendered, "include: true") {
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
