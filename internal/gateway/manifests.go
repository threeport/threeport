package gateway

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func createSupportServicesCollection() (string, error) {
	var supportServicesCollection = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "orchestration.support-services.nukleros.io/v1alpha1",
			"kind":       "SupportServices",
			"metadata": map[string]interface{}{
				"name": "supportservices-sample",
			},
			"spec": map[string]interface{}{
				"tier":                     "development",
				"defaultIngressController": "kong",
			},
		},
	}

	return unstructuredToYAMLString(supportServicesCollection)
}

func createGlooEdge() (string, error) {

	var glooEdge = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.support-services.nukleros.io/v1alpha1",
			"kind":       "GlooEdge",
			"metadata": map[string]interface{}{
				"name": "glooedge",
			},
			"spec": map[string]interface{}{
				"namespace": "nukleros-gateway-system",
				"ports":     []interface{}{},
			},
		},
	}

	return unstructuredToYAMLString(glooEdge)
}

func createExternalDns(
	provider,
	iamRoleArn,
	glooEdgeNamespace,
	kubernetesRuntimeInstanceID string,
) (string, error) {

	zoneType := "public"
	extraArgs := []string{
		"--source=gloo-proxy",
		"--gloo-namespace=" + glooEdgeNamespace,
		"--txt-owner-id=" + kubernetesRuntimeInstanceID + "-",
		"--txt-prefix=" + kubernetesRuntimeInstanceID + "-",
	}

	var externalDns = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.support-services.nukleros.io/v1alpha1",
			"kind":       "ExternalDNS",
			"metadata": map[string]interface{}{
				"name": "externaldns-sample",
			},
			"spec": map[string]interface{}{
				"namespace":          "nukleros-gateway-system",
				"zoneType":           zoneType,
				"domainName":         "qleet.net",
				"image":              "registry.k8s.io/external-dns/external-dns",
				"version":            "v0.13.5",
				"provider":           provider,
				"serviceAccountName": "external-dns",
				"iamRoleArn":         iamRoleArn,
				"extraArgs":          extraArgs,
			},
		},
	}

	return unstructuredToYAMLString(externalDns)
}

func createGlooEdgePort(name string, port int64, ssl bool) map[string]interface{} {

	var portObject = map[string]interface{}{
		"name": name,
		"port": port,
		"ssl":  ssl,
	}

	return portObject
}

func createCertManager(iamRoleArn string) (string, error) {

	var certManager = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "certificates.support-services.nukleros.io/v1alpha1",
			"kind":       "CertManager",
			"metadata": map[string]interface{}{
				"name": "certmanager-sample",
			},
			"spec": map[string]interface{}{
				"namespace": "nukleros-certs-system",
				"cainjector": map[string]interface{}{
					"replicas": 1,
					"image":    "quay.io/jetstack/cert-manager-cainjector",
				},
				"version": "v1.9.1",
				"controller": map[string]interface{}{
					"replicas": 1,
					"image":    "quay.io/jetstack/cert-manager-controller",
				},
				"webhook": map[string]interface{}{
					"replicas": 1,
					"image":    "quay.io/jetstack/cert-manager-webhook",
				},
				"contactEmail": "admin@nukleros.io",
				"iamRoleArn":   iamRoleArn,
			},
		},
	}

	return unstructuredToYAMLString(certManager)
}

func createVirtualService(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {

	var domainList []interface{}
	var virtualServiceName string
	if domain == "" {
		domainList = []interface{}{"*"}
		virtualServiceName = *gatewayDefinition.Name
	} else {
		domainWithoutSchema := strings.SplitN(domain, ".", 2)[1]
		domainList = []interface{}{domain, domainWithoutSchema}
		virtualServiceName = strcase.ToKebab(domainWithoutSchema)
	}

	sslConfig := map[string]interface{}{}
	if *gatewayDefinition.TLSEnabled {
		parts := strings.SplitN(domain, ".", 2)
		dnsZone := parts[1]
		kebabDnsZone := strcase.ToKebab(dnsZone)
		sslConfig = map[string]interface{}{
			"secretRef": map[string]interface{}{
				"name":      kebabDnsZone + "-tls",
				"namespace": "default",
			},
		}
	}

	var virtualService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.solo.io/v1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      virtualServiceName,
				"namespace": "gloo-system",
			},
			"spec": map[string]interface{}{
				"virtualHost": map[string]interface{}{
					"domains": domainList,
					"routes": []interface{}{
						map[string]interface{}{
							"matchers": []interface{}{
								map[string]interface{}{
									"prefix": *gatewayDefinition.Path,
								},
							},
							"routeAction": map[string]interface{}{
								"single": map[string]interface{}{
									"upstream": map[string]interface{}{
										"name":      "my-upstream",
										"namespace": "gloo-system",
									},
								},
							},
						},
					},
				},
				"sslConfig": sslConfig,
			},
		},
	}

	return unstructuredToYAMLString(virtualService)
}

func createIssuer(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {

	parts := strings.SplitN(domain, ".", 2)
	dnsZone := parts[1]
	kebabDnsZone := strcase.ToKebab(dnsZone)

	var issuer = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name": kebabDnsZone,
			},
			"spec": map[string]interface{}{
				"acme": map[string]interface{}{
					"email":  "randy@qleet.io",
					"server": "https://acme-v02.api.letsencrypt.org/directory",
					"privateKeySecretRef": map[string]interface{}{
						"name": "letsencrypt-prod-private-key",
					},
					"solvers": []interface{}{
						map[string]interface{}{
							"selector": map[string]interface{}{
								"dnsZones": []interface{}{
									dnsZone,
								},
							},
							"dns01": map[string]interface{}{
								"route53": map[string]interface{}{
									"region": "us-east-1",
								},
							},
						},
					},
				},
			},
		},
	}

	return unstructuredToYAMLString(issuer)
}

func createCertificate(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {

	parts := strings.SplitN(domain, ".", 2)
	dnsZone := parts[1]
	kebabDnsZone := strcase.ToKebab(dnsZone)

	var certificate = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name": kebabDnsZone,
			},
			"spec": map[string]interface{}{
				"secretName": kebabDnsZone + "-tls",
				"dnsNames": []interface{}{
					dnsZone,
				},
				"issuerRef": map[string]interface{}{
					"name": kebabDnsZone,
					"kind": "Issuer",
				},
			},
		},
	}

	return unstructuredToYAMLString(certificate)
}

// unstructuredToYAMLString converts an unstructured object into a YAML string.
func unstructuredToYAMLString(unstructuredManifest *unstructured.Unstructured) (string, error) {
	bytes, err := yaml.Marshal(unstructuredManifest.Object)
	if err != nil {
		return "", fmt.Errorf("error marshaling YAML: %w", err)
	}
	stringManifest := string(bytes)
	return stringManifest, nil
}
