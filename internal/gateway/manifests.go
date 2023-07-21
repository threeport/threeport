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
			"spec": nil,
		},
	}

	return unstructuredToYAMLString(glooEdge)
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

// unstructuredToYAMLString converts an unstructured object into a YAML string.
func unstructuredToYAMLString(unstructuredManifest *unstructured.Unstructured) (string, error) {
	bytes, err := yaml.Marshal(unstructuredManifest.Object)
	if err != nil {
		return "", fmt.Errorf("error marshaling YAML: %w", err)
	}
	stringManifest := string(bytes)
	return stringManifest, nil
}
