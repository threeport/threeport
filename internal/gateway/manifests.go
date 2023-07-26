package gateway

import (
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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

func createGlooEdgePort(name string, port int64, ssl bool) map[string]interface{} {

	var portObject = map[string]interface{}{
		"name": name,
		"port": port,
		"ssl":  ssl,
	}

	return portObject
}

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

func createCertManager() (string, error) {

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
			},
		},
	}

	return unstructuredToYAMLString(certManager)
}

func createVirtualService(gatewayDefinition *v0.GatewayDefinition) (string, error) {

	var virtualService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.solo.io/v1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      "default",
				"namespace": "gloo-system",
			},
			"spec": map[string]interface{}{
				"virtualHost": map[string]interface{}{
					"domains": []interface{}{
						"*",
					},
					"routes": []interface{}{
						map[string]interface{}{
							"matchers": []interface{}{
								map[string]interface{}{
									"prefix": "/",
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

	return unstructuredToYAMLString(virtualService)
}

func createIssuer(gatewayDefinition *v0.GatewayDefinition) (string, error) {
	var issuer = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name":      "workload-123",
				"namespace": "workload-123",
			},
			"spec": map[string]interface{}{
				"acme": map[string]interface{}{
					"solvers": []interface{}{
						map[string]interface{}{
							"selector": map[string]interface{}{
								"dnsZones": []interface{}{
									"corp-domain.com",
								},
							},
							"dns01": map[string]interface{}{
								"route53": map[string]interface{}{
									"region": "us-east-1",
									"role":   "arn:aws:iam::YYYYYYYYYYYY:role/dns-manager",
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

func createCertificate(gatewayDefinition *v0.GatewayDefinition) (string, error) {

	var certificate = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "corp-domain",
				"namespace": "workload-123",
			},
			"spec": map[string]interface{}{
				"secretName": "corp-domain-tls",
				"dnsNames": []interface{}{
					"corp-domain.com",
				},
				"issuerRef": map[string]interface{}{
					"name": "workload-123",
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
