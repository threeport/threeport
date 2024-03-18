package gateway

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// getGlooEdgeYaml creates a gloo edge custom resource.
func getGlooEdgeYaml() (string, error) {

	var glooEdge = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.support-services.nukleros.io/v1alpha1",
			"kind":       "GlooEdge",
			"metadata": map[string]interface{}{
				"name": "glooedge",
			},
			"spec": map[string]interface{}{
				"namespace": util.GatewaySystemNamespace,
				"ports":     []interface{}{},
			},
		},
	}

	return util.UnstructuredToYaml(glooEdge)
}

// getExternalDnsYaml creates an external DNS custom resource.
func getExternalDnsYaml(
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
				"name": "externaldns",
			},
			"spec": map[string]interface{}{
				"namespace":          util.GatewaySystemNamespace,
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

	return util.UnstructuredToYaml(externalDns)
}

// getGlooEdgePort creates a gloo edge port.
func getGlooEdgePort(protocol, name string, port int64, ssl bool) *unstructured.Unstructured {

	var portObject = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"name":     name,
			"protocol": protocol,
			"port":     port,
			"ssl":      ssl,
		},
	}

	return portObject
}

// getCertManagerYaml creates a cert manager for the given IAM role ARN.
func getCertManagerYaml(iamRoleArn string) (string, error) {

	var certManager = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "certificates.support-services.nukleros.io/v1alpha1",
			"kind":       "CertManager",
			"metadata": map[string]interface{}{
				"name": "certmanager",
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

	return util.UnstructuredToYaml(certManager)
}

// getCleanedDomain returns a cleaned domain.
func getCleanedDomain(domain string) string {
	cleanedDomain := strings.TrimPrefix(domain, "http://")
	cleanedDomain = strings.TrimPrefix(cleanedDomain, "https://")
	return cleanedDomain
}

// getVirtualServicesYaml creates a virtual service for the given domain.
func getVirtualServicesYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition, domain string) ([]string, error) {

	var manifests []string

	domain = getCleanedDomain(domain)

	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
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

		virtualServiceManifest, err := util.UnstructuredToYaml(virtualService)
		if err != nil {
			return []string{}, fmt.Errorf("error marshaling YAML: %w", err)
		}

		manifests = append(manifests, virtualServiceManifest)
	}

	return manifests, nil
}

// getTcpGatewaysYaml creates a tcp gateway for the given domain.
func getTcpGatewaysYaml(r *controller.Reconciler, gatewayDefinition *v0.GatewayDefinition) ([]string, error) {

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

		virtualServiceManifest, err := util.UnstructuredToYaml(tcpGateway)
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

// getIssuerYaml creates an issuer for the given domain.
func getIssuerYaml(gatewayDefinition *v0.GatewayDefinition, domain, adminEmail string) (string, error) {

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

	return util.UnstructuredToYaml(issuer)
}

// getCertificateYaml creates a certificate for the given domain.
func getCertificateYaml(gatewayDefinition *v0.GatewayDefinition, domain string) (string, error) {

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

	return util.UnstructuredToYaml(certificate)
}
