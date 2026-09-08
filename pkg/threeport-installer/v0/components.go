package v0

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/version"
	"github.com/threeport/threeport/pkg/api-server/v0/database"
	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	DbInitFilename            = "db.sql"
	DbInitLocation            = "/etc/threeport/db-create"
	ThreeportApiCaSecret      = "api-ca"
	ThreeportApiConfigSecret  = "api-config"
	dbRootCertSecretName      = "db-root-cert"
	dbThreeportCertSecretName = "db-threeport-cert"
)

// InstallComputeSpaceControlPlaneComponents installs the Threeport control
// plane components that are deployed to Threeport-managed compute space
// clusters, i.e. clusters that do not have the Threeport control plane installed.
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

	// threeport agent
	if err := cpi.InstallThreeportAgent(
		kubeClient,
		mapper,
		nil,
	); err != nil {
		return fmt.Errorf("failed to install threeport agent: %w", err)
	}

	// threeport CRDs
	if err := InstallThreeportCRDs(kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to install threeport CRDs: %w", err)
	}

	return nil
}

// InstallComputeSpaceWorkloadControllerRBAC grants the control-plane workload
// controllers cluster-admin on a managed (compute space) GKE cluster.  The
// helm-workload-controller and kubernetes-workload-controller deploy arbitrary
// resources to managed clusters and connect to them as their own GKE Workload
// Identity principals (via per-request ADC tokens), so the managed cluster must
// authorize those principals directly.  This mirrors the bindings created on the
// control-plane cluster in InstallThreeportControllers.
//
// Only the kind:User Workload Identity subject is bound: on a remote managed
// cluster the controllers never authenticate with an in-cluster ServiceAccount
// token, so a kind:ServiceAccount subject would never match.  gcpProjectID is the
// project of the GKE cluster hosting the control plane (where the controller pods
// run), which determines the Workload Identity pool in the principal name.
func (cpi *ControlPlaneInstaller) InstallComputeSpaceWorkloadControllerRBAC(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	gcpProjectID string,
) error {
	workloadControllers := []string{
		ThreeportHelmWorkloadControllerName,
		ThreeportKubernetesWorkloadControllerName,
	}

	for _, controllerName := range workloadControllers {
		clusterAdminBinding := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRoleBinding",
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("%s-cluster-admin", controllerName),
				},
				"roleRef": map[string]interface{}{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "ClusterRole",
					"name":     "cluster-admin",
				},
				"subjects": []interface{}{
					map[string]interface{}{
						"kind":     "User",
						"name":     fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", gcpProjectID, ControlPlaneNamespace, controllerName),
						"apiGroup": "rbac.authorization.k8s.io",
					},
				},
			},
		}
		if err := cpi.CreateOrUpdateKubeResource(clusterAdminBinding, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create %s cluster-admin binding on managed cluster: %w", controllerName, err)
		}
	}

	return nil
}

// UpdateThreeportAPIDeployment installs the threeport API in a Kubernetes
// cluster.
func (cpi *ControlPlaneInstaller) UpdateThreeportAPIDeployment(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	dbCreds *auth.DbCreds,
) error {
	apiImage := cpi.getImage(cpi.Opts.RestApiInfo.Name, cpi.Opts.RestApiInfo.ImageName, cpi.Opts.RestApiInfo.ImageNamespace, cpi.Opts.RestApiInfo.ImageTag)
	apiArgs := cpi.getAPIArgs()
	apiVols, apiVolMounts, err := cpi.getAPIVolumes()
	if err != nil {
		return fmt.Errorf("could not get vols: %w", err)
	}

	apiImagePullSecrets := cpi.getImagePullSecrets(cpi.Opts.RestApiInfo.ImagePullSecretName)

	dbMigratorImage := fmt.Sprintf(
		"%s/%s:%s",
		cpi.Opts.DatabaseMigratorInfo.ImageNamespace,
		cpi.Opts.DatabaseMigratorInfo.ImageName,
		cpi.Opts.DatabaseMigratorInfo.ImageTag,
	)

	dbMigratorArgs := []interface{}{"-env-file=/etc/threeport/env", "up"}

	// only create DB cert secrets if they don't already exist — regenerating
	// them would break TLS against a running CRDB whose node certs are signed
	// by the original CA (e.g. tptdev debug). On reconciler retry the secrets
	// may not exist yet, so we check rather than relying on CreateOrUpdateKubeResources.
	_, dbCertErr := kube.GetResource("", "v1", "Secret", cpi.Opts.Namespace, dbRootCertSecretName, kubeClient, *mapper)
	if dbCertErr != nil {
		// secret for 'root' user credentials to database - used for database
		// initialization
		var dbRootCertsSecret = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      dbRootCertSecretName,
					"namespace": cpi.Opts.Namespace,
				},
				"stringData": map[string]interface{}{
					"ca.crt":          dbCreds.AuthConfig.CAPemEncoded,
					"client.root.crt": dbCreds.RootCert,
					"client.root.key": dbCreds.RootKey,
				},
			},
		}

		if err := cpi.CreateOrUpdateKubeResource(dbRootCertsSecret, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create DB root user certs secret: %w", err)
		}

		// secret for 'threeport' user credentials to database - used by threeport
		// API for DB connectectivity
		var dbThreeportCertsSecret = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      dbThreeportCertSecretName,
					"namespace": cpi.Opts.Namespace,
				},
				"stringData": map[string]interface{}{
					"ca.crt":               dbCreds.AuthConfig.CAPemEncoded,
					"client.threeport.crt": dbCreds.ThreeportCert,
					"client.threeport.key": dbCreds.ThreeportKey,
				},
			},
		}

		if err := cpi.CreateOrUpdateKubeResource(dbThreeportCertsSecret, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create DB threeport user certs secret: %w", err)
		}
	}

	var dbCreateConfig = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "db-create",
				"namespace": cpi.Opts.Namespace,
			},
			"data": map[string]interface{}{
				"db.sql": `CREATE USER IF NOT EXISTS threeport;
CREATE DATABASE IF NOT EXISTS threeport_api encoding='utf-8';
GRANT ALL ON DATABASE threeport_api TO threeport;
`,
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(dbCreateConfig, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create DB initialization config map: %w", err)
	}

	var apiSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      ThreeportApiConfigSecret,
				"namespace": cpi.Opts.Namespace,
			},
			"stringData": map[string]interface{}{
				"env": fmt.Sprintf(`DB_HOST=%[1]s.%[2]s.svc.cluster.local
DB_USER=%[3]s
DB_NAME=%[4]s
DB_PORT=%[5]s
DB_SSL_MODE=%[6]s
NATS_HOST=%[7]s.%[2]s.svc.cluster.local
NATS_PORT=4222
THREEPORT_API_ENDPOINT=%[8]s.%[2]s.svc.cluster.local
THREEPORT_CONTROL_PLANE_NAMESPACE=%[2]s
`,
					database.ThreeportDatabaseHost,
					cpi.Opts.Namespace,
					database.ThreeportDatabaseUser,
					database.ThreeportDatabaseName,
					database.ThreeportDatabasePort,
					database.ThreeportDatabaseSslMode,
					natsServiceName,
					ThreeportAPIServiceResourceName,
				),
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(apiSecret, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for DB connection: %w", err)
	}

	// configure ports
	ports := []map[string]interface{}{
		{
			"containerPort": 1323,
			"name":          "api",
			"protocol":      "TCP",
		},
	}

	initContainers := []interface{}{
		map[string]interface{}{
			"name":            "db-init",
			"image":           dbMigratorImage,
			"imagePullPolicy": cpi.getImagePullPolicy(),
			"command": []interface{}{
				fmt.Sprintf("/%s", cpi.Opts.DatabaseMigratorInfo.BinaryName),
			},
			"args": []interface{}{"-env-file=/etc/threeport/env", "initialize"},
			"volumeMounts": []interface{}{
				map[string]interface{}{
					"name":      "db-create",
					"mountPath": "/etc/threeport/db-create",
				},
				map[string]interface{}{
					"name":      ThreeportApiConfigSecret,
					"mountPath": "/etc/threeport/",
				},
				map[string]interface{}{
					"name":      "db-root-cert",
					"mountPath": "/etc/threeport/db-certs",
				},
			},
		},
		map[string]interface{}{
			"name":            "database-migrator",
			"image":           dbMigratorImage,
			"imagePullPolicy": cpi.getImagePullPolicy(),
			"command": []interface{}{
				fmt.Sprintf("/%s", cpi.Opts.DatabaseMigratorInfo.BinaryName),
			},
			"args": dbMigratorArgs,
			"volumeMounts": []interface{}{
				map[string]interface{}{
					"name":      ThreeportApiConfigSecret,
					"mountPath": "/etc/threeport/",
				},
				map[string]interface{}{
					"name":      "db-threeport-cert",
					"mountPath": "/etc/threeport/db-certs",
				},
			},
		},
	}

	// configure additional init containers
	for _, ic := range cpi.Opts.RestApiAdditionalInitContainers {
		initContainers = append(initContainers, ic)
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
						"initContainers": initContainers,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "api-server",
								"image":           apiImage,
								"command":         cpi.getCommand(cpi.Opts.RestApiInfo.BinaryName),
								"imagePullPolicy": cpi.getImagePullPolicy(),
								"args":            apiArgs,
								"ports":           ports,
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

	return nil
}

// InstallThreeportAPITLS installs TLS assets for threeport API.
func (cpi *ControlPlaneInstaller) InstallThreeportAPITLS(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	authConfig *auth.AuthConfig,
	serverAltNames ...string,
) error {
	if authConfig != nil {
		// generate server certificate
		serverCertificate, serverPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
			"threeport-api-server",
			apilib.CoreApiNamespace,
			auth.OUControlPlane,
			serverAltNames...,
		)
		if err != nil {
			return fmt.Errorf("failed to generate server certificate and private key: %w", err)
		}

		var apiCa = cpi.getTLSSecret(ThreeportApiCaSecret, authConfig.CAPemEncoded, authConfig.CAPrivateKeyPemEncoded)
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

	// ClusterRole granting all controllers CRUD on ThreeportWorkload resources.
	// ThreeportWorkload is cluster-scoped, so a ClusterRole is required.
	threeportWorkloadClusterRole := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "threeport-controllers-threeportworkloads",
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
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(threeportWorkloadClusterRole, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create threeport controllers threeportworkloads cluster role: %w", err)
	}

	for _, controller := range cpi.Opts.ControllerList {
		if !*controller.Enabled {
			continue
		}

		// if auth is enabled on API, generate client cert and key and store in
		// secrets
		if authConfig != nil {
			certificate, privateKey, err := auth.GenerateCertificate(
				authConfig.CAConfig,
				&authConfig.CAPrivateKey,
				controller.Name,
				apilib.CoreApiNamespace,
				auth.OUControlPlane,
			)
			if err != nil {
				return fmt.Errorf("failed to generate client certificate and private key for kubernetes workload controller: %w", err)
			}

			ca := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"type":       "Opaque",
					"metadata": map[string]interface{}{
						"name":      fmt.Sprintf("%s-ca", controller.Name),
						"namespace": cpi.Opts.Namespace,
					},
					"stringData": map[string]interface{}{
						"tls.crt": authConfig.CAPemEncoded,
					},
				},
			}
			if err := cpi.CreateOrUpdateKubeResource(ca, kubeClient, mapper); err != nil {
				return fmt.Errorf("failed to create API server ca secret for kubernetes workload controller: %w", err)
			}

			cert := cpi.getTLSSecret(fmt.Sprintf("%s-cert", controller.Name), certificate, privateKey)
			if err := cpi.CreateOrUpdateKubeResource(cert, kubeClient, mapper); err != nil {
				return fmt.Errorf("failed to create API server certificate secret for kubernetes workload controller: %w", err)
			}
		}

		// create controller service account
		serviceAccountMetadata := map[string]interface{}{
			"name":      controller.ServiceAccountName,
			"namespace": cpi.Opts.Namespace,
		}

		// add Workload Identity annotation for gcp-controller when GCP service account is configured
		if controller.Name == ThreeportGcpControllerName && cpi.Opts.GcpServiceAccountEmail != "" {
			serviceAccountMetadata["annotations"] = map[string]interface{}{
				"iam.gke.io/gcp-service-account": cpi.Opts.GcpServiceAccountEmail,
			}
		}

		serviceAccount := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ServiceAccount",
				"metadata":   serviceAccountMetadata,
			},
		}
		if err := cpi.CreateOrUpdateKubeResource(serviceAccount, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create controller service account: %w", err)
		}

		// Bind the shared ClusterRole to this controller's service account so it
		// can manage ThreeportWorkload resources.
		threeportWorkloadSubjects := []interface{}{
			map[string]interface{}{
				"kind":      "ServiceAccount",
				"name":      controller.ServiceAccountName,
				"namespace": cpi.Opts.Namespace,
			},
		}
		// On GKE with Workload Identity enabled, the kube-apiserver authenticates
		// pod tokens as "serviceAccount:PROJECT.svc.id.goog[NAMESPACE/KSA]" rather
		// than "system:serviceaccount:NAMESPACE:NAME". The kind:ServiceAccount
		// subject only matches the latter, so we add a kind:User subject for the
		// WI principal to ensure RBAC applies to the actual pod identity.
		if cpi.Opts.InfraProvider == v0.KubernetesRuntimeInfraProviderGKE && cpi.Opts.GcpProjectId != "" {
			threeportWorkloadSubjects = append(threeportWorkloadSubjects, map[string]interface{}{
				"kind":      "User",
				"name":      fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", cpi.Opts.GcpProjectId, cpi.Opts.Namespace, controller.ServiceAccountName),
				"apiGroup":  "rbac.authorization.k8s.io",
			})
		}
		threeportWorkloadClusterRoleBinding := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRoleBinding",
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("%s-%s-threeportworkloads", cpi.Opts.Namespace, controller.ServiceAccountName),
				},
				"roleRef": map[string]interface{}{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "ClusterRole",
					"name":     "threeport-controllers-threeportworkloads",
				},
				"subjects": threeportWorkloadSubjects,
			},
		}
		if err := cpi.CreateOrUpdateKubeResource(threeportWorkloadClusterRoleBinding, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create threeportworkloads cluster role binding for %s: %w", controller.Name, err)
		}

		// The helm-workload-controller and kubernetes-workload-controller deploy
		// arbitrary resources — Helm charts or raw manifests — that can define any
		// Kubernetes resource type in any namespace (including creating
		// namespaces). The control-plane-controller similarly creates namespaces,
		// secrets, service accounts, and StatefulSets/Deployments directly when
		// bootstrapping a new child control plane. cluster-admin is required so
		// they can manage the full lifecycle of those resources. On GKE with
		// Workload Identity, each controller authenticates to the target cluster
		// as its own WI principal (via per-request ADC tokens), so both the
		// ServiceAccount and User subjects are bound.
		if controller.Name == ThreeportHelmWorkloadControllerName ||
			controller.Name == ThreeportKubernetesWorkloadControllerName ||
			controller.Name == ThreeportControlPlaneControllerName {
			clusterAdminSubjects := []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      controller.ServiceAccountName,
					"namespace": cpi.Opts.Namespace,
				},
			}
			if cpi.Opts.InfraProvider == v0.KubernetesRuntimeInfraProviderGKE && cpi.Opts.GcpProjectId != "" {
				clusterAdminSubjects = append(clusterAdminSubjects, map[string]interface{}{
					"kind":     "User",
					"name":     fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", cpi.Opts.GcpProjectId, cpi.Opts.Namespace, controller.ServiceAccountName),
					"apiGroup": "rbac.authorization.k8s.io",
				})
			}
			clusterAdminBinding := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "rbac.authorization.k8s.io/v1",
					"kind":       "ClusterRoleBinding",
					"metadata": map[string]interface{}{
						"name": fmt.Sprintf("%s-%s-cluster-admin", cpi.Opts.Namespace, controller.ServiceAccountName),
					},
					"roleRef": map[string]interface{}{
						"apiGroup": "rbac.authorization.k8s.io",
						"kind":     "ClusterRole",
						"name":     "cluster-admin",
					},
					"subjects": clusterAdminSubjects,
				},
			}
			if err := cpi.CreateOrUpdateKubeResource(clusterAdminBinding, kubeClient, mapper); err != nil {
				return fmt.Errorf("failed to create %s cluster-admin binding: %w", controller.Name, err)
			}
		}

		if err := cpi.UpdateControllerDeployment(
			kubeClient,
			mapper,
			*controller,
		); err != nil {
			return fmt.Errorf("failed to install %s: %w", controller.Name, err)
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
	controller v0.ControlPlaneComponent,
) error {

	var deployName string
	if cpi.isThreeportManagedController(controller) {
		deployName = fmt.Sprintf("threeport-%s", controller.Name)
	} else {
		deployName = fmt.Sprintf("%s-%s", cpi.Opts.Name, controller.Name)
	}

	controllerDeployment, err := cpi.getControllerDeployment(
		deployName,
		cpi.Opts.Namespace,
		controller,
	)
	if err != nil {
		return fmt.Errorf("failed to get %s deployment: %w", controller.Name, err)
	}

	if err := cpi.CreateOrUpdateKubeResource(controllerDeployment, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create %s deployment: %w", controller.Name, err)
	}
	return nil
}

// InstallThreeportAgent installs the threeport agent on a Kubernetes cluster.
func (cpi *ControlPlaneInstaller) InstallThreeportAgent(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	authConfig *auth.AuthConfig,
) error {

	// if auth is enabled on API, generate client cert and key and store in
	// secrets
	if authConfig != nil {
		agentCertificate, agentPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
			"threeport-agent",
			apilib.CoreApiNamespace,
			auth.OUControlPlane,
		)
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
	); err != nil {
		return fmt.Errorf("failed to update threeport agent deployment: %w", err)
	}
	return nil
}

// UpdateThreeportAgentDeployment updates the threeport agent on a Kubernetes cluster.
func (cpi *ControlPlaneInstaller) UpdateThreeportAgentDeployment(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {

	agentImage := cpi.getImage(cpi.Opts.AgentInfo.Name, cpi.Opts.AgentInfo.ImageName, cpi.Opts.AgentInfo.ImageNamespace, cpi.Opts.AgentInfo.ImageTag)
	agentArgs := cpi.getAgentArgs()
	agentVols, agentVolMounts, err := cpi.getControllerVolumes(*cpi.Opts.AgentInfo)
	if err != nil {
		return fmt.Errorf("could not get agent vols: %w", err)
	}
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
											"workloadType": map[string]interface{}{
												"description": "WorkloadType informs the threeport agent which threeport API type was used to represent a Kubernetes workload.",
												"enum": []interface{}{
													"KubernetesWorkloadInstance",
													"HelmWorkloadInstance",
												},
												"type": "string",
											},
											"workloadInstanceId": map[string]interface{}{
												"description": "KubernetesWorkloadInstance is the unique ID for a threeport object that represents a deployed instance of a workload.",
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
					"app.kubernetes.io/version":    version.GetVersion(),
					"app.kubernetes.io/component":  "runtime-agent",
					"app.kubernetes.io/part-of":    cpi.Opts.Namespace,
					"app.kubernetes.io/managed-by": "threeport",
				},
				"name":      ThreeportAgentName,
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"name":      ThreeportAgentName,
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"name":      ThreeportAgentName,
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"name":      ThreeportAgentName,
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
					"app.kubernetes.io/instance":   "threeport-agent" + cpi.Opts.ControlPlaneName + "",
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
								"image":           "ghcr.io/kube-rbac-proxy/kube-rbac-proxy:v0.22.0",
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
								"imagePullPolicy": cpi.getImagePullPolicy(),
								"command":         cpi.getCommand(cpi.Opts.AgentInfo.BinaryName),
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
						"serviceAccountName":            ThreeportAgentName,
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
	// delete the control plane namespace
	if err := DeleteNamespaces(
		kubeClient,
		mapper,
		[]string{cpi.Opts.Namespace},
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete control plane namespace: %w", err)
	}

	return nil
}

// DeleteNamespace deletes a list of namespaces from a Kubernetes cluster.
func DeleteNamespaces(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	namespaces []string,
) error {

	// initiate namespace deletion
	for _, name := range namespaces {

		// configure resource interface
		namespaceResource := kubeClient.Resource(
			schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
		)
		deletePolicy := metav1.DeletePropagationForeground
		deleteOptions := metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		}

		// delete the namespace
		if err := namespaceResource.Delete(
			context.TODO(),
			name,
			deleteOptions,
		); err != nil {
			return fmt.Errorf("failed to delete namespace: %w", err)
		}
	}

	// wait for all namespaces to be deleted
	for _, name := range namespaces {
		util.Retry(120, 1, func() error {
			_, err := kube.GetResource(
				"",
				"",
				"Namespace",
				"",
				name,
				kubeClient,
				*mapper,
			)
			if err == nil {
				return errors.New("namespace is still present")
			}

			return nil
		})
	}

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

			switch cpi.Opts.InfraProvider {
			case v0.KubernetesRuntimeInfraProviderEKS:
				if apiEndpoint, found, err = unstructured.NestedString(firstIngress, "hostname"); err != nil || !found {
					return fmt.Errorf("failed to retrieve threeport API load balancer hostname: %w", err)
				}
			case v0.KubernetesRuntimeInfraProviderOKE:
				if apiEndpoint, found, err = unstructured.NestedString(firstIngress, "ip"); err != nil || !found {
					return fmt.Errorf("failed to retrieve threeport API load balancer ip: %w", err)
				}
			case v0.KubernetesRuntimeInfraProviderGKE:
				if apiEndpoint, found, err = unstructured.NestedString(firstIngress, "ip"); err != nil || !found {
					return fmt.Errorf("failed to retrieve threeport API load balancer ip: %w", err)
				}
			default:
				return fmt.Errorf("unsupported infrastructure provider: %s", cpi.Opts.InfraProvider)
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

// getAPIArgs returns the args that are passed to the API server.
func (cpi *ControlPlaneInstaller) getAPIArgs() []interface{} {

	// in tptdev, auth is disabled by default
	// in tptctl, auth is enabled by default

	switch {
	case cpi.Opts.Debug:
		args := []interface{}{
			"-auto-migrate=true",
			"-verbose=true",
		}
		if !cpi.Opts.AuthEnabled {
			args = append(args, "-auth-enabled=false")
		}
		if cpi.Opts.PaginationMode != nil && *cpi.Opts.PaginationMode != "" {
			args = append(args, fmt.Sprintf("-pagination-mode=%s", *cpi.Opts.PaginationMode))
		}
		return args
	default:
		args := []interface{}{
			"-auto-migrate=true",
		}

		// disable auth if authConfig is not set in tptctl
		if !cpi.Opts.AuthEnabled {
			args = append(args, "-auth-enabled=false")
		}
		if cpi.Opts.PaginationMode != nil && *cpi.Opts.PaginationMode != "" {
			args = append(args, fmt.Sprintf("-pagination-mode=%s", *cpi.Opts.PaginationMode))
		}
		return args
	}
}

// getControllerArgs returns the args that are passed to a controller.
func (cpi *ControlPlaneInstaller) getControllerArgs() []interface{} {

	// in tptdev, auth is disabled by default
	// in tptctl, auth is enabled by default

	// enable auth if authConfig is set in dev environment
	switch {
	case cpi.Opts.Debug:
		args := []interface{}{}
		if !cpi.Opts.AuthEnabled {
			args = append(args, "-auth-enabled=false")
		}
		if cpi.Opts.Verbose {
			args = append(args, "-verbose=true")
		}
		return args
	default:
		args := []interface{}{}

		if !cpi.Opts.AuthEnabled {
			args = append(args, "-auth-enabled=false")
		}
		if cpi.Opts.Verbose {
			args = append(args, "-verbose=true")
		}
		return args
	}
}

// getAPIVolumes returns volumes and volume mounts for the API server.
func (cpi *ControlPlaneInstaller) getAPIVolumes() ([]interface{}, []interface{}, error) {
	vols := []interface{}{
		map[string]interface{}{
			"name": "db-root-cert",
			"secret": map[string]interface{}{
				"secretName": dbRootCertSecretName,
			},
		},
		map[string]interface{}{
			"name": "db-threeport-cert",
			"secret": map[string]interface{}{
				"secretName": dbThreeportCertSecretName,
			},
		},
		map[string]interface{}{
			"name": ThreeportApiConfigSecret,
			"secret": map[string]interface{}{
				"secretName": ThreeportApiConfigSecret,
			},
		},
		map[string]interface{}{
			"name": "db-create",
			"configMap": map[string]interface{}{
				"name": "db-create",
			},
		},
	}

	volMounts := []interface{}{
		map[string]interface{}{
			"name":      ThreeportApiConfigSecret,
			"mountPath": "/etc/threeport/",
		},
		map[string]interface{}{
			"name":      "db-threeport-cert",
			"mountPath": "/etc/threeport/db-certs",
		},
	}

	additionalVolumes := make([]map[string]interface{}, 0)
	if cpi.Opts.RestApiInfo.AdditionalVolumes != nil {
		var v []map[string]interface{}
		err := json.Unmarshal([]byte(*cpi.Opts.RestApiInfo.AdditionalVolumes), &v)
		if err != nil {
			return []interface{}{}, []interface{}{}, fmt.Errorf("failed to unmarshal vol json: %w", err)
		}

		additionalVolumes = v
	}

	for _, v := range additionalVolumes {
		vols = append(vols, v)
	}

	additionalVolumeMounts := make([]map[string]interface{}, 0)
	if cpi.Opts.RestApiInfo.AdditionalVolumeMounts != nil {
		var v []map[string]interface{}
		err := json.Unmarshal([]byte(*cpi.Opts.RestApiInfo.AdditionalVolumeMounts), &v)
		if err != nil {
			return []interface{}{}, []interface{}{}, fmt.Errorf("failed to unmarshal vol-mount json: %w", err)
		}

		additionalVolumeMounts = v
	}

	for _, vm := range additionalVolumeMounts {
		volMounts = append(volMounts, vm)
	}

	if cpi.Opts.AuthEnabled {
		caVol, caVolMount := cpi.getSecretVols(ThreeportApiCaSecret, "/etc/threeport/ca")
		certVol, certVolMount := cpi.getSecretVols("api-cert", "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	return vols, volMounts, nil
}

// getImage returns the proper container image to use for the
func (cpi *ControlPlaneInstaller) getImage(name, imageName, imageRepo, imageTag string) string {
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
func (cpi *ControlPlaneInstaller) getControllerVolumes(controller v0.ControlPlaneComponent) ([]interface{}, []interface{}, error) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	if cpi.Opts.AuthEnabled {
		caVol, caVolMount := cpi.getSecretVols(fmt.Sprintf("%s-ca", controller.Name), "/etc/threeport/ca")
		certVol, certVolMount := cpi.getSecretVols(fmt.Sprintf("%s-cert", controller.Name), "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
	}

	additionalVolumes := make([]map[string]interface{}, 0)
	if controller.AdditionalVolumes != nil {
		var v []map[string]interface{}
		err := json.Unmarshal([]byte(*controller.AdditionalVolumes), &v)
		if err != nil {
			return []interface{}{}, []interface{}{}, fmt.Errorf("failed to unmarshal vol json: %w", err)
		}

		additionalVolumes = v
	}

	for _, v := range additionalVolumes {
		vols = append(vols, v)
	}

	additionalVolumeMounts := make([]map[string]interface{}, 0)
	if controller.AdditionalVolumeMounts != nil {
		var v []map[string]interface{}
		err := json.Unmarshal([]byte(*controller.AdditionalVolumeMounts), &v)
		if err != nil {
			return []interface{}{}, []interface{}{}, fmt.Errorf("failed to unmarshal vol-mount json: %w", err)
		}

		additionalVolumeMounts = v
	}

	for _, vm := range additionalVolumeMounts {
		volMounts = append(volMounts, vm)
	}

	return vols, volMounts, nil
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
func (cpi *ControlPlaneInstaller) getAPIServiceType() string {
	if cpi.Opts.InfraProvider == "kind" {
		return "NodePort"
	}

	if !cpi.Opts.RestApiLoadBalancer {
		return "ClusterIP"
	}

	return "LoadBalancer"
}

// getAPIServiceAnnotations returns the threeport API's service annotation based
// on infra provider to provision the correct load balancer.
func (cpi *ControlPlaneInstaller) getAPIServiceAnnotations() map[string]interface{} {
	switch {
	case cpi.Opts.InfraProvider == v0.KubernetesRuntimeInfraProviderEKS && cpi.Opts.RestApiLoadBalancer:
		return map[string]interface{}{
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
		}
	case cpi.Opts.InfraProvider == v0.KubernetesRuntimeInfraProviderOKE:
		return map[string]interface{}{
			"oci.oraclecloud.com/load-balancer-type": "nlb",
		}
	}

	return map[string]interface{}{}
}

// GetAPIServicePort returns the threeport API's service port based on
// whether authentication is enabled.  When auth/TLS is enabled, port
// 443 is used; otherwise port 80.
func (cpi *ControlPlaneInstaller) GetAPIServicePort() (string, int32) {
	if cpi.Opts.AuthEnabled {
		return "https", 443
	}

	return "http", 80
}

// getAgentArgs returns the args that are passed to the threeport agent.  In
// tptdev, auth is disabled by default.  In tptctl auth is enabled by
// default.
func (cpi *ControlPlaneInstaller) getAgentArgs() []interface{} {
	switch {
	case cpi.Opts.Debug:
		args := []interface{}{
			"--metrics-bind-address=127.0.0.1:8080",
			"--leader-elect",
		}
		if !cpi.Opts.AuthEnabled {
			args = append(args, "--auth-enabled=false")
		}
		return args
	default:
		// disable auth if authConfig is not set on non-dev deployment
		args := []interface{}{
			"--metrics-bind-address=127.0.0.1:8080",
			"--leader-elect",
		}
		if !cpi.Opts.AuthEnabled {
			args = append(args, "--auth-enabled=false")
		}
		return args
	}
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
				"API_SERVER":            fmt.Sprintf("%s.%s.svc.cluster.local", cpi.Opts.RestApiInfo.ServiceResourceName, cpi.Opts.Namespace),
				"MSG_BROKER_HOST":       fmt.Sprintf("%s.%s.svc.cluster.local", natsServiceName, cpi.Opts.Namespace),
				"MSG_BROKER_PORT":       "4222",
				"AWS_ROLE_SESSION_NAME": util.AwsResourceManagerRoleSessionName,
			},
		},
	}
}

// getImagePullPolicy returns the image pull policy based on debug mode.
func (cpi *ControlPlaneInstaller) getImagePullPolicy() string {
	if cpi.Opts.Debug {
		return "Always"
	}
	return "IfNotPresent"
}

// getControllerDeployment returns the Kubernetes deployment resource for a
// controller.
func (cpi *ControlPlaneInstaller) getControllerDeployment(
	deployName string,
	namespace string,
	controller v0.ControlPlaneComponent,
) (*unstructured.Unstructured, error) {

	controllerImage := cpi.getImage(controller.Name, controller.ImageName, controller.ImageNamespace, controller.ImageTag)
	controllerVols, controllerVolMounts, err := cpi.getControllerVolumes(controller)
	if err != nil {
		return nil, fmt.Errorf("could not get vols for controller %s: %w", controller.Name, err)
	}

	controllerArgs := cpi.getControllerArgs()
	controllerImagePullSecrets := cpi.getImagePullSecrets(controller.ImagePullSecretName)

	ports := []map[string]interface{}{}

	envFrom := []interface{}{
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
	}

	envRef := make([]map[string]interface{}, 0)
	if controller.AdditionalEnvRef != nil {
		var v []map[string]interface{}
		err := json.Unmarshal([]byte(*controller.AdditionalEnvRef), &v)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		envRef = v
	}

	for _, e := range envRef {
		envFrom = append(envFrom, e)
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
						"serviceAccountName": controller.ServiceAccountName,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            controller.Name,
								"image":           controllerImage,
								"command":         cpi.getCommand(controller.BinaryName),
								"imagePullPolicy": cpi.getImagePullPolicy(),
								"args":            controllerArgs,
								"envFrom":         envFrom,
								"volumeMounts":    controllerVolMounts,
								"readinessProbe":  cpi.getReadinessProbe(),
								"ports":           ports,
							},
						},
						"imagePullSecrets": controllerImagePullSecrets,
						"volumes":          controllerVols,
					},
				},
			},
		},
	}, nil
}

func (cpi *ControlPlaneInstaller) getReadinessProbe() map[string]interface{} {
	return map[string]interface{}{
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
	}
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

// getCommand returns the args that are passed to the container.
func (cpi *ControlPlaneInstaller) getCommand(name string) []interface{} {
	return []interface{}{
		fmt.Sprintf("/%s", name),
	}
}
