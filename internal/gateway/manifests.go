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

	return unstructuredToYamlString(supportServicesCollection)
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

	return unstructuredToYamlString(glooEdge)
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

	return unstructuredToYamlString(externalDns)
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

	return unstructuredToYamlString(certManager)
}

// getCleanedDomain returns a cleaned domain.
func getCleanedDomain(domain string) string {
	cleanedDomain := strings.TrimPrefix(domain, "http://")
	cleanedDomain = strings.TrimPrefix(cleanedDomain, "https://")
	return cleanedDomain
}

// createVirtualServicesYaml creates a virtual service for the given domain.
func createVirtualServicesYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition, domain string) ([]string, error) {

	var manifests []string

	domain = getCleanedDomain(domain)

	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
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
							},
						},
					},
				},
			},
		}

		// get route array
		routes, found, err := unstructured.NestedSlice(virtualService.Object, "spec", "virtualHost", "routes")
		if err != nil || !found {
			return nil, fmt.Errorf("failed to get virtualservice route: %w", err)
		}
		if len(routes) == 0 {
			return nil, fmt.Errorf("no routes found")
		}

		// configure https redirect
		if httpPort.HTTPSRedirect != nil && *httpPort.HTTPSRedirect {
			redirectAction := map[string]interface{}{
				"hostRedirect":  domain,
				"httpsRedirect": true,
			}
			unstructured.SetNestedMap(
				routes[0].(map[string]interface{}),
				redirectAction,
				"redirectAction",
			)
		} else {
			// configure route action
			routeAction := map[string]interface{}{
				"single": map[string]interface{}{
					"upstream": map[string]interface{}{
						"name":      "my-upstream",
						"namespace": "gloo-system",
					},
				},
			}
			unstructured.SetNestedMap(
				routes[0].(map[string]interface{}),
				routeAction,
				"routeAction",
			)
		}

		// set route field
		err = unstructured.SetNestedSlice(virtualService.Object, routes, "spec", "virtualHost", "routes")
		if err != nil {
			return nil, fmt.Errorf("failed to set route on virtual service: %w", err)
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

		virtualServiceManifest, err := unstructuredToYamlString(virtualService)
		if err != nil {
			return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
		}

		manifests = append(manifests, virtualServiceManifest)
	}

	return manifests, nil
}

// createTcpGatewaysYaml creates a tcp gateway for the given domain.
func createTcpGatewaysYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) ([]string, error) {

	var manifests []string

	gatewayTcpPorts, err := client.GetGatewayTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway tcp ports: %w", err)
	}
	for _, tcpPort := range *gatewayTcpPorts {

		tcpGateway := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.solo.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("%s-%d", *gatewayDefinition.Name, *tcpPort.Port),
					"namespace": "gloo-system",
				},
				"spec": map[string]interface{}{
					"bindAddress": "::",
					"bindPort":    8000 + *tcpPort.Port,
					"tcpGateway": map[string]interface{}{
						"tcpHosts": []interface{}{
							map[string]interface{}{
								"name": "upstream-host",
								"destination": map[string]interface{}{
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
					"useProxyProto": false,
				},
			},
		}

		// TODO: configure ssl
		// if tcpPort.TLSEnabled != nil && *tcpPort.TLSEnabled {
		// }

		virtualServiceManifest, err := unstructuredToYamlString(tcpGateway)
		if err != nil {
			return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
		}

		manifests = append(manifests, virtualServiceManifest)
	}

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

// createIssuerYaml creates an issuer for the given domain.
func createIssuerYaml(gatewayDefinition *v0.GatewayDefinition, domain, adminEmail string) (string, error) {

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

	return unstructuredToYamlString(issuer)
}

// createCertificateYaml creates a certificate for the given domain.
func createCertificateYaml(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {

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

	return unstructuredToYamlString(certificate)
}

// unstructuredToYamlString converts an unstructured object into a YAML string.
func unstructuredToYamlString(unstructuredManifest *unstructured.Unstructured) (string, error) {
	bytes, err := yaml.Marshal(unstructuredManifest.Object)
	if err != nil {
		return "", fmt.Errorf("error marshaling YAML: %w", err)
	}
	stringManifest := string(bytes)
	return stringManifest, nil
}
