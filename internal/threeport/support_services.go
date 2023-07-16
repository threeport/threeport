package threeport

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/kube"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const (
	SupportServicesNamespace = "support-services-system"
	SupportServicesOperatorImage = "ghcr.io/nukleros/support-services-operator:v0.2.0"
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
	var certManagerCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "certmanagers.certificates.support-services.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "certificates.support-services.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "CertManager",
					"listKind": "CertManagerList",
					"plural":   "certmanagers",
					"singular": "certmanager",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "CertManager is the Schema for the certmanagers API.",
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
										"description": "CertManagerSpec defines the desired state of CertManager.",
										"properties": map[string]interface{}{
											"cainjector": map[string]interface{}{
												"properties": map[string]interface{}{
													"image": map[string]interface{}{
														"default":     "quay.io/jetstack/cert-manager-cainjector",
														"description": "(Default: \"quay.io/jetstack/cert-manager-cainjector\") Image repo and name to use for cert-manager cainjector.",
														"type":        "string",
													},
													"replicas": map[string]interface{}{
														"default":     2,
														"description": "(Default: 2) Number of replicas to use for the cert-manager cainjector deployment.",
														"type":        "integer",
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
											"contactEmail": map[string]interface{}{
												"description": "Contact e-mail address for receiving updates about certificates from LetsEncrypt.",
												"type":        "string",
											},
											"controller": map[string]interface{}{
												"properties": map[string]interface{}{
													"image": map[string]interface{}{
														"default":     "quay.io/jetstack/cert-manager-controller",
														"description": "(Default: \"quay.io/jetstack/cert-manager-controller\") Image repo and name to use for cert-manager controller.",
														"type":        "string",
													},
													"replicas": map[string]interface{}{
														"default":     2,
														"description": "(Default: 2) Number of replicas to use for the cert-manager controller deployment.",
														"type":        "integer",
													},
												},
												"type": "object",
											},
											"namespace": map[string]interface{}{
												"default":     "nukleros-certs-system",
												"description": "(Default: \"nukleros-certs-system\") Namespace to use for certificate support services.",
												"type":        "string",
											},
											"version": map[string]interface{}{
												"default":     "v1.9.1",
												"description": "(Default: \"v1.9.1\") Version of cert-manager to use.",
												"type":        "string",
											},
											"webhook": map[string]interface{}{
												"properties": map[string]interface{}{
													"image": map[string]interface{}{
														"default":     "quay.io/jetstack/cert-manager-webhook",
														"description": "(Default: \"quay.io/jetstack/cert-manager-webhook\") Image repo and name to use for cert-manager webhook.",
														"type":        "string",
													},
													"replicas": map[string]interface{}{
														"default":     2,
														"description": "(Default: 2) Number of replicas to use for the cert-manager webhook deployment.",
														"type":        "integer",
													},
												},
												"type": "object",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "CertManagerStatus defines the observed state of CertManager.",
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
	if _, err := kube.CreateResource(certManagerCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create cert manager crd: %w", err)
	}

	var externalDNSCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "externaldns.gateway.support-services.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "gateway.support-services.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "ExternalDNS",
					"listKind": "ExternalDNSList",
					"plural":   "externaldns",
					"singular": "externaldns",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "ExternalDNS is the Schema for the externaldns API.",
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
										"description": "ExternalDNSSpec defines the desired state of ExternalDNS.",
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
											"iamRoleArn": map[string]interface{}{
												"description": "On AWS, the IAM Role ARN that gives external-dns access to Route53",
												"type":        "string",
											},
											"image": map[string]interface{}{
												"default":     "k8s.gcr.io/external-dns/external-dns",
												"description": "(Default: \"k8s.gcr.io/external-dns/external-dns\") Image repo and name to use for external-dns.",
												"type":        "string",
											},
											"namespace": map[string]interface{}{
												"default":     "nukleros-ingress-system",
												"description": "(Default: \"nukleros-ingress-system\") Namespace to use for ingress support services.",
												"type":        "string",
											},
											"provider": map[string]interface{}{
												"default":     "none",
												"description": "(Default: \"none\") The DNS provider to use for setting DNS records with external-dns.  One of: none | active-directory | google | route53.",
												"enum": []interface{}{
													"none",
													"active-directory",
													"google",
													"route53",
												},
												"type": "string",
											},
											"serviceAccountName": map[string]interface{}{
												"default":     "external-dns",
												"description": "(Default: \"external-dns\") The name of the external-dns service account which is referenced in role policy doc for AWS.",
												"type":        "string",
											},
											"version": map[string]interface{}{
												"default":     "v0.12.2",
												"description": "(Default: \"v0.12.2\") Version of external-dns to use.",
												"type":        "string",
											},
											"zoneType": map[string]interface{}{
												"default":     "private",
												"description": "(Default: \"private\") Type of DNS hosted zone to manage.",
												"enum": []interface{}{
													"private",
													"public",
												},
												"type": "string",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "ExternalDNSStatus defines the observed state of ExternalDNS.",
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
	if _, err := kube.CreateResource(externalDNSCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create external dns crd: %w", err)
	}

	var glooEdgeCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "glooedges.gateway.support-services.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "gateway.support-services.nukleros.io",
				"names": map[string]interface{}{
					"kind":     "GlooEdge",
					"listKind": "GlooEdgeList",
					"plural":   "glooedges",
					"singular": "glooedge",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "GlooEdge is the Schema for the glooedges API.",
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
										"description": "GlooEdgeSpec defines the desired state of GlooEdge.",
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
											"namespace": map[string]interface{}{
												"default":     "nukleros-gateway-system",
												"description": "(Default: \"nukleros-gateway-system\") Namespace to use for gateway support services.",
												"type":        "string",
											},
											"ports": map[string]interface{}{
												"items": map[string]interface{}{
													"properties": map[string]interface{}{
														"name": map[string]interface{}{
															"type": "string",
														},
														"port": map[string]interface{}{
															"format": "int64",
															"type":   "integer",
														},
														"ssl": map[string]interface{}{
															"type": "boolean",
														},
													},
													"type": "object",
												},
												"type": "array",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "GlooEdgeStatus defines the observed state of GlooEdge.",
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
	if _, err := kube.CreateResource(glooEdgeCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create gloo edge crd: %w", err)
	}

	var supportServicesCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.9.0",
				},
				"creationTimestamp": nil,
				"name":              "supportservices.orchestration.support-services.nukleros.io",
			},
			"spec": map[string]interface{}{
				"group": "orchestration.support-services.nukleros.io",
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
												"default":     "kong",
												"description": "(Default: \"kong\") The default ingress for setting TLS certs.  One of: kong | nginx.",
												"enum": []interface{}{
													"kong",
													"nginx",
												},
												"type": "string",
											},
											"tier": map[string]interface{}{
												"default":     "development",
												"description": "(Default: \"development\") The tier of cluster being used.  One of: development | staging | production.",
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
	if _, err := kube.CreateResource(supportServicesCRD, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create support services crd: %w", err)
	}

	return nil
}

// InstallThreeportSupportServicesOperator installs the support services operator
func InstallThreeportSupportServicesOperator(
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
					"app.kubernetes.io/component":  "manager",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "system",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "namespace",
					"app.kubernetes.io/part-of":    "support-services-operator",
					"control-plane":                "controller-manager",
				},
				"name": SupportServicesNamespace,
			},
		},
	}
	if _, err := kube.CreateResource(namespace, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	var serviceAccount = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kuberentes.io/instance":   "controller-manager",
					"app.kubernetes.io/component":  "rbac",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "serviceaccount",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
				"name":      "support-services-operator-controller-manager",
				"namespace": SupportServicesNamespace,
			},
		},
	}
	if _, err := kube.CreateResource(serviceAccount, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	var roleLeaderElection = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "rbac",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "leader-election-role",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "role",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
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
	if _, err := kube.CreateResource(roleLeaderElection, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create leader election role: %w", err)
	}

	var clusterRoleManager = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"creationTimestamp": nil,
				"name":              "support-services-operator-manager-role",
			},
			"rules": []interface{}{
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
						"certificates.support-services.nukleros.io",
					},
					"resources": []interface{}{
						"certmanagers",
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
						"certificates.support-services.nukleros.io",
					},
					"resources": []interface{}{
						"certmanagers/status",
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
						"leases",
					},
					"verbs": []interface{}{
						"*",
						"create",
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
						"configmaps",
					},
					"verbs": []interface{}{
						"*",
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
						"events",
					},
					"verbs": []interface{}{
						"create",
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
						"list",
						"watch",
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
						"watch",
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
						"enterprise.gloo.solo.io",
					},
					"resources": []interface{}{
						"authconfigs",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
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
						"gateway.networking.k8s.io",
					},
					"resources": []interface{}{
						"gateways",
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
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"*",
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
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"gateways",
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
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"httpgateways",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"routeoptions",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"routetables",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"tcpgateways",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"virtualhostoptions",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.solo.io",
					},
					"resources": []interface{}{
						"virtualservices",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.support-services.nukleros.io",
					},
					"resources": []interface{}{
						"externaldns",
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
						"gateway.support-services.nukleros.io",
					},
					"resources": []interface{}{
						"externaldns/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gateway.support-services.nukleros.io",
					},
					"resources": []interface{}{
						"glooedges",
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
						"gateway.support-services.nukleros.io",
					},
					"resources": []interface{}{
						"glooedges/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gloo.solo.io",
					},
					"resources": []interface{}{
						"*",
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
						"gloo.solo.io",
					},
					"resources": []interface{}{
						"proxies",
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
						"gloo.solo.io",
					},
					"resources": []interface{}{
						"settings",
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
						"gloo.solo.io",
					},
					"resources": []interface{}{
						"upstreamgroups",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"patch",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"gloo.solo.io",
					},
					"resources": []interface{}{
						"upstreams",
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
						"graphql.gloo.solo.io",
					},
					"resources": []interface{}{
						"graphqlapis",
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
						"graphql.gloo.solo.io",
					},
					"resources": []interface{}{
						"graphqlapis/status",
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
						"orchestration.support-services.nukleros.io",
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
						"orchestration.support-services.nukleros.io",
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
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ratelimit.solo.io",
					},
					"resources": []interface{}{
						"ratelimitconfigs",
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
						"ratelimit.solo.io",
					},
					"resources": []interface{}{
						"ratelimitconfigs/status",
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
			},
		},
	}
	if _, err := kube.CreateResource(clusterRoleManager, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create manager cluster role: %w", err)
	}

	var clusterRoleMetricsReader = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "kube-rbac-proxy",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "metrics-reader",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "clusterrole",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
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
	if _, err := kube.CreateResource(clusterRoleMetricsReader, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create metrics reader cluster role: %w", err)
	}

	var clusterRoleProxy = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "kube-rbac-proxy",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "proxy-role",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "clusterrole",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
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
	if _, err := kube.CreateResource(clusterRoleProxy, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create proxy cluster role: %w", err)
	}

	var roleBindingLeaderElection = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "rbac",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "leader-election-rolebinding",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "rolebinding",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
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
	if _, err := kube.CreateResource(roleBindingLeaderElection, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create leader election role binding: %w", err)
	}

	var roleBindingManager = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "rbac",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "manager-rolebinding",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "clusterrolebinding",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
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
	if _, err := kube.CreateResource(roleBindingManager, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create manager role binding: %w", err)
	}

	var clusterRoleBindingProxy = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "kube-rbac-proxy",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "proxy-rolebinding",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "clusterrolebinding",
					"app.kubernetes.io/part-of":    "support-services-operator",
				},
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
	if _, err := kube.CreateResource(clusterRoleBindingProxy, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create proxy cluster role binding: %w", err)
	}

	var serviceMetrics = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "kube-rbac-proxy",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "controller-manager-metrics-service",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "service",
					"app.kubernetes.io/part-of":    "support-services-operator",
					"control-plane":                "controller-manager",
				},
				"name":      "support-services-operator-controller-manager-metrics-service",
				"namespace": SupportServicesNamespace,
			},
			"spec": map[string]interface{}{
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
			},
		},
	}
	if _, err := kube.CreateResource(serviceMetrics, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create metrics service: %w", err)
	}

	var deployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/component":  "manager",
					"app.kubernetes.io/created-by": "support-services-operator",
					"app.kubernetes.io/instance":   "controller-manager",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/name":       "deployment",
					"app.kubernetes.io/part-of":    "support-services-operator",
					"control-plane":                "controller-manager",
				},
				"name":      "support-services-operator-controller-manager",
				"namespace": SupportServicesNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"control-plane": "controller-manager",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/default-container": "manager",
						},
						"labels": map[string]interface{}{
							"control-plane": "controller-manager",
						},
					},
					"spec": map[string]interface{}{
						"affinity": map[string]interface{}{
							"nodeAffinity": map[string]interface{}{
								"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
									"nodeSelectorTerms": []interface{}{
										map[string]interface{}{
											"matchExpressions": []interface{}{
												map[string]interface{}{
													"key":      "kubernetes.io/arch",
													"operator": "In",
													"values": []interface{}{
														"amd64",
														"arm64",
														"ppc64le",
														"s390x",
													},
												},
												map[string]interface{}{
													"key":      "kubernetes.io/os",
													"operator": "In",
													"values": []interface{}{
														"linux",
													},
												},
											},
										},
									},
								},
							},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"args": []interface{}{
									"--secure-listen-address=0.0.0.0:8443",
									"--upstream=http://127.0.0.1:8080/",
									"--logtostderr=true",
									"--v=0",
								},
								"image": RBACProxyImage,
								"name":  "kube-rbac-proxy",
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 8443,
										"name":          "https",
										"protocol":      "TCP",
									},
								},
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "500m",
										"memory": "128Mi",
									},
									"requests": map[string]interface{}{
										"cpu":    "5m",
										"memory": "64Mi",
									},
								},
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
									"capabilities": map[string]interface{}{
										"drop": []interface{}{
											"ALL",
										},
									},
								},
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
								"image": SupportServicesOperatorImage,
								"livenessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/healthz",
										"port": 8081,
									},
									"initialDelaySeconds": 15,
									"periodSeconds":       20,
								},
								"name": "manager",
								"readinessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/readyz",
										"port": 8081,
									},
									"initialDelaySeconds": 5,
									"periodSeconds":       10,
								},
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "500m",
										"memory": "128Mi",
									},
									"requests": map[string]interface{}{
										"cpu":    "10m",
										"memory": "64Mi",
									},
								},
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
									"capabilities": map[string]interface{}{
										"drop": []interface{}{
											"ALL",
										},
									},
								},
							},
						},
						"securityContext": map[string]interface{}{
							"runAsNonRoot": true,
						},
						"serviceAccountName":            "support-services-operator-controller-manager",
						"terminationGracePeriodSeconds": 10,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(deployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
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
	if infraProvider == v0.KubernetesRuntimeInfraProviderEKS {
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
