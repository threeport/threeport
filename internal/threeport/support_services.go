package threeport

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/kube"
)

const (
	SupportServicesNamespace     = "support-services-system"
	SupportServicesOperatorImage = "ghcr.io/nukleros/support-services-operator:v0.1.12"
	RBACProxyImage               = "gcr.io/kubebuilder/kube-rbac-proxy:v0.8.0"

	// links the service account delcared in the IngressComponent resource to the
	// resource config for eks-cluster to create the attached IAM role.
	DNSManagerServiceAccountName     = "external-dns"
	DNSManagerServiceAccountNamepace = "nukleros-ingress-system"

	// links the service account used by the EBS CSI driver to the resource
	// config for eks-cluster to create the attached IAM role.
	StorageManagerServiceAccountName      = "ebs-csi-controller-sa"
	StorageManagerServiceAccountNamespace = "kube-system"

	// links the service account used by the cluster autoscaler installation to
	// the config for eks-cluster to create the attached IAM role.
	ClusterAutoscalerServiceAccountName      = "cluster-autoscaler"
	ClusterAutoscalerServiceAccountNamespace = "kube-system"
)

// InstallThreeportCRDs installs all CRDs needed by threeport in the target
// cluster.
func InstallThreeportCRDs(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	var certsComponentCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "certificatescomponents.platform.addons.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "platform.addons.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "CertificatesComponent",
					"listKind": "CertificatesComponentList",
					"plural":   "certificatescomponents",
					"singular": "certificatescomponent",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "CertificatesComponent is the Schema for the certificatescomponents API.",
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{
										"description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
										"type":        "string",
									},
									"kind": map[string]interface{}{
										"description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
										"type":        "string",
									},
									"metadata": map[string]interface{}{
										"type": "object",
									},
									"spec": map[string]interface{}{
										"description": "CertificatesComponentSpec defines the desired state of CertificatesComponent.",
										"properties": map[string]interface{}{
											"certManager": map[string]interface{}{
												"properties": map[string]interface{}{
													"cainjector": map[string]interface{}{
														"properties": map[string]interface{}{
															"image": map[string]interface{}{
																"default": "quay.io/jetstack/cert-manager-cainjector",
																"description": `(Default: "quay.io/jetstack/cert-manager-cainjector") 
 Image repo and name to use for cert-manager cainjector.`,
																"type": "string",
															},
															"replicas": map[string]interface{}{
																"default": 2,
																"description": `(Default: 2) 
 Number of replicas to use for the cert-manager cainjector deployment.`,
																"type": "integer",
															},
														},
														"type": "object",
													},
													"contactEmail": map[string]interface{}{
														"description": "Contact e-mail address for receiving updates about certificates from LetsEncrypt.",
														"type":        "string",
													},
													"controller": map[string]interface{}{
														"properties": map[string]interface{}{
															"image": map[string]interface{}{
																"default": "quay.io/jetstack/cert-manager-controller",
																"description": `(Default: "quay.io/jetstack/cert-manager-controller") 
 Image repo and name to use for cert-manager controller.`,
																"type": "string",
															},
															"replicas": map[string]interface{}{
																"default": 2,
																"description": `(Default: 2) 
 Number of replicas to use for the cert-manager controller deployment.`,
																"type": "integer",
															},
														},
														"type": "object",
													},
													"version": map[string]interface{}{
														"default": "v1.9.1",
														"description": `(Default: "v1.9.1") 
 Version of cert-manager to use.`,
														"type": "string",
													},
													"webhook": map[string]interface{}{
														"properties": map[string]interface{}{
															"image": map[string]interface{}{
																"default": "quay.io/jetstack/cert-manager-webhook",
																"description": `(Default: "quay.io/jetstack/cert-manager-webhook") 
 Image repo and name to use for cert-manager webhook.`,
																"type": "string",
															},
															"replicas": map[string]interface{}{
																"default": 2,
																"description": `(Default: 2) 
 Number of replicas to use for the cert-manager webhook deployment.`,
																"type": "integer",
															},
														},
														"type": "object",
													},
												},
												"type": "object",
											},
											"collection": map[string]interface{}{
												"description": "Specifies a reference to the collection to use for this workload. Requires the name and namespace input to find the collection. If no collection field is set, default to selecting the only workload collection in the cluster, which will result in an error if not exactly one collection is found.",
												"properties": map[string]interface{}{
													"name": map[string]interface{}{
														"description": "Required if specifying collection.  The name of the collection within a specific collection.namespace to reference.",
														"type":        "string",
													},
													"namespace": map[string]interface{}{
														"description": "(Default: \"\") The namespace where the collection exists.  Required only if the collection is namespace scoped and not cluster scoped.",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"name",
												},
												"type": "object",
											},
											"namespace": map[string]interface{}{
												"default": "nukleros-certs-system",
												"description": `(Default: "nukleros-certs-system") 
 Namespace to use for certificate support services.`,
												"type": "string",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "CertificatesComponentStatus defines the observed state of CertificatesComponent.",
										"properties": map[string]interface{}{
											"conditions": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "PhaseCondition describes an event that has occurred during a phase of the controller reconciliation loop.",
													"properties": map[string]interface{}{
														"lastModified": map[string]interface{}{
															"description": "LastModified defines the time in which this component was updated.",
															"type":        "string",
														},
														"message": map[string]interface{}{
															"description": "Message defines a helpful message from the phase.",
															"type":        "string",
														},
														"phase": map[string]interface{}{
															"description": "Phase defines the phase in which the condition was set.",
															"type":        "string",
														},
														"state": map[string]interface{}{
															"description": "PhaseState defines the current state of the phase.",
															"enum": []interface{}{
																"Complete",
																"Reconciling",
																"Failed",
																"Pending",
															},
															"type": "string",
														},
													},
													"required": []interface{}{
														"lastModified",
														"message",
														"phase",
														"state",
													},
													"type": "object",
												},
												"type": "array",
											},
											"created": map[string]interface{}{
												"type": "boolean",
											},
											"dependenciesSatisfied": map[string]interface{}{
												"type": "boolean",
											},
											"resources": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "ChildResource is the resource and its condition as stored on the workload custom resource's status field.",
													"properties": map[string]interface{}{
														"condition": map[string]interface{}{
															"description": "ResourceCondition defines the current condition of this resource.",
															"properties": map[string]interface{}{
																"created": map[string]interface{}{
																	"description": "Created defines whether this object has been successfully created or not.",
																	"type":        "boolean",
																},
																"lastModified": map[string]interface{}{
																	"description": "LastModified defines the time in which this resource was updated.",
																	"type":        "string",
																},
																"message": map[string]interface{}{
																	"description": "Message defines a helpful message from the resource phase.",
																	"type":        "string",
																},
															},
															"required": []interface{}{
																"created",
															},
															"type": "object",
														},
														"group": map[string]interface{}{
															"description": "Group defines the API Group of the resource.",
															"type":        "string",
														},
														"kind": map[string]interface{}{
															"description": "Kind defines the kind of the resource.",
															"type":        "string",
														},
														"name": map[string]interface{}{
															"description": "Name defines the name of the resource from the metadata.name field.",
															"type":        "string",
														},
														"namespace": map[string]interface{}{
															"description": "Namespace defines the namespace in which this resource exists in.",
															"type":        "string",
														},
														"version": map[string]interface{}{
															"description": "Version defines the API Version of the resource.",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"group",
														"kind",
														"name",
														"namespace",
														"version",
													},
													"type": "object",
												},
												"type": "array",
											},
										},
										"type": "object",
									},
								},
								"type": "object",
							},
						},
						"served":  true,
						"storage": true,
						"subresources": map[string]interface{}{
							"status": map[string]interface{}{},
						},
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(certsComponentCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services cert component CRD: %w", err)
	}

	var ingressComponentCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "ingresscomponents.platform.addons.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "platform.addons.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "IngressComponent",
					"listKind": "IngressComponentList",
					"plural":   "ingresscomponents",
					"singular": "ingresscomponent",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "IngressComponent is the Schema for the ingresscomponents API.",
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{
										"description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
										"type":        "string",
									},
									"kind": map[string]interface{}{
										"description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
										"type":        "string",
									},
									"metadata": map[string]interface{}{
										"type": "object",
									},
									"spec": map[string]interface{}{
										"description": "IngressComponentSpec defines the desired state of IngressComponent.",
										"properties": map[string]interface{}{
											"collection": map[string]interface{}{
												"description": "Specifies a reference to the collection to use for this workload. Requires the name and namespace input to find the collection. If no collection field is set, default to selecting the only workload collection in the cluster, which will result in an error if not exactly one collection is found.",
												"properties": map[string]interface{}{
													"name": map[string]interface{}{
														"description": "Required if specifying collection.  The name of the collection within a specific collection.namespace to reference.",
														"type":        "string",
													},
													"namespace": map[string]interface{}{
														"description": "(Default: \"\") The namespace where the collection exists.  Required only if the collection is namespace scoped and not cluster scoped.",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"name",
												},
												"type": "object",
											},
											"domainName": map[string]interface{}{
												"type": "string",
											},
											"externalDNS": map[string]interface{}{
												"properties": map[string]interface{}{
													"iamRoleArn": map[string]interface{}{
														"description": "On AWS, the IAM Role ARN that gives external-dns access to Route53",
														"type":        "string",
													},
													"image": map[string]interface{}{
														"default": "k8s.gcr.io/external-dns/external-dns",
														"description": `(Default: "k8s.gcr.io/external-dns/external-dns") 
 Image repo and name to use for external-dns.`,
														"type": "string",
													},
													"provider": map[string]interface{}{
														"default": "none",
														"description": `(Default: "none") 
 The DNS provider to use for setting DNS records with external-dns.  One of: none | active-directory | google | route53.`,
														"enum": []interface{}{
															"none",
															"active-directory",
															"google",
															"route53",
														},
														"type": "string",
													},
													"serviceAccountName": map[string]interface{}{
														"default": "external-dns",
														"description": `(Default: "external-dns") 
 The name of the external-dns service account which is referenced in role policy doc for AWS.`,
														"type": "string",
													},
													"version": map[string]interface{}{
														"default": "v0.12.2",
														"description": `(Default: "v0.12.2") 
 Version of external-dns to use.`,
														"type": "string",
													},
													"zoneType": map[string]interface{}{
														"default": "private",
														"description": `(Default: "private") 
 Type of DNS hosted zone to manage.`,
														"enum": []interface{}{
															"private",
															"public",
														},
														"type": "string",
													},
												},
												"type": "object",
											},
											"kong": map[string]interface{}{
												"properties": map[string]interface{}{
													"gateway": map[string]interface{}{
														"properties": map[string]interface{}{
															"image": map[string]interface{}{
																"default": "kong/kong-gateway",
																"description": `(Default: "kong/kong-gateway") 
 Image repo and name to use for kong gateway.`,
																"type": "string",
															},
															"version": map[string]interface{}{
																"default": "2.8",
																"description": `(Default: "2.8") 
 Version of kong gateway to use.`,
																"type": "string",
															},
														},
														"type": "object",
													},
													"include": map[string]interface{}{
														"default": true,
														"description": `(Default: true) 
 Include the Kong ingress controller when installing ingress components.`,
														"type": "boolean",
													},
													"ingressController": map[string]interface{}{
														"properties": map[string]interface{}{
															"image": map[string]interface{}{
																"default": "kong/kubernetes-ingress-controller",
																"description": `(Default: "kong/kubernetes-ingress-controller") 
 Image repo and name to use for kong ingress controller.`,
																"type": "string",
															},
															"version": map[string]interface{}{
																"default": "2.5.0",
																"description": `(Default: "2.5.0") 
 Version of kong ingress controller to use.`,
																"type": "string",
															},
														},
														"type": "object",
													},
													"proxyServiceName": map[string]interface{}{
														"default":     "kong-proxy",
														"description": "(Default: \"kong-proxy\")",
														"type":        "string",
													},
													"replicas": map[string]interface{}{
														"default": 2,
														"description": `(Default: 2) 
 Number of replicas to use for the kong ingress deployment.`,
														"type": "integer",
													},
												},
												"type": "object",
											},
											"namespace": map[string]interface{}{
												"default": "nukleros-ingress-system",
												"description": `(Default: "nukleros-ingress-system") 
 Namespace to use for ingress support services.`,
												"type": "string",
											},
											"nginx": map[string]interface{}{
												"properties": map[string]interface{}{
													"image": map[string]interface{}{
														"default": "nginx/nginx-ingress",
														"description": `(Default: "nginx/nginx-ingress") 
 Image repo and name to use for nginx.`,
														"type": "string",
													},
													"include": map[string]interface{}{
														"default": false,
														"description": `(Default: false) 
 Include the Nginx ingress controller when installing ingress components.`,
														"type": "boolean",
													},
													"installType": map[string]interface{}{
														"default": "deployment",
														"description": `(Default: "deployment") 
 Method of install nginx ingress controller.  One of: deployment | daemonset.`,
														"enum": []interface{}{
															"deployment",
															"daemonset",
														},
														"type": "string",
													},
													"replicas": map[string]interface{}{
														"default": 2,
														"description": `(Default: 2) 
 Number of replicas to use for the nginx ingress controller deployment.`,
														"type": "integer",
													},
													"version": map[string]interface{}{
														"default": "2.3.0",
														"description": `(Default: "2.3.0") 
 Version of nginx to use.`,
														"type": "string",
													},
												},
												"type": "object",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "IngressComponentStatus defines the observed state of IngressComponent.",
										"properties": map[string]interface{}{
											"conditions": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "PhaseCondition describes an event that has occurred during a phase of the controller reconciliation loop.",
													"properties": map[string]interface{}{
														"lastModified": map[string]interface{}{
															"description": "LastModified defines the time in which this component was updated.",
															"type":        "string",
														},
														"message": map[string]interface{}{
															"description": "Message defines a helpful message from the phase.",
															"type":        "string",
														},
														"phase": map[string]interface{}{
															"description": "Phase defines the phase in which the condition was set.",
															"type":        "string",
														},
														"state": map[string]interface{}{
															"description": "PhaseState defines the current state of the phase.",
															"enum": []interface{}{
																"Complete",
																"Reconciling",
																"Failed",
																"Pending",
															},
															"type": "string",
														},
													},
													"required": []interface{}{
														"lastModified",
														"message",
														"phase",
														"state",
													},
													"type": "object",
												},
												"type": "array",
											},
											"created": map[string]interface{}{
												"type": "boolean",
											},
											"dependenciesSatisfied": map[string]interface{}{
												"type": "boolean",
											},
											"resources": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "ChildResource is the resource and its condition as stored on the workload custom resource's status field.",
													"properties": map[string]interface{}{
														"condition": map[string]interface{}{
															"description": "ResourceCondition defines the current condition of this resource.",
															"properties": map[string]interface{}{
																"created": map[string]interface{}{
																	"description": "Created defines whether this object has been successfully created or not.",
																	"type":        "boolean",
																},
																"lastModified": map[string]interface{}{
																	"description": "LastModified defines the time in which this resource was updated.",
																	"type":        "string",
																},
																"message": map[string]interface{}{
																	"description": "Message defines a helpful message from the resource phase.",
																	"type":        "string",
																},
															},
															"required": []interface{}{
																"created",
															},
															"type": "object",
														},
														"group": map[string]interface{}{
															"description": "Group defines the API Group of the resource.",
															"type":        "string",
														},
														"kind": map[string]interface{}{
															"description": "Kind defines the kind of the resource.",
															"type":        "string",
														},
														"name": map[string]interface{}{
															"description": "Name defines the name of the resource from the metadata.name field.",
															"type":        "string",
														},
														"namespace": map[string]interface{}{
															"description": "Namespace defines the namespace in which this resource exists in.",
															"type":        "string",
														},
														"version": map[string]interface{}{
															"description": "Version defines the API Version of the resource.",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"group",
														"kind",
														"name",
														"namespace",
														"version",
													},
													"type": "object",
												},
												"type": "array",
											},
										},
										"type": "object",
									},
								},
								"type": "object",
							},
						},
						"served":  true,
						"storage": true,
						"subresources": map[string]interface{}{
							"status": map[string]interface{}{},
						},
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(ingressComponentCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services ingress component CRD: %w", err)
	}

	var secretsComponentCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "secretscomponents.platform.addons.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "platform.addons.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "SecretsComponent",
					"listKind": "SecretsComponentList",
					"plural":   "secretscomponents",
					"singular": "secretscomponent",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "SecretsComponent is the Schema for the secretscomponents API.",
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{
										"description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
										"type":        "string",
									},
									"kind": map[string]interface{}{
										"description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
										"type":        "string",
									},
									"metadata": map[string]interface{}{
										"type": "object",
									},
									"spec": map[string]interface{}{
										"description": "SecretsComponentSpec defines the desired state of SecretsComponent.",
										"properties": map[string]interface{}{
											"collection": map[string]interface{}{
												"description": "Specifies a reference to the collection to use for this workload. Requires the name and namespace input to find the collection. If no collection field is set, default to selecting the only workload collection in the cluster, which will result in an error if not exactly one collection is found.",
												"properties": map[string]interface{}{
													"name": map[string]interface{}{
														"description": "Required if specifying collection.  The name of the collection within a specific collection.namespace to reference.",
														"type":        "string",
													},
													"namespace": map[string]interface{}{
														"description": "(Default: \"\") The namespace where the collection exists.  Required only if the collection is namespace scoped and not cluster scoped.",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"name",
												},
												"type": "object",
											},
											"externalSecrets": map[string]interface{}{
												"properties": map[string]interface{}{
													"certController": map[string]interface{}{
														"properties": map[string]interface{}{
															"replicas": map[string]interface{}{
																"default": 1,
																"description": `(Default: 1) 
 Number of replicas to use for the external-secrets cert-controller deployment.`,
																"type": "integer",
															},
														},
														"type": "object",
													},
													"controller": map[string]interface{}{
														"properties": map[string]interface{}{
															"replicas": map[string]interface{}{
																"default": 2,
																"description": `(Default: 2) 
 Number of replicas to use for the external-secrets controller deployment.`,
																"type": "integer",
															},
														},
														"type": "object",
													},
													"image": map[string]interface{}{
														"default": "ghcr.io/external-secrets/external-secrets",
														"description": `(Default: "ghcr.io/external-secrets/external-secrets") 
 Image repo and name to use for external-secrets.`,
														"type": "string",
													},
													"version": map[string]interface{}{
														"default": "v0.5.9",
														"description": `(Default: "v0.5.9") 
 Version of external-secrets to use.`,
														"type": "string",
													},
													"webhook": map[string]interface{}{
														"properties": map[string]interface{}{
															"replicas": map[string]interface{}{
																"default": 2,
																"description": `(Default: 2) 
 Number of replicas to use for the external-secrets webhook deployment.`,
																"type": "integer",
															},
														},
														"type": "object",
													},
												},
												"type": "object",
											},
											"namespace": map[string]interface{}{
												"default": "nukleros-secrets-system",
												"description": `(Default: "nukleros-secrets-system") 
 Namespace to use for secrets support services.`,
												"type": "string",
											},
											"reloader": map[string]interface{}{
												"properties": map[string]interface{}{
													"image": map[string]interface{}{
														"default": "stakater/reloader",
														"description": `(Default: "stakater/reloader") 
 Image repo and name to use for reloader.`,
														"type": "string",
													},
													"replicas": map[string]interface{}{
														"default": 1,
														"description": `(Default: 1) 
 Number of replicas to use for the reloader deployment.`,
														"type": "integer",
													},
													"version": map[string]interface{}{
														"default": "v0.0.119",
														"description": `(Default: "v0.0.119") 
 Version of reloader to use.`,
														"type": "string",
													},
												},
												"type": "object",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "SecretsComponentStatus defines the observed state of SecretsComponent.",
										"properties": map[string]interface{}{
											"conditions": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "PhaseCondition describes an event that has occurred during a phase of the controller reconciliation loop.",
													"properties": map[string]interface{}{
														"lastModified": map[string]interface{}{
															"description": "LastModified defines the time in which this component was updated.",
															"type":        "string",
														},
														"message": map[string]interface{}{
															"description": "Message defines a helpful message from the phase.",
															"type":        "string",
														},
														"phase": map[string]interface{}{
															"description": "Phase defines the phase in which the condition was set.",
															"type":        "string",
														},
														"state": map[string]interface{}{
															"description": "PhaseState defines the current state of the phase.",
															"enum": []interface{}{
																"Complete",
																"Reconciling",
																"Failed",
																"Pending",
															},
															"type": "string",
														},
													},
													"required": []interface{}{
														"lastModified",
														"message",
														"phase",
														"state",
													},
													"type": "object",
												},
												"type": "array",
											},
											"created": map[string]interface{}{
												"type": "boolean",
											},
											"dependenciesSatisfied": map[string]interface{}{
												"type": "boolean",
											},
											"resources": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "ChildResource is the resource and its condition as stored on the workload custom resource's status field.",
													"properties": map[string]interface{}{
														"condition": map[string]interface{}{
															"description": "ResourceCondition defines the current condition of this resource.",
															"properties": map[string]interface{}{
																"created": map[string]interface{}{
																	"description": "Created defines whether this object has been successfully created or not.",
																	"type":        "boolean",
																},
																"lastModified": map[string]interface{}{
																	"description": "LastModified defines the time in which this resource was updated.",
																	"type":        "string",
																},
																"message": map[string]interface{}{
																	"description": "Message defines a helpful message from the resource phase.",
																	"type":        "string",
																},
															},
															"required": []interface{}{
																"created",
															},
															"type": "object",
														},
														"group": map[string]interface{}{
															"description": "Group defines the API Group of the resource.",
															"type":        "string",
														},
														"kind": map[string]interface{}{
															"description": "Kind defines the kind of the resource.",
															"type":        "string",
														},
														"name": map[string]interface{}{
															"description": "Name defines the name of the resource from the metadata.name field.",
															"type":        "string",
														},
														"namespace": map[string]interface{}{
															"description": "Namespace defines the namespace in which this resource exists in.",
															"type":        "string",
														},
														"version": map[string]interface{}{
															"description": "Version defines the API Version of the resource.",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"group",
														"kind",
														"name",
														"namespace",
														"version",
													},
													"type": "object",
												},
												"type": "array",
											},
										},
										"type": "object",
									},
								},
								"type": "object",
							},
						},
						"served":  true,
						"storage": true,
						"subresources": map[string]interface{}{
							"status": map[string]interface{}{},
						},
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(secretsComponentCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services secrets component CRD: %w", err)
	}

	var setupCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "supportservices.setup.addons.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "setup.addons.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "SupportServices",
					"listKind": "SupportServicesList",
					"plural":   "supportservices",
					"singular": "supportservices",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "SupportServices is the Schema for the supportservices API.",
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{
										"description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
										"type":        "string",
									},
									"kind": map[string]interface{}{
										"description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
										"type":        "string",
									},
									"metadata": map[string]interface{}{
										"type": "object",
									},
									"spec": map[string]interface{}{
										"description": "SupportServicesSpec defines the desired state of SupportServices.",
										"properties": map[string]interface{}{
											"defaultIngressController": map[string]interface{}{
												"default": "kong",
												"description": `(Default: "kong") 
 The default ingress for setting TLS certs.  One of: kong | nginx.`,
												"enum": []interface{}{
													"kong",
													"nginx",
												},
												"type": "string",
											},
											"tier": map[string]interface{}{
												"default": "development",
												"description": `(Default: "development") 
 The tier of cluster being used.  One of: development | staging | production.`,
												"enum": []interface{}{
													"development",
													"staging",
													"production",
												},
												"type": "string",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "SupportServicesStatus defines the observed state of SupportServices.",
										"properties": map[string]interface{}{
											"conditions": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "PhaseCondition describes an event that has occurred during a phase of the controller reconciliation loop.",
													"properties": map[string]interface{}{
														"lastModified": map[string]interface{}{
															"description": "LastModified defines the time in which this component was updated.",
															"type":        "string",
														},
														"message": map[string]interface{}{
															"description": "Message defines a helpful message from the phase.",
															"type":        "string",
														},
														"phase": map[string]interface{}{
															"description": "Phase defines the phase in which the condition was set.",
															"type":        "string",
														},
														"state": map[string]interface{}{
															"description": "PhaseState defines the current state of the phase.",
															"enum": []interface{}{
																"Complete",
																"Reconciling",
																"Failed",
																"Pending",
															},
															"type": "string",
														},
													},
													"required": []interface{}{
														"lastModified",
														"message",
														"phase",
														"state",
													},
													"type": "object",
												},
												"type": "array",
											},
											"created": map[string]interface{}{
												"type": "boolean",
											},
											"dependenciesSatisfied": map[string]interface{}{
												"type": "boolean",
											},
											"resources": map[string]interface{}{
												"items": map[string]interface{}{
													"description": "ChildResource is the resource and its condition as stored on the workload custom resource's status field.",
													"properties": map[string]interface{}{
														"condition": map[string]interface{}{
															"description": "ResourceCondition defines the current condition of this resource.",
															"properties": map[string]interface{}{
																"created": map[string]interface{}{
																	"description": "Created defines whether this object has been successfully created or not.",
																	"type":        "boolean",
																},
																"lastModified": map[string]interface{}{
																	"description": "LastModified defines the time in which this resource was updated.",
																	"type":        "string",
																},
																"message": map[string]interface{}{
																	"description": "Message defines a helpful message from the resource phase.",
																	"type":        "string",
																},
															},
															"required": []interface{}{
																"created",
															},
															"type": "object",
														},
														"group": map[string]interface{}{
															"description": "Group defines the API Group of the resource.",
															"type":        "string",
														},
														"kind": map[string]interface{}{
															"description": "Kind defines the kind of the resource.",
															"type":        "string",
														},
														"name": map[string]interface{}{
															"description": "Name defines the name of the resource from the metadata.name field.",
															"type":        "string",
														},
														"namespace": map[string]interface{}{
															"description": "Namespace defines the namespace in which this resource exists in.",
															"type":        "string",
														},
														"version": map[string]interface{}{
															"description": "Version defines the API Version of the resource.",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"group",
														"kind",
														"name",
														"namespace",
														"version",
													},
													"type": "object",
												},
												"type": "array",
											},
										},
										"type": "object",
									},
								},
								"type": "object",
							},
						},
						"served":  true,
						"storage": true,
						"subresources": map[string]interface{}{
							"status": map[string]interface{}{},
						},
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(setupCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services setup CRD: %w", err)
	}

	return nil
}

// InstallThreeportSupportServices installs the threeport control plane's support
// services, e.g. TLS assets.
func InstallThreeportSupportServices(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	adminEmail string,
) error {
	var namespace = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"control-plane":               "controller-manager",
					"kubernetes.io/metadata.name": "support-services-operator-system",
				},
				"name": SupportServicesNamespace,
			},
		},
	}
	if _, err := kube.CreateResource(namespace, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services namespace: %w", err)
	}

	var serviceAccount = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name":      "support-services-operator-controller-manager",
				"namespace": SupportServicesNamespace,
			},
		},
	}
	if _, err := kube.CreateResource(serviceAccount, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services service account: %w", err)
	}

	var leaderElectionRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      "support-services-operator-leader-election-role",
				"namespace": SupportServicesNamespace,
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
						"create",
						"update",
						"patch",
						"delete",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"coordination.k8s.io",
					},
					"resources": []interface{}{
						"leases",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
						"create",
						"update",
						"patch",
						"delete",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"events",
					},
					"verbs": []interface{}{
						"create",
						"patch",
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(leaderElectionRole, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services leader election role: %w", err)
	}

	var managerRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "support-services-operator-manager-role",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acid.zalan.do",
					},
					"resources": []interface{}{
						"operatorconfigurations",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acid.zalan.do",
					},
					"resources": []interface{}{
						"postgresqls",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acid.zalan.do",
					},
					"resources": []interface{}{
						"postgresqls/status",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acid.zalan.do",
					},
					"resources": []interface{}{
						"postgresteams",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acme.cert-manager.io",
					},
					"resources": []interface{}{
						"challenges",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acme.cert-manager.io",
					},
					"resources": []interface{}{
						"challenges/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acme.cert-manager.io",
					},
					"resources": []interface{}{
						"challenges/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acme.cert-manager.io",
					},
					"resources": []interface{}{
						"orders",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acme.cert-manager.io",
					},
					"resources": []interface{}{
						"orders/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"acme.cert-manager.io",
					},
					"resources": []interface{}{
						"orders/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"admissionregistration.k8s.io",
					},
					"resources": []interface{}{
						"mutatingwebhookconfigurations",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"admissionregistration.k8s.io",
					},
					"resources": []interface{}{
						"validatingwebhookconfigurations",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"apiextensions.k8s.io",
					},
					"resources": []interface{}{
						"customresourcedefinitions",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"apiregistration.k8s.io",
					},
					"resources": []interface{}{
						"apiservices",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"application.addons.nukleros.io",
					},
					"resources": []interface{}{
						"databasecomponents",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"application.addons.nukleros.io",
					},
					"resources": []interface{}{
						"databasecomponents/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"apps",
					},
					"resources": []interface{}{
						"daemonsets",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"apps",
					},
					"resources": []interface{}{
						"deployments",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"apps",
					},
					"resources": []interface{}{
						"statefulsets",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"authorization.k8s.io",
					},
					"resources": []interface{}{
						"subjectaccessreviews",
					},
					"verbs": []interface{}{
						"create",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"batch",
					},
					"resources": []interface{}{
						"cronjobs",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"certificaterequests",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"certificaterequests/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"certificaterequests/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"certificates",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"certificates/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"certificates/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"clusterissuers",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"clusterissuers/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"issuers",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"issuers/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cert-manager.io",
					},
					"resources": []interface{}{
						"signers",
					},
					"verbs": []interface{}{
						"approve",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"certificates.k8s.io",
					},
					"resources": []interface{}{
						"certificatesigningrequests",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"certificates.k8s.io",
					},
					"resources": []interface{}{
						"certificatesigningrequests/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"certificates.k8s.io",
					},
					"resources": []interface{}{
						"signers",
					},
					"verbs": []interface{}{
						"sign",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"cis.f5.com",
					},
					"resources": []interface{}{
						"ingresslinks",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongclusterplugins",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongclusterplugins/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongconsumers",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongconsumers/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongingresses",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongingresses/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongplugins",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"kongplugins/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"tcpingresses",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"tcpingresses/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"udpingresses",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"configuration.konghq.com",
					},
					"resources": []interface{}{
						"udpingresses/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"coordination.k8s.io",
					},
					"resources": []interface{}{
						"configmaps",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"coordination.k8s.io",
					},
					"resources": []interface{}{
						"leases",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"deployments",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"endpoints",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"endpoints/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"events",
					},
					"verbs": []interface{}{
						"create",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"leases",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"namespaces",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"persistentvolumeclaims",
					},
					"verbs": []interface{}{
						"delete",
						"get",
						"list",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"persistentvolumes",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"pods",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"pods/exec",
					},
					"verbs": []interface{}{
						"create",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"secrets",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"secrets/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"serviceaccounts",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"serviceaccounts/token",
					},
					"verbs": []interface{}{
						"create",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"services",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"services/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"extensions",
					},
					"resources": []interface{}{
						"daemonsets",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"extensions",
					},
					"resources": []interface{}{
						"deployments",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"extensions",
					},
					"resources": []interface{}{
						"ingresses",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"extensions",
					},
					"resources": []interface{}{
						"ingresses/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"clusterexternalsecrets",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"clusterexternalsecrets/finalizers",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"clusterexternalsecrets/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"clustersecretstores",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"clustersecretstores/finalizers",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"clustersecretstores/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"externalsecrets",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"externalsecrets/finalizers",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"externalsecrets/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"secretstores",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"deletecollection",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"secretstores/finalizers",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"external-secrets.io",
					},
					"resources": []interface{}{
						"secretstores/status",
					},
					"verbs": []interface{}{
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"externaldns.nginx.org",
					},
					"resources": []interface{}{
						"dnsendpoints",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"externaldns.nginx.org",
					},
					"resources": []interface{}{
						"dnsendpoints/status",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"gatewayclasses",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"gatewayclasses/status",
					},
					"verbs": []interface{}{
						"get",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"gateways",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"gateways/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"gateways/status",
					},
					"verbs": []interface{}{
						"get",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"httproutes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"httproutes/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"httproutes/status",
					},
					"verbs": []interface{}{
						"get",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"referencepolicies",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"referencepolicies/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"referencepolicies/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"tcproutes",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"tcproutes/status",
					},
					"verbs": []interface{}{
						"get",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"tlsroutes",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"tlsroutes/status",
					},
					"verbs": []interface{}{
						"get",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"udproutes",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"udproutes/status",
					},
					"verbs": []interface{}{
						"get",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"dnsendpoints/status",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"globalconfigurations",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"policies",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"policies/status",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"transportservers",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"transportservers/status",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"virtualserverroutes",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"virtualserverroutes/status",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"virtualservers",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"k8s.nginx.org",
					},
					"resources": []interface{}{
						"virtualservers/status",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"networking.internal.knative.dev",
					},
					"resources": []interface{}{
						"ingresses",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"networking.internal.knative.dev",
					},
					"resources": []interface{}{
						"ingresses/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"networking.k8s.io",
					},
					"resources": []interface{}{
						"ingressclasses",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"networking.k8s.io",
					},
					"resources": []interface{}{
						"ingresses",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"networking.k8s.io",
					},
					"resources": []interface{}{
						"ingresses/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"networking.k8s.io",
					},
					"resources": []interface{}{
						"ingresses/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"platform.addons.nukleros.io",
					},
					"resources": []interface{}{
						"certificatescomponents",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"platform.addons.nukleros.io",
					},
					"resources": []interface{}{
						"certificatescomponents/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"platform.addons.nukleros.io",
					},
					"resources": []interface{}{
						"ingresscomponents",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"platform.addons.nukleros.io",
					},
					"resources": []interface{}{
						"ingresscomponents/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"platform.addons.nukleros.io",
					},
					"resources": []interface{}{
						"secretscomponents",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"platform.addons.nukleros.io",
					},
					"resources": []interface{}{
						"secretscomponents/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"policy",
					},
					"resources": []interface{}{
						"poddisruptionbudgets",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"rbac.authorization.k8s.io",
					},
					"resources": []interface{}{
						"clusterrolebindings",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"rbac.authorization.k8s.io",
					},
					"resources": []interface{}{
						"clusterroles",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"rbac.authorization.k8s.io",
					},
					"resources": []interface{}{
						"rolebindings",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"rbac.authorization.k8s.io",
					},
					"resources": []interface{}{
						"roles",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"route.openshift.io",
					},
					"resources": []interface{}{
						"routes/custom-host",
					},
					"verbs": []interface{}{
						"create",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"setup.addons.nukleros.io",
					},
					"resources": []interface{}{
						"supportservices",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"setup.addons.nukleros.io",
					},
					"resources": []interface{}{
						"supportservices/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(managerRole, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services manager role: %w", err)
	}

	var metricsRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "support-services-operator-metrics-reader",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"nonResourceURLs": []interface{}{
						"/metrics",
					},
					"verbs": []interface{}{
						"get",
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(metricsRole, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services metrics role: %w", err)
	}

	var proxyRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "support-services-operator-proxy-role",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"authentication.k8s.io",
					},
					"resources": []interface{}{
						"tokenreviews",
					},
					"verbs": []interface{}{
						"create",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"authorization.k8s.io",
					},
					"resources": []interface{}{
						"subjectaccessreviews",
					},
					"verbs": []interface{}{
						"create",
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(proxyRole, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services proxy role: %w", err)
	}

	var leaderElectionRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      "support-services-operator-leader-election-rolebinding",
				"namespace": SupportServicesNamespace,
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "support-services-operator-leader-election-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "support-services-operator-controller-manager",
					"namespace": SupportServicesNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(leaderElectionRoleBinding, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services leader election role binding: %w", err)
	}

	var managerRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "support-services-operator-manager-rolebinding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "support-services-operator-manager-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "support-services-operator-controller-manager",
					"namespace": SupportServicesNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(managerRoleBinding, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services manager role binding: %w", err)
	}

	var proxyRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "support-services-operator-proxy-rolebinding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "support-services-operator-proxy-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "support-services-operator-controller-manager",
					"namespace": SupportServicesNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(proxyRoleBinding, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services proxy role binding: %w", err)
	}

	var managerConfig = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "support-services-operator-manager-config",
				"namespace": SupportServicesNamespace,
			},
			"data": map[string]interface{}{
				"controller_manager_config.yaml": `apiVersion: controller-runtime.sigs.k8s.io/v1alpha1
	kind: ControllerManagerConfig
	health:
	  healthProbeBindAddress: :8081
	metrics:
	  bindAddress: 127.0.0.1:8080
	webhook:
	  port: 9443
	leaderElection:
	  leaderElect: true
	  resourceName: bb9cd6ef.addons.nukleros.io
	`,
			},
		},
	}
	if _, err := kube.CreateResource(managerConfig, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services manager configmap: %w", err)
	}

	var deployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"control-plane": "controller-manager",
				},
				"name":      "support-services-operator-controller-manager",
				"namespace": SupportServicesNamespace,
			},
			"spec": map[string]interface{}{
				"progressDeadlineSeconds": 600,
				"replicas":                1,
				"revisionHistoryLimit":    10,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"control-plane": "controller-manager",
					},
				},
				"strategy": map[string]interface{}{
					"rollingUpdate": map[string]interface{}{
						"maxSurge":       "25%",
						"maxUnavailable": "25%",
					},
					"type": "RollingUpdate",
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"creationTimestamp": nil,
						"labels": map[string]interface{}{
							"control-plane": "controller-manager",
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"args": []interface{}{
									"--secure-listen-address=0.0.0.0:8443",
									"--upstream=http://127.0.0.1:8080/",
									"--logtostderr=true",
									"--v=10",
								},
								"image":           RBACProxyImage,
								"imagePullPolicy": "IfNotPresent",
								"name":            "kube-rbac-proxy",
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 8443,
										"name":          "https",
										"protocol":      "TCP",
									},
								},
								"resources":                map[string]interface{}{},
								"terminationMessagePath":   "/dev/termination-log",
								"terminationMessagePolicy": "File",
							},
							map[string]interface{}{
								"args": []interface{}{
									"--health-probe-bind-address=:8081",
									"--metrics-bind-address=127.0.0.1:8080",
									"--leader-elect",
								},
								"command": []interface{}{
									"/manager",
								},
								"image":           SupportServicesOperatorImage,
								"imagePullPolicy": "IfNotPresent",
								"livenessProbe": map[string]interface{}{
									"failureThreshold": 3,
									"httpGet": map[string]interface{}{
										"path":   "/healthz",
										"port":   8081,
										"scheme": "HTTP",
									},
									"initialDelaySeconds": 15,
									"periodSeconds":       20,
									"successThreshold":    1,
									"timeoutSeconds":      1,
								},
								"name": "manager",
								"readinessProbe": map[string]interface{}{
									"failureThreshold": 3,
									"httpGet": map[string]interface{}{
										"path":   "/readyz",
										"port":   8081,
										"scheme": "HTTP",
									},
									"initialDelaySeconds": 5,
									"periodSeconds":       10,
									"successThreshold":    1,
									"timeoutSeconds":      1,
								},
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "100m",
										"memory": "30Mi",
									},
									"requests": map[string]interface{}{
										"cpu":    "100m",
										"memory": "20Mi",
									},
								},
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
								},
								"terminationMessagePath":   "/dev/termination-log",
								"terminationMessagePolicy": "File",
							},
						},
						"dnsPolicy":     "ClusterFirst",
						"restartPolicy": "Always",
						"schedulerName": "default-scheduler",
						"securityContext": map[string]interface{}{
							"runAsNonRoot": true,
							"fsGroup":      2000,
							"runAsUser":    1000,
						},
						"serviceAccount":                "support-services-operator-controller-manager",
						"serviceAccountName":            "support-services-operator-controller-manager",
						"terminationGracePeriodSeconds": 10,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(deployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services deployment: %w", err)
	}

	var service = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"control-plane": "controller-manager",
				},
				"name":      "support-services-operator-controller-manager-metrics-service",
				"namespace": SupportServicesNamespace,
			},
			"spec": map[string]interface{}{
				"internalTrafficPolicy": "Cluster",
				"ipFamilies": []interface{}{
					"IPv4",
				},
				"ipFamilyPolicy": "SingleStack",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "https",
						"port":       8443,
						"protocol":   "TCP",
						"targetPort": "https",
					},
				},
				"selector": map[string]interface{}{
					"control-plane": "controller-manager",
				},
				"sessionAffinity": "None",
				"type":            "ClusterIP",
			},
		},
	}
	if _, err := kube.CreateResource(service, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services service: %w", err)
	}

	var setup = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "setup.addons.nukleros.io/v1alpha1",
			"kind":       "SupportServices",
			"metadata": map[string]interface{}{
				"name": "threeport-support-services",
			},
			"spec": map[string]interface{}{
				"tier": "development",
			},
		},
	}
	if _, err := kube.CreateResource(setup, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services setup resource: %w", err)
	}

	var certsComponent = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "platform.addons.nukleros.io/v1alpha1",
			"kind":       "CertificatesComponent",
			"metadata": map[string]interface{}{
				"name": "threeport-control-plane-certs",
			},
			"spec": map[string]interface{}{
				"namespace": "threeport-certs",
				"certManager": map[string]interface{}{
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
					"contactEmail": adminEmail,
				},
			},
		},
	}
	if _, err := kube.CreateResource(certsComponent, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services certs component: %w", err)
	}

	return nil
}

// InstallThreeportSystemServices installs system services that do not directly
// service tenant workload such as cluster autoscaler.  Installed only on
// clusters using eks provider.
func InstallThreeportSystemServices(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	infraProvider string,
	clusterName string,
) error {
	if infraProvider == ControlPlaneInfraProviderEKS {
		var clusterAutoscalerServiceAcct = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ServiceAccount",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"k8s-addon": "cluster-autoscaler.addons.k8s.io",
						"k8s-app":   "cluster-autoscaler",
					},
					"name":      "cluster-autoscaler",
					"namespace": "kube-system",
				},
			},
		}
		if _, err := kube.CreateResource(clusterAutoscalerServiceAcct, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create cluster autoscaler service account: %w", err)
		}

		var clusterAutoscalerClusterRole = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRole",
				"metadata": map[string]interface{}{
					"name": "cluster-autoscaler",
					"labels": map[string]interface{}{
						"k8s-addon": "cluster-autoscaler.addons.k8s.io",
						"k8s-app":   "cluster-autoscaler",
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"events",
							"endpoints",
						},
						"verbs": []interface{}{
							"create",
							"patch",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"pods/eviction",
						},
						"verbs": []interface{}{
							"create",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"pods/status",
						},
						"verbs": []interface{}{
							"update",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"endpoints",
						},
						"resourceNames": []interface{}{
							"cluster-autoscaler",
						},
						"verbs": []interface{}{
							"get",
							"update",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"nodes",
						},
						"verbs": []interface{}{
							"watch",
							"list",
							"get",
							"update",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"namespaces",
							"pods",
							"services",
							"replicationcontrollers",
							"persistentvolumeclaims",
							"persistentvolumes",
						},
						"verbs": []interface{}{
							"watch",
							"list",
							"get",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"extensions",
						},
						"resources": []interface{}{
							"replicasets",
							"daemonsets",
						},
						"verbs": []interface{}{
							"watch",
							"list",
							"get",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"policy",
						},
						"resources": []interface{}{
							"poddisruptionbudgets",
						},
						"verbs": []interface{}{
							"watch",
							"list",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"apps",
						},
						"resources": []interface{}{
							"statefulsets",
							"replicasets",
							"daemonsets",
						},
						"verbs": []interface{}{
							"watch",
							"list",
							"get",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"storage.k8s.io",
						},
						"resources": []interface{}{
							"storageclasses",
							"csinodes",
							"csidrivers",
							"csistoragecapacities",
						},
						"verbs": []interface{}{
							"watch",
							"list",
							"get",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"batch",
							"extensions",
						},
						"resources": []interface{}{
							"jobs",
						},
						"verbs": []interface{}{
							"get",
							"list",
							"watch",
							"patch",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"coordination.k8s.io",
						},
						"resources": []interface{}{
							"leases",
						},
						"verbs": []interface{}{
							"create",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"coordination.k8s.io",
						},
						"resourceNames": []interface{}{
							"cluster-autoscaler",
						},
						"resources": []interface{}{
							"leases",
						},
						"verbs": []interface{}{
							"get",
							"update",
						},
					},
				},
			},
		}
		if _, err := kube.CreateResource(clusterAutoscalerClusterRole, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create cluster autoscaler cluster role: %w", err)
		}

		var clusterAutoscalerRole = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "Role",
				"metadata": map[string]interface{}{
					"name":      "cluster-autoscaler",
					"namespace": "kube-system",
					"labels": map[string]interface{}{
						"k8s-addon": "cluster-autoscaler.addons.k8s.io",
						"k8s-app":   "cluster-autoscaler",
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"configmaps",
						},
						"verbs": []interface{}{
							"create",
							"list",
							"watch",
						},
					},
					map[string]interface{}{
						"apiGroups": []interface{}{
							"",
						},
						"resources": []interface{}{
							"configmaps",
						},
						"resourceNames": []interface{}{
							"cluster-autoscaler-status",
							"cluster-autoscaler-priority-expander",
						},
						"verbs": []interface{}{
							"delete",
							"get",
							"update",
							"watch",
						},
					},
				},
			},
		}
		if _, err := kube.CreateResource(clusterAutoscalerRole, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create cluster autoscaler role: %w", err)
		}

		var clusterAutoscalerClusterRoleBinding = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRoleBinding",
				"metadata": map[string]interface{}{
					"name": "cluster-autoscaler",
					"labels": map[string]interface{}{
						"k8s-addon": "cluster-autoscaler.addons.k8s.io",
						"k8s-app":   "cluster-autoscaler",
					},
				},
				"roleRef": map[string]interface{}{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "ClusterRole",
					"name":     "cluster-autoscaler",
				},
				"subjects": []interface{}{
					map[string]interface{}{
						"kind":      "ServiceAccount",
						"name":      "cluster-autoscaler",
						"namespace": "kube-system",
					},
				},
			},
		}
		if _, err := kube.CreateResource(clusterAutoscalerClusterRoleBinding, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create cluster autoscaler cluster role binding: %w", err)
		}

		var clusterAutoscalerRoleBinding = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "RoleBinding",
				"metadata": map[string]interface{}{
					"name":      "cluster-autoscaler",
					"namespace": "kube-system",
					"labels": map[string]interface{}{
						"k8s-addon": "cluster-autoscaler.addons.k8s.io",
						"k8s-app":   "cluster-autoscaler",
					},
				},
				"roleRef": map[string]interface{}{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "Role",
					"name":     "cluster-autoscaler",
				},
				"subjects": []interface{}{
					map[string]interface{}{
						"kind":      "ServiceAccount",
						"name":      "cluster-autoscaler",
						"namespace": "kube-system",
					},
				},
			},
		}
		if _, err := kube.CreateResource(clusterAutoscalerRoleBinding, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create cluster autoscaler role binding: %w", err)
		}

		var clusterAutoscalerDeployment = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "cluster-autoscaler",
					"namespace": "kube-system",
					"labels": map[string]interface{}{
						"app": "cluster-autoscaler",
					},
				},
				"spec": map[string]interface{}{
					"replicas": 1,
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": "cluster-autoscaler",
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": "cluster-autoscaler",
							},
							"annotations": map[string]interface{}{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "8085",
							},
						},
						"spec": map[string]interface{}{
							"priorityClassName": "system-cluster-critical",
							"securityContext": map[string]interface{}{
								"runAsNonRoot": true,
								"runAsUser":    65534,
								"fsGroup":      65534,
								"seccompProfile": map[string]interface{}{
									"type": "RuntimeDefault",
								},
							},
							"serviceAccountName": "cluster-autoscaler",
							"containers": []interface{}{
								map[string]interface{}{
									"image": "registry.k8s.io/autoscaling/cluster-autoscaler:v1.26.2",
									"name":  "cluster-autoscaler",
									"resources": map[string]interface{}{
										"limits": map[string]interface{}{
											"cpu":    "100m",
											"memory": "600Mi",
										},
										"requests": map[string]interface{}{
											"cpu":    "100m",
											"memory": "600Mi",
										},
									},
									"command": []interface{}{
										"./cluster-autoscaler",
										"--v=4",
										"--stderrthreshold=info",
										"--cloud-provider=aws",
										"--skip-nodes-with-local-storage=false",
										"--expander=least-waste",
										fmt.Sprintf("--node-group-auto-discovery=asg:tag=k8s.io/cluster-autoscaler/enabled,k8s.io/cluster-autoscaler/%s", clusterName),
									},
									"volumeMounts": []interface{}{
										map[string]interface{}{
											"name":      "ssl-certs",
											"mountPath": "/etc/ssl/certs/ca-certificates.crt", // /etc/ssl/certs/ca-bundle.crt for Amazon Linux Worker Nodes
											"readOnly":  true,
										},
									},
									"imagePullPolicy": "Always",
									"securityContext": map[string]interface{}{
										"allowPrivilegeEscalation": false,
										"capabilities": map[string]interface{}{
											"drop": []interface{}{
												"ALL",
											},
										},
										"readOnlyRootFilesystem": true,
									},
								},
							},
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "ssl-certs",
									"hostPath": map[string]interface{}{
										"path": "/etc/ssl/certs/ca-bundle.crt",
									},
								},
							},
						},
					},
				},
			},
		}
		if _, err := kube.CreateResource(clusterAutoscalerDeployment, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create cluster autoscaler deployment: %w", err)
		}
	}

	return nil
}
