package v0

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/version"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ThreeportDevImages returns a map of main package dirs to dev image names
func (cpi *ControlPlaneInstaller) ThreeportDevImages() map[string]string {
	devImageSuffix := "-dev"
	devImages := make(map[string]string)

	for _, c := range cpi.Opts.ControllerList {
		devImages[c.Name] = fmt.Sprintf("%s%s:latest", c.ImageName, devImageSuffix)
	}

	devImages[cpi.Opts.RestApiInfo.Name] = fmt.Sprintf("%s%s:latest", cpi.Opts.RestApiInfo.ImageName, devImageSuffix)
	devImages[cpi.Opts.AgentInfo.Name] = fmt.Sprintf("%s%s:latest", cpi.Opts.AgentInfo.ImageName, devImageSuffix)

	return devImages
}

// InstallComputeSpaceControlPlaneComponents
func (cpi *ControlPlaneInstaller) InstallComputeSpaceControlPlaneComponents(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	runtimeInstanceName string,
) error {
	// threeport control plane namespace
	if err := cpi.CreateThreeportControlPlaneNamespace(
		kubeClient,
		mapper,
	); err != nil {
		return fmt.Errorf("failed to create threeport control plane namespace: %w", err)
	}

	if err := cpi.InstallThreeportAgent(
		kubeClient,
		mapper,
		runtimeInstanceName,
		nil,
	); err != nil {
		return fmt.Errorf("failed to install threeport agent: %w", err)
	}

	// threeport CRDs
	if err := InstallThreeportCRDs(kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to install threeport CRDs: %w", err)
	}

	// support services operator
	if err := InstallThreeportSupportServicesOperator(kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to install support services operator: %w", err)
	}
	return nil
}

// InstallThreeportControlPlaneAPI installs the threeport API in a Kubernetes
// cluster.
func (cpi *ControlPlaneInstaller) UpdateThreeportAPIDeployment(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	liveReload,
	isAuthEnabled bool,
	infraProvider string,
	encryptionKey string,
) error {
	apiImage := cpi.getImage(liveReload, cpi.Opts.RestApiInfo.Name, cpi.Opts.RestApiInfo.ImageName, cpi.Opts.RestApiInfo.ImageRepo, cpi.Opts.RestApiInfo.ImageTag)
	apiArgs := cpi.getAPIArgs(liveReload, isAuthEnabled)
	apiVols, apiVolMounts := cpi.getAPIVolumes(liveReload, isAuthEnabled)
	apiServiceType := cpi.getAPIServiceType(infraProvider)
	apiServiceAnnotations := getAPIServiceAnnotations(infraProvider)
	apiServicePortName, apiServicePort := cpi.getAPIServicePort(infraProvider, isAuthEnabled)
	apiImagePullSecrets := cpi.getImagePullSecrets(cpi.Opts.RestApiInfo.ImagePullSecretName)

	var dbCreateConfig = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "db-create",
				"namespace": cpi.Opts.Namespace,
			},
			"data": map[string]interface{}{
				"db.sql": `CREATE USER IF NOT EXISTS tp_rest_api
  LOGIN
;
CREATE DATABASE IF NOT EXISTS threeport_api
    encoding='utf-8'
;
GRANT ALL ON DATABASE threeport_api TO tp_rest_api;
`,
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(dbCreateConfig, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create DB configmap: %w", err)
	}

	var apiSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "db-config",
				"namespace": cpi.Opts.Namespace,
			},
			"stringData": map[string]interface{}{
				"env": `DB_HOST=crdb
DB_USER=tp_rest_api
DB_PASSWORD=tp-rest-api-pwd
DB_NAME=threeport_api
DB_PORT=26257
DB_SSL_MODE=disable
NATS_HOST=nats-js
NATS_PORT=4222
`,
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(apiSecret, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var encryptionSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "encryption-key",
				"namespace": cpi.Opts.Namespace,
			},
			"stringData": map[string]interface{}{
				"ENCRYPTION_KEY": encryptionKey,
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(encryptionSecret, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create API server secret: %w", err)
	}

	var apiDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      cpi.Opts.RestApiInfo.ServiceResourceName,
				"namespace": cpi.Opts.Namespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": cpi.Opts.RestApiInfo.ServiceResourceName,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": cpi.Opts.RestApiInfo.ServiceResourceName,
						},
					},
					"spec": map[string]interface{}{
						"initContainers": []interface{}{
							map[string]interface{}{
								"name":            "db-init",
								"image":           "cockroachdb/cockroach:v22.2.2",
								"imagePullPolicy": "IfNotPresent",
								"command": []interface{}{
									"bash",
									"-c",
									//- "cockroach sql --insecure --host crdb --port 26257 -f /etc/threeport/db-create/db.sql && cockroach sql --insecure --host crdb --port 26257 --database threeport_api -f /etc/threeport/db-load/create_tables.sql && cockroach sql --insecure --host crdb --port 26257 --database threeport_api -f /etc/threeport/db-load/fill_tables.sql"
									"cockroach sql --insecure --host crdb --port 26257 -f /etc/threeport/db-create/db.sql",
								},
								"volumeMounts": []interface{}{
									//- name: db-load
									//  mountPath: "/etc/threeport/db-load"
									map[string]interface{}{
										"name":      "db-create",
										"mountPath": "/etc/threeport/db-create",
									},
								},
							},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "api-server",
								"image":           apiImage,
								"command":         getCommand(cpi.Opts.RestApiInfo.Name, liveReload, false),
								"imagePullPolicy": "IfNotPresent",
								"args":            apiArgs,
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 1323,
										"name":          "api",
										"protocol":      "TCP",
									},
								},
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "encryption-key",
										},
									},
								},
								"volumeMounts":   apiVolMounts,
								"readinessProbe": cpi.getReadinessProbe(),
							},
						},
						"imagePullSecrets": apiImagePullSecrets,
						"volumes":          apiVols,
					},
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(apiDeployment, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create API server deployment: %w", err)
	}

	// configure node port based on infra provider
	port := map[string]interface{}{
		"name":       apiServicePortName,
		"port":       apiServicePort,
		"protocol":   "TCP",
		"targetPort": 1323,
	}
	if infraProvider == "kind" && !cpi.Opts.InThreeport {
		port["nodePort"] = 30000
	}
	var apiService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":        cpi.Opts.RestApiInfo.ServiceResourceName,
				"namespace":   cpi.Opts.Namespace,
				"annotations": apiServiceAnnotations,
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app.kubernetes.io/name": cpi.Opts.RestApiInfo.ServiceResourceName,
				},
				"ports": []interface{}{
					port,
				},
				"type": apiServiceType,
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(apiService, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server service: %w", err)
	}

	return nil
}

// InstallThreeportAPITLS installs TLS assets for threeport API.
func (cpi *ControlPlaneInstaller) InstallThreeportAPITLS(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	authConfig *auth.AuthConfig,
	serverAltName string,
) error {
	if authConfig != nil {
		// generate server certificate
		serverCertificate, serverPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
			serverAltName,
		)
		if err != nil {
			return fmt.Errorf("failed to generate server certificate and private key: %w", err)
		}

		var apiCa = cpi.getTLSSecret("api-ca", authConfig.CAPemEncoded, authConfig.CAPrivateKeyPemEncoded)
		if err := cpi.CreateOrUpdateKubeResource(apiCa, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create API server ca secret: %w", err)
		}

		var apiCert = cpi.getTLSSecret("api-cert", serverCertificate, serverPrivateKey)
		if err := cpi.CreateOrUpdateKubeResource(apiCert, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create API server certificate secret: %w", err)
		}
	}

	return nil
}

// InstallThreeportControllers installs the threeport controllers in a
// Kubernetes cluster.
func (cpi *ControlPlaneInstaller) InstallThreeportControllers(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	authConfig *auth.AuthConfig,
) error {
	controllerSecret := cpi.getControllerSecret("controller", cpi.Opts.Namespace)
	if err := cpi.CreateOrUpdateKubeResource(controllerSecret, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create controller secret: %w", err)
	}

	for _, controller := range cpi.Opts.ControllerList {
		if !*controller.Enabled {
			continue
		}

		// if auth is enabled on API, generate client cert and key and store in
		// secrets
		if authConfig != nil {

			certificate, privateKey, err := auth.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
			if err != nil {
				return fmt.Errorf("failed to generate client certificate and private key for workload controller: %w", err)
			}

			ca := cpi.getTLSSecret(fmt.Sprintf("%s-ca", controller.Name), authConfig.CAPemEncoded, "")
			if err := cpi.CreateOrUpdateKubeResource(ca, kubeClient, mapper); err != nil {
				return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
			}

			cert := cpi.getTLSSecret(fmt.Sprintf("%s-cert", controller.Name), certificate, privateKey)
			if err := cpi.CreateOrUpdateKubeResource(cert, kubeClient, mapper); err != nil {
				return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
			}
		}

		if err := cpi.UpdateControllerDeployment(
			kubeClient,
			mapper,
			true,
			*controller,
			authConfig != nil,
		); err != nil {
			return fmt.Errorf("failed to install %s: %w", *&controller.Name, err)
		}
	}

	return nil
}

// CreateOrUpdateKubeResource creates or updates a Kubernetes resource.
func (cpi *ControlPlaneInstaller) CreateOrUpdateKubeResource(
	resource *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	if cpi.Opts.CreateOrUpdateKubeResources {
		if _, err := kube.CreateOrUpdateResource(resource, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create/update resource: %w", err)
		}
	} else {
		if _, err := kube.CreateResource(resource, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create resource: %w", err)
		}
	}
	return nil
}

// UpdateControllerDeployment installs a threeport controller by name.
func (cpi *ControlPlaneInstaller) UpdateControllerDeployment(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	liveReload bool,
	installInfo v0.ControlPlaneComponent,
	isAuthEnabled bool,
) error {
	controllerImage := cpi.getImage(liveReload, installInfo.Name, installInfo.ImageName, installInfo.ImageRepo, installInfo.ImageTag)
	controllerVols, controllerVolMounts := cpi.getControllerVolumes(installInfo.Name, liveReload, isAuthEnabled)
	controllerArgs := cpi.getControllerArgs(installInfo.Name, liveReload, cpi.Opts.Debug, isAuthEnabled)
	controllerImagePullSecrets := cpi.getImagePullSecrets(installInfo.ImagePullSecretName)

	var deployName string
	if cpi.isThreeportManagedController(installInfo) {
		deployName = fmt.Sprintf("threeport-%s", installInfo.Name)
	} else {
		deployName = fmt.Sprintf("%s-%s", cpi.Opts.Name, installInfo.Name)
	}

	serviceAccountName := installInfo.ServiceAccountName

	var controllerDeployment = cpi.getControllerDeployment(
		deployName,
		installInfo.Name,
		cpi.Opts.Namespace,
		serviceAccountName,
		controllerImage,
		controllerArgs,
		controllerVols,
		controllerVolMounts,
		controllerImagePullSecrets,
		liveReload,
		false,
	)
	if err := cpi.CreateOrUpdateKubeResource(controllerDeployment, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create workload controller deployment: %w", err)
	}
	return nil
}

// InstallThreeportAgent installs the threeport agent on a Kubernetes cluster.
func (cpi *ControlPlaneInstaller) InstallThreeportAgent(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	threeportInstanceName string,
	authConfig *auth.AuthConfig,
) error {

	// if auth is enabled on API, generate client cert and key and store in
	// secrets
	if authConfig != nil {
		agentCertificate, agentPrivateKey, err := auth.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
		if err != nil {
			return fmt.Errorf("failed to generate client certificate and private key for threeport agent: %w", err)
		}

		agentCa := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"type":       "Opaque",
				"metadata": map[string]interface{}{
					"name":      "agent-ca",
					"namespace": cpi.Opts.Namespace,
				},
				"stringData": map[string]interface{}{
					"tls.crt": authConfig.CAPemEncoded,
				},
			},
		}
		if err := cpi.CreateOrUpdateKubeResource(agentCa, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create/update API server ca secret for threeport agent: %w", err)
		}

		var agentCert = cpi.getTLSSecret("agent-cert", agentCertificate, agentPrivateKey)
		if err := cpi.CreateOrUpdateKubeResource(agentCert, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create/update API server certificate secret for threeport agent: %w", err)
		}
	}

	if err := cpi.UpdateThreeportAgentDeployment(
		kubeClient,
		mapper,
		threeportInstanceName,
		false,
		authConfig != nil,
	); err != nil {
		return fmt.Errorf("failed to update threeport agent deployment: %w", err)
	}
	return nil
}

func (cpi *ControlPlaneInstaller) UpdateThreeportAgentDeployment(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	threeportInstanceName string,
	liveReload,
	isAuthEnabled bool,
) error {

	agentImage := cpi.getImage(liveReload, cpi.Opts.AgentInfo.Name, cpi.Opts.AgentInfo.ImageName, cpi.Opts.AgentInfo.ImageRepo, cpi.Opts.AgentInfo.ImageTag)
	agentArgs := cpi.getAgentArgs(liveReload, isAuthEnabled)
	agentVols, agentVolMounts := cpi.getControllerVolumes("agent", liveReload, isAuthEnabled)
	agentImagePullSecrets := cpi.getImagePullSecrets(cpi.Opts.AgentInfo.ImagePullSecretName)

	var threeportAgentCRD = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"controller-gen.kubebuilder.io/version": "v0.11.3",
				},
				"creationTimestamp": nil,
				"name":              "threeportworkloads.control-plane.threeport.io",
			},
			"spec": map[string]interface{}{
				"group": "control-plane.threeport.io",
				"names": map[string]interface{}{
					"kind":     "ThreeportWorkload",
					"listKind": "ThreeportWorkloadList",
					"plural":   "threeportworkloads",
					"singular": "threeportworkload",
				},
				"scope": "Cluster",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "ThreeportWorkload is the Schema for the threeportworkloads API",
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
										"description": "ThreeportWorkloadSpec defines the desired state of ThreeportWorkload",
										"properties": map[string]interface{}{
											"workloadInstanceId": map[string]interface{}{
												"description": "WorkloadInstance is the unique ID for a threeport object that represents a deployed instance of a workload.",
												"type":        "integer",
											},
											"workloadResourceInstances": map[string]interface{}{
												"description": "WorkloadResources is a slice of WorkloadResource objects.",
												"items": map[string]interface{}{
													"description": "WorkloadResource is a Kubernetes resource that should be watched and reported upon by the threeport agent.",
													"properties": map[string]interface{}{
														"group": map[string]interface{}{
															"type": "string",
														},
														"kind": map[string]interface{}{
															"type": "string",
														},
														"name": map[string]interface{}{
															"type": "string",
														},
														"namespace": map[string]interface{}{
															"type": "string",
														},
														"threeportID": map[string]interface{}{
															"type": "integer",
														},
														"version": map[string]interface{}{
															"type": "string",
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
										"description": "ThreeportWorkloadStatus defines the observed state of ThreeportWorkload",
										"type":        "object",
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
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentCRD, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent CRD: %w", err)
	}

	var threeportAgentServiceAccount = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-controller-manager",
				"namespace": cpi.Opts.Namespace,
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentServiceAccount, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent service account: %w", err)
	}

	var threeportAgentLeaderElectionRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-leader-election-role",
				"namespace": cpi.Opts.Namespace,
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
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentLeaderElectionRole, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent leader election role: %w", err)
	}

	var threeportAgentManagerRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"creationTimestamp": nil,
				"name":              "threeport-agent-manager-role",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"control-plane.threeport.io",
					},
					"resources": []interface{}{
						"threeportworkloads",
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
						"control-plane.threeport.io",
					},
					"resources": []interface{}{
						"threeportworkloads/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"control-plane.threeport.io",
					},
					"resources": []interface{}{
						"threeportworkloads/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"*",
					},
					"resources": []interface{}{
						"*",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
					},
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentManagerRole, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent manager role: %w", err)
	}

	var threeportAgentMetricsReaderRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name": "threeport-agent-metrics-reader",
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
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentMetricsReaderRole, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent metrics reader role: %w", err)
	}

	var threeportAgentProxyRole = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name": "threeport-agent-proxy-role",
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
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentProxyRole, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent proxy role: %w", err)
	}

	var threeportAgentLeaderElectionRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-leader-election-rolebinding",
				"namespace": cpi.Opts.Namespace,
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "threeport-agent-leader-election-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "threeport-agent-controller-manager",
					"namespace": cpi.Opts.Namespace,
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentLeaderElectionRoleBinding, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent leader election role binding: %w", err)
	}

	var threeportAgentManagerRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name": "threeport-agent-manager-rolebinding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "threeport-agent-manager-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "threeport-agent-controller-manager",
					"namespace": cpi.Opts.Namespace,
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentManagerRoleBinding, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent manager role binding: %w", err)
	}

	var threeportAgentProxyRoleBinding = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name": "threeport-agent-proxy-rolebinding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "threeport-agent-proxy-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "threeport-agent-controller-manager",
					"namespace": cpi.Opts.Namespace,
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentProxyRoleBinding, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent proxy role binding: %w", err)
	}

	var threeportAgentControllerManagerMetricsService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-controller-manager-metrics-service",
				"namespace": cpi.Opts.Namespace,
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
					"app.kubernetes.io/name": "threeport-agent",
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentControllerManagerMetricsService, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent controller manager metrics service: %w", err)
	}

	var threeportAgentDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "threeport-agent",
					"app.kubernetes.io/instance":   "threeport-agent" + threeportInstanceName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      ThreeportAgentDeployName,
				"namespace": cpi.Opts.Namespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": "threeport-agent",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/default-container": "manager",
						},
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": "threeport-agent",
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
								"image":           "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1",
								"imagePullPolicy": "IfNotPresent",
								"name":            "kube-rbac-proxy",
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
								"args":            agentArgs,
								"image":           agentImage,
								"imagePullPolicy": "IfNotPresent",
								"command":         getCommand(cpi.Opts.AgentInfo.Name, liveReload, false),
								//"livenessProbe": map[string]interface{}{
								//	"httpGet": map[string]interface{}{
								//		"path": "/healthz",
								//		"port": 8081,
								//	},
								//	"initialDelaySeconds": 5,
								//	"periodSeconds":       20,
								//},
								"name": "manager",
								"readinessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/readyz",
										"port": 8081,
									},
									"initialDelaySeconds": 5,
									"periodSeconds":       10,
								},
								//"resources": map[string]interface{}{
								//	"limits": map[string]interface{}{
								//		"cpu":    "500m",
								//		"memory": "128Mi",
								//	},
								//	"requests": map[string]interface{}{
								//		"cpu":    "10m",
								//		"memory": "64Mi",
								//	},
								//},
								//"securityContext": map[string]interface{}{
								//	"allowPrivilegeEscalation": false,
								//	"capabilities": map[string]interface{}{
								//		"drop": []interface{}{
								//			"ALL",
								//		},
								//	},
								//},
								"volumeMounts": agentVolMounts,
							},
						},
						"imagePullSecrets": agentImagePullSecrets,
						"volumes":          agentVols,
						//"securityContext": map[string]interface{}{
						//	"runAsNonRoot": true,
						//},
						"serviceAccountName":            "threeport-agent-controller-manager",
						"terminationGracePeriodSeconds": 10,
					},
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportAgentDeployment, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent deployment: %w", err)
	}

	return nil
}

// UnInstallThreeportControlPlaneComponents removes any threeport components
// that are tied to infrastructure.  It removes the threeport API's service
// resource that removes the load balancer.  The load balancer must be removed
// prior to deleting infra.
func (cpi *ControlPlaneInstaller) UnInstallThreeportControlPlaneComponents(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	// get the service resource
	apiService, err := cpi.GetThreeportAPIService(kubeClient, *mapper)
	if err != nil {
		return fmt.Errorf("failed to get threeport API service resource: %w", err)
	}

	// delete the service
	if err := kube.DeleteResource(apiService, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to delete the threeport API service resource: %w", err)
	}

	// wait until the service is deleted
	util.Retry(24, 5, func() error {

		// get the service resource
		_, err := cpi.GetThreeportAPIService(kubeClient, *mapper)
		if err == nil {
			return errors.New("service still prresent in cluster")
		}

		return nil
	})

	return nil
}

// GetThreeportAPIEndpoint retrieves the endpoint given to the threeport API
// when the external load balancer was provisioned by the infra provider.  It
// will attempt to retrieve this value several times since the load balancer
// value may not be available immediately.
func (cpi *ControlPlaneInstaller) GetThreeportAPIEndpoint(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (string, error) {
	var apiEndpoint string

	maxAttempts := 12
	waitSeconds := 5
	if err := util.Retry(maxAttempts, waitSeconds,
		func() error {
			apiService, err := cpi.GetThreeportAPIService(kubeClient, mapper)
			if err != nil {
				return fmt.Errorf("failed to get threeport API service resource: %w", err)
			}

			// find the ingress hostname in the service resource
			status, found, err := unstructured.NestedMap(apiService.Object, "status")
			if err != nil || !found {
				return fmt.Errorf("failed to retrieve threeport API service status: %w", err)
			}

			loadBalancer, found, err := unstructured.NestedMap(status, "loadBalancer")
			if err != nil || !found {
				return fmt.Errorf("failed to retrieve threeport API load balancer: %w", err)
			}

			ingress, found, err := unstructured.NestedSlice(loadBalancer, "ingress")
			if err != nil || !found || len(ingress) == 0 {
				return fmt.Errorf("failed to retrieve threeport API load balancer ingress: %w", err)
			}

			firstIngress := ingress[0].(map[string]interface{})
			apiEndpoint, found, err = unstructured.NestedString(firstIngress, "hostname")
			if err != nil || !found {
				return fmt.Errorf("failed to retrieve threeport API load balancer hostname: %w", err)
			}

			return nil
		},
	); err != nil {
		msg := fmt.Sprintf(
			"timed out after %d seconds trying to retrieve threeport API load balancer endpoint",
			maxAttempts*waitSeconds,
		)
		return "", fmt.Errorf("%s: %w", msg, err)
	}

	return apiEndpoint, nil
}

func (cpi *ControlPlaneInstaller) isThreeportManagedController(info v0.ControlPlaneComponent) bool {
	for _, i := range ThreeportControllerList {
		if info.Name == i.Name {
			return true
		}
	}

	return false
}

// getThreeportAPIService returns the Kubernetes service resource for the
// threeport API as an unstructured object.
func (cpi *ControlPlaneInstaller) GetThreeportAPIService(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (*unstructured.Unstructured, error) {
	apiService, err := kube.GetResource(
		"",
		"v1",
		"Service",
		cpi.Opts.Namespace,
		cpi.Opts.RestApiInfo.ServiceResourceName,
		kubeClient,
		mapper,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes service resource for threeport API: %w", err)
	}

	return apiService, nil
}

// getAPIImage returns the proper container image to use for the API.
func getAPIImage(devEnvironment bool, customThreeportImageRepo, customThreeportImageTag string) string {
	if devEnvironment {
		return "threeport-air"
	}

	imageRepo := ThreeportImageRepo
	if customThreeportImageRepo != "" {
		imageRepo = customThreeportImageRepo
	}

	imageTag := version.GetVersion()
	if customThreeportImageTag != "" {
		imageTag = customThreeportImageTag
	}

	apiImage := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		ThreeportAPIImage,
		imageTag,
	)

	return apiImage
}

// getAPIArgs returns the args that are passed to the API server.
func (cpi *ControlPlaneInstaller) getAPIArgs(devEnvironment bool, isAuthEnabled bool) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	if devEnvironment {
		args := "-auto-migrate=true -verbose=true"

		if !isAuthEnabled {
			args += " -auth-enabled=false"
		}

		return getAirArgs("rest-api", args)
	}

	args := []interface{}{
		"-auto-migrate=true",
	}

	// disable auth if authConfig is not set in tptctl
	if !isAuthEnabled {
		args = append(args, "-auth-enabled=false")
	}

	return args
}

// getControllerArgs returns the args that are passed to a controller.
func (cpi *ControlPlaneInstaller) getControllerArgs(name string, devEnvironment, debug bool, isAuthEnabled bool) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	// if devEnvironment && authConfig != nil {
	if devEnvironment {
		if isAuthEnabled {
			return getAirArgs(name, "")
		}
		return getAirArgs(name, "-auth-enabled=false")
	}

	args := []interface{}{}
	if debug {
		if !isAuthEnabled {
			args = append(args, "--")
			args = append(args, "-auth-enabled=false")
		}
		return append(util.StringToInterfaceList(getDelveArgs(name)), args...)
	}

	if !isAuthEnabled {
		args = append(args, "-auth-enabled=false")
	}

	return args
}

func getAirArgs(name, extraArgs string) []interface{} {
	main := "main_gen.go"
	if name == "rest-api" || name == "agent" {
		main = "main.go"
	}

	appendedArgs := ""
	if extraArgs != "" {
		appendedArgs = " -- " + extraArgs
	}

	return []interface{}{
		"-c", "/threeport/cmd/dev/air.toml",
		"-build.cmd", "go build -gcflags='all=-N -l' -o /threeport/bin/threeport-" + name + " /threeport/cmd/" + name + "/" + main,
		"-build.bin", "/usr/local/bin/dlv",
		"-build.args_bin", strings.Join(getDelveArgs(name), " ") + appendedArgs,
	}

}

func getDelveArgs(name string) []string {
	args := []string{
		"--continue",
		"--accept-multiclient",
		"--listen=:40000",
		"--headless=true",
		"--api-version=2",
		"--log",
		"exec",
		name,
	}

	return args
}

// getAPIVolumes returns volumes and volume mounts for the API server.
func (cpi *ControlPlaneInstaller) getAPIVolumes(devEnvironment bool, isAuthEnabled bool) ([]interface{}, []interface{}) {
	vols := []interface{}{
		map[string]interface{}{
			"name": "db-config",
			"secret": map[string]interface{}{
				"secretName": "db-config",
			},
		},
		map[string]interface{}{
			"name": "db-create",
			"configMap": map[string]interface{}{
				"name": "db-create",
			},
		},
		map[string]interface{}{
			"name": "db-load",
			"configMap": map[string]interface{}{
				"name": "db-load",
			},
		},
	}

	volMounts := []interface{}{
		map[string]interface{}{
			"name":      "db-config",
			"mountPath": "/etc/threeport/",
		},
	}

	if isAuthEnabled {
		caVol, caVolMount := cpi.getSecretVols("api-ca", "/etc/threeport/ca")
		certVol, certVolMount := cpi.getSecretVols("api-cert", "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	if devEnvironment {
		vols, volMounts = cpi.getDevEnvironmentVolumes(vols, volMounts)
	}

	return vols, volMounts
}

// getImage returns the proper container image to use for the
func (cpi *ControlPlaneInstaller) getImage(devEnvironment bool, name, imageName, imageRepo, imageTag string) string {
	if devEnvironment {
		// return cpi.ThreeportDevImages()[name]
		return "threeport-air"
	}

	image := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		imageName,
		imageTag,
	)

	return image
}

// getControllerVolumes returns the volumes and volume mounts for the workload
// controller.
func (cpi *ControlPlaneInstaller) getControllerVolumes(name string, devEnvironment bool, isAuthEnabled bool) ([]interface{}, []interface{}) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	if isAuthEnabled {
		caVol, caVolMount := cpi.getSecretVols(fmt.Sprintf("%s-ca", name), "/etc/threeport/ca")
		certVol, certVolMount := cpi.getSecretVols(fmt.Sprintf("%s-cert", name), "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	if devEnvironment {
		vols, volMounts = cpi.getDevEnvironmentVolumes(vols, volMounts)
	}

	return vols, volMounts
}

// getCodePathVols returns the volume and volume mount for dev environments to
// mount local codebase for live reloads.
func (cpi *ControlPlaneInstaller) getCodePathVols() (map[string]interface{}, map[string]interface{}) {
	codePathVol := map[string]interface{}{
		"name": "code-path",
		"hostPath": map[string]interface{}{
			"type": "Directory",
			"path": "/threeport",
		},
	}
	codePathVolMount := map[string]interface{}{
		"name":      "code-path",
		"mountPath": "/threeport",
	}

	return codePathVol, codePathVolMount
}

// getGoPathVols returns the volume and volume mount for dev environments to
// mount local go path.
func (cpi *ControlPlaneInstaller) getGoPathVols() (map[string]interface{}, map[string]interface{}) {
	goPathVol := map[string]interface{}{
		"name": "go-path",
		"hostPath": map[string]interface{}{
			"type": "Directory",
			"path": "/go",
		},
	}
	goPathVolMount := map[string]interface{}{
		"name":      "go-path",
		"mountPath": "/go",
	}

	return goPathVol, goPathVolMount
}

// getGoCacheVols returns the volume and volume mount for dev environments to
// mount local go path.
func (cpi *ControlPlaneInstaller) getGoCacheVols() (map[string]interface{}, map[string]interface{}) {
	goCacheVol := map[string]interface{}{
		"name": "go-cache",
		"hostPath": map[string]interface{}{
			"type": "Directory",
			"path": "/root/.cache/go-build",
		},
	}
	goCacheVolMount := map[string]interface{}{
		"name":      "go-cache",
		"mountPath": "/root/.cache/go-build",
	}

	return goCacheVol, goCacheVolMount
}

// getSecretVols returns volumes and volume mounts for secrets.
func (cpi *ControlPlaneInstaller) getSecretVols(name string, mountPath string) (map[string]interface{}, map[string]interface{}) {

	vol := map[string]interface{}{
		"name": name,
		"secret": map[string]interface{}{
			"secretName": name,
		},
	}

	volMount := map[string]interface{}{
		"name":      name,
		"mountPath": mountPath,
	}

	return vol, volMount

}

// getTLSSecret returns a Kubernetes secret for the given certificate and private key.
func (cpi *ControlPlaneInstaller) getTLSSecret(name string, certificate string, privateKey string) *unstructured.Unstructured {

	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "kubernetes.io/tls",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cpi.Opts.Namespace,
			},
			"stringData": map[string]interface{}{
				"tls.crt": certificate,
				"tls.key": privateKey,
			},
		},
	}

	return secret
}

// getAPIServiceType returns the threeport API's service type based on the infra
// provider.
func (cpi *ControlPlaneInstaller) getAPIServiceType(infraProvider string) string {
	if infraProvider == "kind" {
		return "NodePort"
	}

	return "LoadBalancer"
}

// getAPIServiceAnnotations returns the threeport API's service annotation based
// on infra provider to provision the correct load balancer.
func getAPIServiceAnnotations(infraProvider string) map[string]interface{} {
	if infraProvider == "eks" {
		return map[string]interface{}{
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
		}
	}

	return map[string]interface{}{}
}

// getAPIServicePort returns threeport API's service port based on infra
// provider.  For kind returns 80 or 443 based on whether authentication is
// enabled.
func (cpi *ControlPlaneInstaller) getAPIServicePort(infraProvider string, isAuthEnabled bool) (string, int32) {
	if infraProvider == "kind" {
		if isAuthEnabled {
			return "https", 443
		}
		return "http", 80
	}

	return "https", 443
}

// getAgentArgs returns the args that are passed to the threeport agent.  In
// devEnvironment, auth is disabled by default.  In tptctl auth is enabled by
// default.
func (cpi *ControlPlaneInstaller) getAgentArgs(devEnvironment bool, isAuthEnabled bool) []interface{} {
	// set flags for dev environment
	if devEnvironment {
		flags := "--metrics-bind-address=127.0.0.1:8080 --leader-elect"
		if !isAuthEnabled {
			return getAirArgs("agent", flags+" --auth-enabled=false")
		} else {
			return getAirArgs("agent", flags)
		}
	}

	// disable auth if authConfig is not set on non-dev deployment
	args := []interface{}{
		"--metrics-bind-address=127.0.0.1:8080",
		"--leader-elect",
	}

	if !isAuthEnabled {
		args = append(args, "--auth-enabled=false")
	}

	return args
}

func (cpi *ControlPlaneInstaller) getControllerSecret(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-config", name),
				"namespace": namespace,
			},
			"type": "Opaque",
			"stringData": map[string]interface{}{
				"API_SERVER":            cpi.Opts.RestApiInfo.ServiceResourceName,
				"MSG_BROKER_HOST":       "nats-js",
				"MSG_BROKER_PORT":       "4222",
				"AWS_ROLE_SESSION_NAME": util.AwsResourceManagerRoleSessionName,
			},
		},
	}
}

func (cpi *ControlPlaneInstaller) getControllerDeployment(
	deployName string,
	name string,
	namespace string,
	saName string,
	image string,
	args []interface{},
	volumes []interface{},
	volumeMounts []interface{},
	imagePullSecrets []interface{},
	debug bool,
	liveReload bool,
) *unstructured.Unstructured {

	// set image pull policy based on debug mode
	imagePullPolicy := "IfNotPresent"
	if debug && !liveReload {
		imagePullPolicy = "Always"
	}

	ports := []map[string]interface{}{}
	if debug {
		ports = append(ports,
			map[string]interface{}{
				"containerPort": 40000,
				"name":          "dlv",
				"protocol":      "TCP",
			})
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deployName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": deployName,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": deployName,
						},
					},
					"spec": map[string]interface{}{
						"serviceAccountName": saName,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            name,
								"image":           image,
								"command":         getCommand(name, liveReload, debug),
								"imagePullPolicy": imagePullPolicy,
								"args":            args,
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "controller-config",
										},
									},
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "encryption-key",
										},
									},
								},
								"volumeMounts":   volumeMounts,
								"readinessProbe": cpi.getReadinessProbe(),
								"ports":          ports,
							},
						},
						"imagePullSecrets": imagePullSecrets,
						"volumes":          volumes,
					},
				},
			},
		},
	}
}

func (cpi *ControlPlaneInstaller) getReadinessProbe() map[string]interface{} {
	var readinessProbe = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"failureThreshold": 1,
			"httpGet": map[string]interface{}{
				"path":   "/readyz",
				"port":   8081,
				"scheme": "HTTP",
			},
			"initialDelaySeconds": 1,
			"periodSeconds":       2,
			"successThreshold":    1,
			"timeoutSeconds":      1,
		},
	}
	return readinessProbe.Object
}

func (cpi *ControlPlaneInstaller) getDevEnvironmentVolumes(vols, volMounts []interface{}) ([]interface{}, []interface{}) {
	codePathVol, codePathVolMount := cpi.getCodePathVols()
	vols = append(vols, codePathVol)
	volMounts = append(volMounts, codePathVolMount)

	goPathVol, goPathVolMount := cpi.getGoPathVols()
	vols = append(vols, goPathVol)
	volMounts = append(volMounts, goPathVolMount)

	goCacheVol, goCacheVolMount := cpi.getGoCacheVols()
	vols = append(vols, goCacheVol)
	volMounts = append(volMounts, goCacheVolMount)

	return vols, volMounts
}

// getImagePullSecrets returns the image pull secret config for a control plane
// component.
func (cpi *ControlPlaneInstaller) getImagePullSecrets(imagePullSecretName string) []interface{} {
	if imagePullSecretName == "" {
		return []interface{}{}
	}

	return []interface{}{
		map[string]interface{}{
			"name": imagePullSecretName,
		},
	}
}

// GetThreeportAPIPort returns the port that the threeport API is running on.
func GetThreeportAPIPort(authEnabled bool) int {
	if authEnabled {
		return 443
	}
	return 80
}

// GetLocalThreeportAPIEndpoint returns the endpoint for the threeport API
// running locally.
func GetLocalThreeportAPIEndpoint(authEnabled bool) string {
	return fmt.Sprintf(
		"%s:%d",
		ThreeportLocalAPIEndpoint,
		GetThreeportAPIPort(authEnabled),
	)
}

// getCommand returns the args that are passed to the threeport agent.
func getCommand(name string, liveReload, debug bool) []interface{} {

	if liveReload {
		return []interface{}{
			"/usr/local/bin/air",
		}
	}

	if debug {
		return []interface{}{
			"/usr/local/bin/dlv",
		}
	}

	return []interface{}{
		fmt.Sprintf("/threeport-%s", name),
	}
}

// // getArgs returns the args that are passed to a controller.
// func getArgs(controller string, devEnvironment bool) []interface{} {
// 	if devEnvironment {
// 		return []interface{}{
// 			"-c", "/threeport/cmd/dev/air.toml",
// 			"-build.cmd", "go build -o bin/threeport-" + controller + " cmd/" + controller + "/main.go",
// 			"-build.bin", "./bin/threeport-" + controller,
// 		}
// 	}
// 	return []interface{}{}
// }
