package gateway

import (
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func CreateGlooEdge() *unstructured.Unstructured {

	var glooEdge = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "ingress.support-services.nukleros.io/v1alpha1",
			"kind":       "GlooEdge",
			"metadata": map[string]interface{}{
				"name": "glooedge",
			},
			"spec": nil,
		},
	}

	return glooEdge
}

func CreateGateway() *unstructured.Unstructured {

	var gateway = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Gateway",
			"metadata": map[string]interface{}{
				"name":      "gateway-proxy",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "gloo",
				},
			},
			"spec": map[string]interface{}{
				"bindAddress":   "::",
				"bindPort":      8080,
				"httpGateway":   map[string]interface{}{},
				"useProxyProto": false,
				"ssl":           false,
				"options": map[string]interface{}{
					"accessLoggingService": map[string]interface{}{
						"accessLog": []interface{}{
							map[string]interface{}{
								"fileSink": map[string]interface{}{
									"path":         "/dev/stdout",
									"stringFormat": "",
								},
							},
						},
					},
				},
				"proxyNames": []interface{}{
					"gateway-proxy",
				},
			},
		},
	}

	return gateway
}

func CreateGatewaySSL() *unstructured.Unstructured {

	var gatewaySSL = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.solo.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name":      "gateway-proxy-ssl",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "gloo",
				},
			},
			"spec": map[string]interface{}{
				"bindAddress":   "::",
				"bindPort":      8443,
				"httpGateway":   map[string]interface{}{},
				"useProxyProto": false,
				"ssl":           true,
				"options": map[string]interface{}{
					"accessLoggingService": map[string]interface{}{
						"accessLog": []interface{}{
							map[string]interface{}{
								"fileSink": map[string]interface{}{
									"path":         "/dev/stdout",
									"stringFormat": "",
								},
							},
						},
					},
				},
				"proxyNames": []interface{}{
					"gateway-proxy",
				},
			},
		},
	}

	return gatewaySSL
}

func CreateVirtualService(gatewayDefinition *v0.GatewayDefinition) (string, error) {

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
	bytes, err := yaml.Marshal(unstructuredManifest)
	if err != nil {
		return "", fmt.Errorf("error marshaling YAML: %w", err)
	}
	stringManifest := string(bytes)
	return stringManifest, nil
}
