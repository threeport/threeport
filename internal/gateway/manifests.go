package gateway

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// createSupportServicesCollection creates a support services collection.
func createSupportServicesCollection() (string, error) {
	var supportServicesCollection = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "orchestration.support-services.nukleros.io/v1alpha1",
			"kind":       "SupportServices",
			"metadata": map[string]interface{}{
				"name": "supportservices-sample",
			},
			"spec": map[string]interface{}{
				"tier": "development",
			},
		},
	}

	return unstructuredToYAMLString(supportServicesCollection)
}

// createGlooEdge creates a gloo edge custom resource.
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

// createExternalDns creates an external DNS custom resource.
func createExternalDns(
	domain,
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
				"domainName":         domain,
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

// createGlooEdgePort creates a gloo edge port.
func createGlooEdgePort(protocol, name string, port int64, ssl bool) map[string]interface{} {

	var portObject = map[string]interface{}{
		"name":     name,
		"protocol": protocol,
		"port":     port,
		"ssl":      ssl,
	}

	return portObject
}

// createCertManager creates a cert manager for the given IAM role ARN.
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

// getCleanedDomain returns a cleaned domain.
func getCleanedDomain(domain string) string {
	cleanedDomain := strings.TrimPrefix(domain, "http://")
	cleanedDomain = strings.TrimPrefix(cleanedDomain, "https://")
	return cleanedDomain
}

// createVirtualServices creates a virtual service for the given domain.
func createVirtualServices(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition, domain string) ([]string, error) {

	var manifests []string

	domain = getCleanedDomain(domain)

	gatewayHttpPorts, err := client.GetGatewayHttpPortByGatewayDefinitionID(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway http ports: %w", err)
	}
	for _, httpPort := range *gatewayHttpPorts {

		// configure domain list
		var domainList []interface{}
		if domain == "" {
			domainList = []interface{}{"*"}
		} else {
			domainList = []interface{}{domain}
		}

		virtualServiceName := getVirtualServiceName(gatewayDefinition, domain, *httpPort.Port)

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
										"prefix": *httpPort.Path,
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
				},
			},
		}

		// configure ssl config
		if httpPort.TLSEnabled != nil && *httpPort.TLSEnabled {
			sslConfig := map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name":      strcase.ToKebab(domain) + "-tls",
					"namespace": "default",
				},
				"sniDomains": domainList,
			}
			unstructured.SetNestedMap(virtualService.Object, sslConfig, "spec", "sslConfig")
		}

		virtualServiceManifest, err := unstructuredToYAMLString(virtualService)
		if err != nil {
			return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
		}

		manifests = append(manifests, virtualServiceManifest)
	}

	// return util.HyphenDelimitedString(manifests), nil
	return manifests, nil
}

// getVirtualServiceName returns the name of a virtual service.
func getVirtualServiceName(gatewayDefinition *v0.GatewayDefinition, domain string, port int) string {

	domain = getCleanedDomain(domain)
	if domain == "" {
		return fmt.Sprintf("%s-%d", *gatewayDefinition.Name, port)
	} else {
		return fmt.Sprintf("%s-%d", strcase.ToKebab(domain), port)
	}
}

// createIssuer creates an issuer for the given domain.
func createIssuer(gatewayDefinition *v0.GatewayDefinition, domain, adminEmail string) (string, error) {

	var issuer = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name": strcase.ToKebab(domain),
			},
			"spec": map[string]interface{}{
				"acme": map[string]interface{}{
					"email":  adminEmail,
					"server": "https://acme-staging-v02.api.letsencrypt.org/directory",
					"privateKeySecretRef": map[string]interface{}{
						"name": "letsencrypt-prod-private-key",
					},
					"solvers": []interface{}{
						map[string]interface{}{
							"selector": map[string]interface{}{
								"dnsZones": []interface{}{
									domain,
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

// createCertificate creates a certificate for the given domain.
func createCertificate(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {

	var certificate = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name": strcase.ToKebab(domain),
			},
			"spec": map[string]interface{}{
				"secretName": strcase.ToKebab(domain) + "-tls",
				"dnsNames": []interface{}{
					domain,
				},
				"issuerRef": map[string]interface{}{
					"name": strcase.ToKebab(domain),
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
