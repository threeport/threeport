package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// installEksClusterAutoscaler installs the cluster autoscaler RBAC and deployment.
func installEksClusterAutoscaler(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	clusterName string,
	accountId string,
) error {
	// create the service account
	var clusterAutoscalerServiceAcct = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"k8s-addon": "cluster-autoscaler.addons.k8s.io",
					"k8s-app":   "cluster-autoscaler",
				},
				"name":      ClusterAutoscalerServiceAccountName,
				"namespace": ClusterAutoscalerNamespace,
				"annotations": map[string]interface{}{
					"eks.amazonaws.com/role-arn": tpaws.EksClusterAutoscalingRoleArn(
						accountId,
						clusterName,
					),
				},
			},
		},
	}
	if _, err := kube.CreateResource(clusterAutoscalerServiceAcct, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create cluster autoscaler service account: %w", err)
	}

	// create the cluster role
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

	// create the role
	var clusterAutoscalerRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      "cluster-autoscaler",
				"namespace": ClusterAutoscalerNamespace,
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

	// bind the cluster role
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
					"namespace": ClusterAutoscalerNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(clusterAutoscalerClusterRoleBinding, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create cluster autoscaler cluster role binding: %w", err)
	}

	// bind the role
	var clusterAutoscalerRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      "cluster-autoscaler",
				"namespace": ClusterAutoscalerNamespace,
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
					"namespace": ClusterAutoscalerNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(clusterAutoscalerRoleBinding, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create cluster autoscaler role binding: %w", err)
	}

	// create the deployment
	var clusterAutoscalerDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      ClusterAutoscalerDeployName,
				"namespace": ClusterAutoscalerNamespace,
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

	return nil
}
