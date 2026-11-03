package supportservices

import (
	_ "embed"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

//go:embed manifests/ingress-component.yaml
var ingressComponent []byte

//go:embed manifests/certificates-component.yaml
var certificatesComponent []byte

//go:embed manifests/database-component.yaml
var databaseComponent []byte

//go:embed manifests/secrets-component.yaml
var secretsComponent []byte

const (
	// IngressComponentName is the IngressComponent object this install applies.
	IngressComponentName = "ingress"

	// CertificatesComponentName is the CertificatesComponent object this install applies.
	CertificatesComponentName = "certificates"

	// SecretsComponentName is the SecretsComponent object this install applies.
	SecretsComponentName = "external-secrets"

	// DatabaseComponentName is the DatabaseComponent object this install applies.
	DatabaseComponentName = "database"

	// externalDNSName is the IngressComponent object that installs external-dns.
	externalDNSName = "externaldns"

	// ingressDomainName is the domain the nginx component uses with no public zone.
	ingressDomainName = "threeport.local"

	// externalDNSZoneType is the hosted zone type external-dns manages.
	externalDNSZoneType = "public"
)

// ingressComponentYaml returns the nginx operator component.
func ingressComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(ingressComponent)
	if err != nil {
		return "", err
	}

	// name the component and join it to the SupportServices object
	if err := unstructured.SetNestedField(
		component.Object,
		IngressComponentName,
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

	// include nginx and leave kong out
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

// ExternalDnsYaml creates an ingress component that installs external-dns.
func ExternalDnsYaml(
	domain,
	provider,
	iamRoleArn,
	gcpProject,
	txtOwnerID string,
) (string, error) {
	component, err := util.UnstructuredFromYaml(ingressComponent)
	if err != nil {
		return "", err
	}

	// name this component and leave kong out
	if err := unstructured.SetNestedField(
		component.Object,
		externalDNSName,
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

	// set the public zone, then set the provider
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

// certificatesComponentYaml returns the certificates component.
func certificatesComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(certificatesComponent)
	if err != nil {
		return "", err
	}

	// name the component
	if err := unstructured.SetNestedField(
		component.Object,
		CertificatesComponentName,
		"metadata",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the certificates component name: %w", err)
	}
	return util.UnstructuredToYaml(component)
}

// databaseComponentYaml returns the database component.
func databaseComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(databaseComponent)
	if err != nil {
		return "", err
	}

	// name the component
	if err := unstructured.SetNestedField(
		component.Object,
		DatabaseComponentName,
		"metadata",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the database component name: %w", err)
	}
	return util.UnstructuredToYaml(component)
}

// secretsComponentYaml returns the external secrets component.
func secretsComponentYaml() (string, error) {
	// decode the embedded component
	component, err := util.UnstructuredFromYaml(secretsComponent)
	if err != nil {
		return "", err
	}

	// name the component
	if err := unstructured.SetNestedField(
		component.Object,
		SecretsComponentName,
		"metadata",
		"name",
	); err != nil {
		return "", fmt.Errorf("failed to set the secrets component name: %w", err)
	}
	return util.UnstructuredToYaml(component)
}
