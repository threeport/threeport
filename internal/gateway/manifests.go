package gateway

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func CreateGlooEdge() *unstructured.Unstructured {

	var glooEdge = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "ingress.support-services.nukleros.io/v1alpha1",
			"kind":       "GlooEdge",
			"metadata": map[string]interface{}{
				"name": "glooedge-sample",
			},
			"spec": nil,
		},
	}

	return glooEdge
}

func CreateGateway() *unstructured.Unstructured {

	var gateway = &unstructured.Unstructured{
		Object: map[string]interface{}{
				"apiVersion": "gateway.solo.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
								"app": "gloo",
						},
						"name":      "gateway-proxy-ssl",
						"namespace": "gloo-system",
				},
				"spec": map[string]interface{}{
						"bindAddress": "::",
						"bindPort":    8443,
						"httpGateway": map[string]interface{}{},
						"proxyNames": []interface{}{
								"gateway-proxy",
						},
						"ssl":           true,
						"useProxyProto": false,
				},
		},
	}

	return gateway
}

func CreateVirtualService() *unstructured.Unstructured {

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

	return virtualService
}