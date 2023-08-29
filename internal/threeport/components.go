package threeport

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/util"
	"github.com/threeport/threeport/internal/version"
	"github.com/threeport/threeport/pkg/auth/v0"
	v0 "github.com/threeport/threeport/pkg/client/v0"
)

const (
	ThreeportImageRepo                        = "ghcr.io/threeport"
	ThreeportAPIImage                         = "threeport-rest-api"
	ThreeportWorkloadControllerImage          = "threeport-workload-controller"
	ThreeportKubernetesRuntimeControllerImage = "threeport-kubernetes-runtime-controller"
	ThreeportAwsControllerImage               = "threeport-aws-controller"
	ThreeportGatewayControllerImage           = "threeport-gateway-controller"
	ThreeportAgentDeployName                  = "threeport-agent"
	ThreeportAgentImage                       = "threeport-agent"
	ThreeportAPIServiceResourceName           = "threeport-api-server"
	ThreeportAPIIngressResourceName           = "threeport-api-ingress"
	ThreeportLocalAPIEndpoint                 = "localhost"
)

// ThreeportDevImages returns a map of main package dirs to dev image names
func ThreeportDevImages() map[string]string {
	return getImages(true)
}

// ThreeportImages returns a map of main package dirs to image names
func ThreeportImages() map[string]string {
	return getImages(false)
}

// getImages returns an image map depending on the dev environment flag
func getImages(devEnvironment bool) map[string]string {

	imageSuffix := ""

	// set image suffix for dev environment
	if devEnvironment {
		imageSuffix = "-dev"
	}

	images := map[string]string{
		"rest-api":                      fmt.Sprintf("%s%s:latest", ThreeportAPIImage, imageSuffix),
		"workload-controller":           fmt.Sprintf("%s%s:latest", ThreeportWorkloadControllerImage, imageSuffix),
		"kubernetes-runtime-controller": fmt.Sprintf("%s%s:latest", ThreeportKubernetesRuntimeControllerImage, imageSuffix),
		"aws-controller":                fmt.Sprintf("%s%s:latest", ThreeportAwsControllerImage, imageSuffix),
		"gateway-controller":            fmt.Sprintf("%s%s:latest", ThreeportGatewayControllerImage, imageSuffix),
		"agent":                         fmt.Sprintf("%s%s:latest", ThreeportAgentImage, imageSuffix),
	}

	return images
}

// GetThreeportControllerNames returns a list of all threeport controllers
func GetThreeportControllerNames() []string {
	return []string{
		"workload-controller",
		"kubernetes-runtime-controller",
		"aws-controller",
		"gateway-controller",
	}
}

// InstallComputeSpaceControlPlaneComponents
func InstallComputeSpaceControlPlaneComponents(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	runtimeInstanceName string,
	agentImage string,
) error {
	// threeport control plane namespace
	if err := CreateThreeportControlPlaneNamespace(
		kubeClient,
		mapper,
	); err != nil {
		return fmt.Errorf("failed to create threeport control plane namespace: %w", err)
	}

	// threeport agent
	var agentImageRegistry string
	var agentImageTag string
	if agentImage != "" {
		agentRegistry, _, agentTag, err := util.ParseImage(agentImage)
		if err != nil {
			return fmt.Errorf("failed to parse custom threeport agent image: %w", err)
		}
		agentImageRegistry = agentRegistry
		agentImageTag = agentTag
	}
	if err := InstallThreeportAgent(
		kubeClient,
		mapper,
		runtimeInstanceName,
		false,
		agentImageRegistry,
		agentImageTag,
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
func InstallThreeportAPI(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	customThreeportImageRepo string,
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
	infraProvider string,
	encryptionKey string,
) error {
	apiImage := getAPIImage(devEnvironment, customThreeportImageRepo, customThreeportImageTag)
	apiArgs := getAPIArgs(devEnvironment, authConfig)
	apiVols, apiVolMounts := getAPIVolumes(devEnvironment, authConfig)
	apiServiceType := getAPIServiceType(infraProvider)
	apiServiceAnnotations := getAPIServiceAnnotations(infraProvider)
	apiServicePortName, apiServicePort := getAPIServicePort(infraProvider, authConfig)

	var dbCreateConfig = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "db-create",
				"namespace": "threeport-control-plane",
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
	if _, err := kube.CreateResource(dbCreateConfig, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create DB create configmap: %w", err)
	}

	var apiSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "db-config",
				"namespace": ControlPlaneNamespace,
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
	if _, err := kube.CreateResource(apiSecret, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API server secret: %w", err)
	}

	var encryptionSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "encryption-key",
				"namespace": ControlPlaneNamespace,
			},
			"stringData": map[string]interface{}{
				"ENCRYPTION_KEY": encryptionKey,
			},
		},
	}
	if _, err := kube.CreateResource(encryptionSecret, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API server secret: %w", err)
	}

	var apiDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "threeport-api-server",
				"namespace": ControlPlaneNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": "threeport-api-server",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": "threeport-api-server",
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
								"readinessProbe": getReadinessProbe(),
							},
						},
						"volumes": apiVols,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(apiDeployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API server deployment: %w", err)
	}

	// configure node port based on infra provider
	port := map[string]interface{}{
		"name":       apiServicePortName,
		"port":       apiServicePort,
		"protocol":   "TCP",
		"targetPort": 1323,
	}
	if infraProvider == "kind" {
		port["nodePort"] = 30000
	}
	var apiService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      ThreeportAPIServiceResourceName,
				"namespace": ControlPlaneNamespace,
			},
			"annotations": apiServiceAnnotations,
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app.kubernetes.io/name": "threeport-api-server",
				},
				"ports": []interface{}{
					port,
				},
				"type": apiServiceType,
			},
		},
	}
	if _, err := kube.CreateResource(apiService, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API server service: %w", err)
	}

	return nil
}

// InstallThreeportAPITLS installs TLS assets for threeport API.
func InstallThreeportAPITLS(
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

		var apiCa = getTLSSecret("api-ca", authConfig.CAPemEncoded, authConfig.CAPrivateKeyPemEncoded)
		if _, err := kube.CreateResource(apiCa, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
		}

		var apiCert = getTLSSecret("api-cert", serverCertificate, serverPrivateKey)
		if _, err := kube.CreateResource(apiCert, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
		}
	}

	return nil
}

// InstallThreeportControllers installs the threeport controllers in a
// Kubernetes cluster.
func InstallThreeportControllers(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	customThreeportImageRepo string,
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
) error {
	controllerSecret := getControllerSecret("controller", ControlPlaneNamespace)
	if _, err := kube.CreateResource(controllerSecret, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create workload controller secret: %w", err)
	}

	controllers := GetThreeportControllerNames()

	for _, controller := range controllers {
		if err := InstallController(
			controller,
			kubeClient,
			mapper,
			devEnvironment,
			customThreeportImageRepo,
			customThreeportImageTag,
			authConfig,
		); err != nil {
			return fmt.Errorf("failed to install %s: %w", controller, err)
		}
	}

	return nil
}

// InstallController installs a threeport controller by name.
func InstallController(
	name string,
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	customThreeportImageRepo string,
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
) error {
	controllerImage := getImage(name, devEnvironment, customThreeportImageRepo, customThreeportImageTag)
	controllerVols, controllerVolMounts := getControllerVolumes(name, devEnvironment, authConfig)
	controllerArgs := getControllerArgs(devEnvironment, authConfig)

	// if auth is enabled on API, generate client cert and key and store in
	// secrets
	if authConfig != nil {

		certificate, privateKey, err := auth.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
		if err != nil {
			return fmt.Errorf("failed to generate client certificate and private key for workload controller: %w", err)
		}

		ca := getTLSSecret(fmt.Sprintf("%s-ca", name), authConfig.CAPemEncoded, "")
		if _, err := kube.CreateResource(ca, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret for workload controller: %w", err)
		}

		cert := getTLSSecret(fmt.Sprintf("%s-cert", name), certificate, privateKey)
		if _, err := kube.CreateResource(cert, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret for workload controller: %w", err)
		}
	}

	var controllerDeployment = getControllerDeployment(
		name,
		ControlPlaneNamespace,
		controllerImage,
		controllerArgs,
		controllerVols,
		controllerVolMounts,
	)
	if _, err := kube.CreateResource(controllerDeployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create workload controller deployment: %w", err)
	}

	return nil
}

// InstallThreeportAgent installs the threeport agent on a Kubernetes cluster.
func InstallThreeportAgent(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	threeportInstanceName string,
	devEnvironment bool,
	customThreeportImageRepo string,
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
) error {
	agentImage := getImage("agent", devEnvironment, customThreeportImageRepo, customThreeportImageTag)
	agentArgs := getAgentArgs(devEnvironment, authConfig)
	agentVols, agentVolMounts := getControllerVolumes("agent", devEnvironment, authConfig)

	// if auth is enabled on API, generate client cert and key and store in
	// secrets
	if authConfig != nil {
		agentCertificate, agentPrivateKey, err := auth.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
		if err != nil {
			return fmt.Errorf("failed to generate client certificate and private key for threeport agent: %w", err)
		}

		var agentCa = getTLSSecret("agent-ca", authConfig.CAPemEncoded, "")
		if _, err := kube.CreateResource(agentCa, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret for threeport agent: %w", err)
		}

		var agentCert = getTLSSecret("agent-cert", agentCertificate, agentPrivateKey)
		if _, err := kube.CreateResource(agentCert, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret for threeport agent: %w", err)
		}
	}

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
	if _, err := kube.CreateResource(threeportAgentCRD, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-controller-manager",
				"namespace": ControlPlaneNamespace,
			},
		},
	}
	if _, err := kube.CreateResource(threeportAgentServiceAccount, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-leader-election-role",
				"namespace": ControlPlaneNamespace,
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
	if _, err := kube.CreateResource(threeportAgentLeaderElectionRole, kubeClient, *mapper); err != nil {
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
	if _, err := kube.CreateResource(threeportAgentManagerRole, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
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
	if _, err := kube.CreateResource(threeportAgentMetricsReaderRole, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
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
	if _, err := kube.CreateResource(threeportAgentProxyRole, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-leader-election-rolebinding",
				"namespace": ControlPlaneNamespace,
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
					"namespace": ControlPlaneNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(threeportAgentLeaderElectionRoleBinding, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
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
					"namespace": ControlPlaneNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(threeportAgentManagerRoleBinding, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
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
					"namespace": ControlPlaneNamespace,
				},
			},
		},
	}
	if _, err := kube.CreateResource(threeportAgentProxyRoleBinding, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      "threeport-agent-controller-manager-metrics-service",
				"namespace": ControlPlaneNamespace,
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
	if _, err := kube.CreateResource(threeportAgentControllerManagerMetricsService, kubeClient, *mapper); err != nil {
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
					"app.kubernetes.io/part-of":    "threeport-control-plane",
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      ThreeportAgentDeployName,
				"namespace": ControlPlaneNamespace,
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
						"volumes": agentVols,
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
	if _, err := kube.CreateResource(threeportAgentDeployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create threeport agent deployment: %w", err)
	}

	return nil
}

// UnInstallThreeportControlPlaneComponents removes any threeport components
// that are tied to infrastructure.  It removes the threeport API's service
// resource that removes the load balancer.  The load balancer must be removed
// prior to deleting infra.
func UnInstallThreeportControlPlaneComponents(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	// get the service resource
	apiService, err := getThreeportAPIService(kubeClient, *mapper)
	if err != nil {
		return fmt.Errorf("failed to get threeport API service resource: %w", err)
	}

	// delete the service
	if err := kube.DeleteResource(apiService, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to delete the threeport API service resource: %w", err)
	}

	return nil
}

// WaitForThreeportAPI waits for the threeport API to respond to a request.
func WaitForThreeportAPI(apiClient *http.Client, apiEndpoint string) error {
	var waitError error

	attempts := 0
	maxAttempts := 30
	waitSeconds := 10
	apiReady := false
	for attempts < maxAttempts {
		_, err := v0.GetResponse(
			apiClient,
			fmt.Sprintf("%s/version", apiEndpoint),
			http.MethodGet,
			new(bytes.Buffer),
			http.StatusOK,
		)
		if err != nil {
			waitError = err
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}
		apiReady = true
		break
	}
	if !apiReady {
		msg := fmt.Sprintf(
			"timed out after %d seconds waiting for 200 response from threeport API",
			maxAttempts*waitSeconds,
		)
		return fmt.Errorf("%s: %w", msg, waitError)
	}

	return nil
}

// GetThreeportAPIEndpoint retrieves the endpoint given to the threeport API
// when the external load balancer was provisioned by the infra provider.  It
// will attempt to retrieve this value several times since the load balancer
// value may not be available immediately.
func GetThreeportAPIEndpoint(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (string, error) {
	var apiEndpoint string
	var getEndpointErr error

	attempts := 0
	maxAttempts := 12
	waitSeconds := 5
	apiEndpointRetrieved := false
	for attempts < maxAttempts {
		// get the service resource
		apiService, err := getThreeportAPIService(kubeClient, mapper)
		if err != nil {
			getEndpointErr = err
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}

		// find the ingress hostname in the service resource
		status, found, err := unstructured.NestedMap(apiService.Object, "status")
		if err != nil || !found {
			getEndpointErr = err
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}

		loadBalancer, found, err := unstructured.NestedMap(status, "loadBalancer")
		if err != nil || !found {
			getEndpointErr = err
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}

		ingress, found, err := unstructured.NestedSlice(loadBalancer, "ingress")
		if err != nil || !found || len(ingress) == 0 {
			getEndpointErr = err
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}

		firstIngress := ingress[0].(map[string]interface{})
		apiEndpoint, found, err = unstructured.NestedString(firstIngress, "hostname")
		if err != nil || !found {
			getEndpointErr = err
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}
		apiEndpointRetrieved = true
		break
	}

	if !apiEndpointRetrieved {
		if getEndpointErr == nil {
			errors.New("hostname for load balancer not found in threeport API service")
		}
		msg := fmt.Sprintf(
			"timed out after %d seconds trying to retrieve threeport API load balancer endpoint",
			maxAttempts*waitSeconds,
		)
		return apiEndpoint, fmt.Errorf("%s: %w", msg, getEndpointErr)
	}

	return apiEndpoint, nil
}

// getThreeportAPIService returns the Kubernetes service resource for the
// threeport API as an unstructured object.
func getThreeportAPIService(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (*unstructured.Unstructured, error) {
	apiService, err := kube.GetResource(
		"",
		"v1",
		"Service",
		ControlPlaneNamespace,
		ThreeportAPIServiceResourceName,
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
		devImages := ThreeportDevImages()
		return devImages["rest-api"]
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
func getAPIArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	if devEnvironment {
		args := "-auto-migrate=true -verbose=true"

		if authConfig == nil {
			args += " -auth-enabled=false"
		}

		// -build.args_bin is an air flag, not a part of the API server
		return []interface{}{
			"-build.args_bin",
			args,
		}
	}

	args := []interface{}{
		"-auto-migrate=true",
	}

	// disable auth if authConfig is not set in tptctl
	if authConfig == nil {
		args = append(args, "-auth-enabled=false")
	}

	return args
}

// getControllerArgs returns the args that are passed to a controller.
func getControllerArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	// if devEnvironment && authConfig != nil {
	if devEnvironment && authConfig == nil {
		return []interface{}{
			"-build.args_bin",
			"-auth-enabled=false",
		}
	}

	// disable auth if authConfig is not set in tptctl
	if authConfig == nil {
		return []interface{}{
			"-auth-enabled=false",
		}
	}

	return []interface{}{}
}

// getAPIVolumes returns volumes and volume mounts for the API server.
func getAPIVolumes(devEnvironment bool, authConfig *auth.AuthConfig) ([]interface{}, []interface{}) {
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

	if authConfig != nil {
		caVol, caVolMount := getSecretVols("api-ca", "/etc/threeport/ca")
		certVol, certVolMount := getSecretVols("api-cert", "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	if devEnvironment {
		vols, volMounts = getDevEnvironmentVolumes(vols, volMounts)
	}

	return vols, volMounts
}

// getImage returns the proper container image to use for the
func getImage(name string, devEnvironment bool, customThreeportImageRepo, customThreeportImageTag string) string {
	if devEnvironment {
		return ThreeportDevImages()[name]
	}

	imageRepo := ThreeportImageRepo
	if customThreeportImageRepo != "" {
		imageRepo = customThreeportImageRepo
	}

	imageTag := version.GetVersion()
	if customThreeportImageTag != "" {
		imageTag = customThreeportImageTag
	}

	workloadControllerImage := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		fmt.Sprintf("threeport-%s", name),
		imageTag,
	)

	return workloadControllerImage
}

// getControllerVolumes returns the volumes and volume mounts for the workload
// controller.
func getControllerVolumes(name string, devEnvironment bool, authConfig *auth.AuthConfig) ([]interface{}, []interface{}) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	if authConfig != nil {
		caVol, caVolMount := getSecretVols(fmt.Sprintf("%s-ca", name), "/etc/threeport/ca")
		certVol, certVolMount := getSecretVols(fmt.Sprintf("%s-cert", name), "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	if devEnvironment {
		vols, volMounts = getDevEnvironmentVolumes(vols, volMounts)
	}

	return vols, volMounts
}

// getWorkloadControllerArgs returns the args that are passed to the workload controller.
func getWorkloadControllerArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	// if devEnvironment && authConfig != nil {
	if devEnvironment && authConfig == nil {
		return []interface{}{
			"-build.args_bin",
			"-auth-enabled=false",
		}
	}

	// disable auth if authConfig is not set in tptctl
	if authConfig == nil {
		return []interface{}{
			"-auth-enabled=false",
		}
	}

	return []interface{}{}
}

// getKubernetesRuntimeControllerImage returns the proper container image to use for the
// kubernetes runtime controller.
func getKubernetesRuntimeControllerImage(devEnvironment bool, customThreeportImageRepo, customThreeportImageTag string) string {
	if devEnvironment {
		devImages := ThreeportDevImages()
		return devImages["kubernetes-runtime-controller"]
	}

	imageRepo := ThreeportImageRepo
	if customThreeportImageRepo != "" {
		imageRepo = customThreeportImageRepo
	}

	imageTag := version.GetVersion()
	if customThreeportImageTag != "" {
		imageTag = customThreeportImageTag
	}

	kubernetesRuntimeControllerImage := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		ThreeportKubernetesRuntimeControllerImage,
		imageTag,
	)

	return kubernetesRuntimeControllerImage
}

// getKubernetesRuntimeControllerVolumes returns the volumes and volume mounts for the workload
// controller.
func getKubernetesRuntimeControllerVolumes(devEnvironment bool, authConfig *auth.AuthConfig) ([]interface{}, []interface{}) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	if authConfig != nil {
		caVol, caVolMount := getSecretVols("kubernetes-runtime-ca", "/etc/threeport/ca")
		certVol, certVolMount := getSecretVols("kubernetes-runtime-cert", "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	if devEnvironment {
		vols, volMounts = getDevEnvironmentVolumes(vols, volMounts)
	}

	return vols, volMounts
}

// getKubernetesRuntimeControllerArgs returns the args that are passed to the kubernetes runtime controller.
func getKubernetesRuntimeControllerArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	// if devEnvironment && authConfig != nil {
	if devEnvironment && authConfig == nil {
		return []interface{}{
			"-build.args_bin",
			"-auth-enabled=false",
		}
	}

	// disable auth if authConfig is not set in tptctl
	if authConfig == nil {
		return []interface{}{
			"-auth-enabled=false",
		}
	}

	return []interface{}{}
}

// getAwsControllerImage returns the proper container image to use for the
// aws controller.
func getAwsControllerImage(devEnvironment bool, customThreeportImageRepo, customThreeportImageTag string) string {
	if devEnvironment {
		devImages := ThreeportDevImages()
		return devImages["aws-controller"]
	}

	imageRepo := ThreeportImageRepo
	if customThreeportImageRepo != "" {
		imageRepo = customThreeportImageRepo
	}

	imageTag := version.GetVersion()
	if customThreeportImageTag != "" {
		imageTag = customThreeportImageTag
	}

	awsControllerImage := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		ThreeportAwsControllerImage,
		imageTag,
	)

	return awsControllerImage
}

// getAwsControllerVolumes returns the volumes and volume mounts for the aws
// controller.
func getAwsControllerVolumes(devEnvironment bool, authConfig *auth.AuthConfig) ([]interface{}, []interface{}) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	if authConfig != nil {
		caVol, caVolMount := getSecretVols("aws-ca", "/etc/threeport/ca")
		certVol, certVolMount := getSecretVols("aws-cert", "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	if devEnvironment {
		vols, volMounts = getDevEnvironmentVolumes(vols, volMounts)
	}

	return vols, volMounts
}

// getAwsControllerArgs returns the args that are passed to the aws controller.
func getAwsControllerArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {

	// in devEnvironment, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	// if devEnvironment && authConfig != nil {
	if devEnvironment && authConfig == nil {
		return []interface{}{
			"-build.args_bin",
			"-auth-enabled=false",
		}
	}

	// disable auth if authConfig is not set in tptctl
	if authConfig == nil {
		return []interface{}{
			"-auth-enabled=false",
		}
	}

	return []interface{}{}
}

// getCodePathVols returns the volume and volume mount for dev environments to
// mount local codebase for live reloads.
func getCodePathVols() (map[string]interface{}, map[string]interface{}) {
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
func getGoPathVols() (map[string]interface{}, map[string]interface{}) {
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
func getGoCacheVols() (map[string]interface{}, map[string]interface{}) {
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
func getSecretVols(name string, mountPath string) (map[string]interface{}, map[string]interface{}) {

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
func getTLSSecret(name string, certificate string, privateKey string) *unstructured.Unstructured {

	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "kubernetes.io/tls",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ControlPlaneNamespace,
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
func getAPIServiceType(infraProvider string) string {
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
func getAPIServicePort(infraProvider string, authConfig *auth.AuthConfig) (string, int32) {
	if infraProvider == "kind" {
		if authConfig != nil {
			return "https", 443
		}
		return "http", 80
	}

	return "https", 443
}

// getAgentArgs returns the args that are passed to the threeport agent.  In
// devEnvironment, auth is disabled by default.  In tptctl auth is enabled by
// default.
func getAgentArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {
	// set flags for dev environment
	if devEnvironment {
		if authConfig == nil {
			return []interface{}{
				"-build.args_bin",
				"--auth-enabled=false, --metrics-bind-address=127.0.0.1:8080, --leader-elect",
			}
		} else {
			return []interface{}{
				"-build.args_bin",
				"--metrics-bind-address=127.0.0.1:8080, --leader-elect",
			}
		}
	}

	// disable auth if authConfig is not set on non-dev deployment
	if authConfig == nil {
		return []interface{}{
			"--auth-enabled=false",
			"--metrics-bind-address=127.0.0.1:8080",
			"--leader-elect",
		}
	}

	return []interface{}{
		"--metrics-bind-address=127.0.0.1:8080",
		"--leader-elect",
	}
}

func getControllerSecret(name, namespace string) *unstructured.Unstructured {
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
				"API_SERVER":      "threeport-api-server",
				"MSG_BROKER_HOST": "nats-js",
				"MSG_BROKER_PORT": "4222",
			},
		},
	}
}

func getControllerDeployment(name, namespace, image string, args, volumes, volumeMounts []interface{}) *unstructured.Unstructured {
	deployName := fmt.Sprintf("threeport-%s", name)
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
						"containers": []interface{}{
							map[string]interface{}{
								"name":            name,
								"image":           image,
								"imagePullPolicy": "IfNotPresent",
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
								"readinessProbe": getReadinessProbe(),
							},
						},
						"volumes": volumes,
					},
				},
			},
		},
	}
}

func getReadinessProbe() map[string]interface{} {
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

func getDevEnvironmentVolumes(vols, volMounts []interface{}) ([]interface{}, []interface{}) {
	codePathVol, codePathVolMount := getCodePathVols()
	vols = append(vols, codePathVol)
	volMounts = append(volMounts, codePathVolMount)

	goPathVol, goPathVolMount := getGoPathVols()
	vols = append(vols, goPathVol)
	volMounts = append(volMounts, goPathVolMount)

	goCacheVol, goCacheVolMount := getGoCacheVols()
	vols = append(vols, goCacheVol)
	volMounts = append(volMounts, goCacheVolMount)

	return vols, volMounts
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
