package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

const dbCredsSecretName = "db-certs"

// CreateThreeportControlPlaneNamespace creates the threeport control plane
// namespace in a Kubernetes cluster.
func (cpi *ControlPlaneInstaller) CreateThreeportControlPlaneNamespace(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	var namespace = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": cpi.Opts.Namespace,
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(namespace, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	return nil
}

// InstallThreeportControlPlaneDependencies installs the necessary components
// for the threeport REST API and controllers to operate.  It includes the
// database and message broker.
func (cpi *ControlPlaneInstaller) InstallThreeportControlPlaneDependencies(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	infraProvider,
	encryptionKey string,
	dbCreds *auth.DbCreds,
) error {
	if err := cpi.CreateThreeportControlPlaneNamespace(kubeClient, mapper); err != nil {
		return fmt.Errorf("failed in create threeport control plane namespace: %w", err)
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

	var natsPDB = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata": map[string]interface{}{
				"name":      "nats-js",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":     "nats",
					"app.kubernetes.io/instance": "nats-js",
					"app.kubernetes.io/version":  "2.9.3",
				},
			},
			"spec": map[string]interface{}{
				"maxUnavailable": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name":     "nats",
						"app.kubernetes.io/instance": "nats-js",
					},
				},
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(natsPDB, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var natsServiceAccount = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name":      "nats-js",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":     "nats",
					"app.kubernetes.io/instance": "nats-js",
					"app.kubernetes.io/version":  "2.9.3",
				},
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(natsServiceAccount, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var natsConfig = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "nats-js-config",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":     "nats",
					"app.kubernetes.io/instance": "nats-js",
					"app.kubernetes.io/version":  "2.9.3",
				},
			},
			"data": map[string]interface{}{
				"nats.conf": `# NATS Clients Port
port: 4222

# PID file shared with configuration reloader.
pid_file: "/var/run/nats/nats.pid"

###############
#             #
# Monitoring  #
#             #
###############
http: 8222
server_name:$POD_NAME
###################################
#                                 #
# NATS JetStream                  #
#                                 #
###################################
jetstream {
  max_mem: 30Mi
}
lame_duck_grace_period: 10s
lame_duck_duration: 30s
store_dir: /data
`,
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(natsConfig, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var natsService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "nats-js",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":     "nats",
					"app.kubernetes.io/instance": "nats-js",
					"app.kubernetes.io/version":  "2.9.3",
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app.kubernetes.io/name":     "nats",
					"app.kubernetes.io/instance": "nats-js",
				},
				"clusterIP":                "None",
				"publishNotReadyAddresses": true,
				"ports": []interface{}{
					map[string]interface{}{
						"name":        "client",
						"port":        4222,
						"appProtocol": "tcp",
					},
					map[string]interface{}{
						"name":        "cluster",
						"port":        6222,
						"appProtocol": "tcp",
					},
					map[string]interface{}{
						"name":        "monitor",
						"port":        8222,
						"appProtocol": "http",
					},
					map[string]interface{}{
						"name":        "metrics",
						"port":        7777,
						"appProtocol": "http",
					},
					map[string]interface{}{
						"name":        "leafnodes",
						"port":        7422,
						"appProtocol": "tcp",
					},
					map[string]interface{}{
						"name":        "gateways",
						"port":        7522,
						"appProtocol": "tcp",
					},
				},
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(natsService, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var natsBoxDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "nats-js-box",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app":   "nats-js-box",
					"chart": "nats-0.18.2",
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "nats-js-box",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "nats-js-box",
						},
					},
					"spec": map[string]interface{}{
						"volumes": nil,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "nats-box",
								"image":           "natsio/nats-box:0.14.3",
								"imagePullPolicy": "IfNotPresent",
								"resources":       nil,
								"env": []interface{}{
									map[string]interface{}{
										"name":  "NATS_URL",
										"value": "nats-js",
									},
								},
								"command": []interface{}{
									"tail",
									"-f",
									"/dev/null",
								},
								"volumeMounts": nil,
							},
						},
					},
				},
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(natsBoxDeployment, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var natsStatefulSet = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      "nats-js",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"helm.sh/chart":                "nats-0.18.2",
					"app.kubernetes.io/name":       "nats",
					"app.kubernetes.io/instance":   "nats-js",
					"app.kubernetes.io/version":    "2.9.3",
					"app.kubernetes.io/managed-by": "Helm",
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name":     "nats",
						"app.kubernetes.io/instance": "nats-js",
					},
				},
				"replicas":            1,
				"serviceName":         "nats-js",
				"podManagementPolicy": "Parallel",
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"prometheus.io/path":   "/metrics",
							"prometheus.io/port":   "7777",
							"prometheus.io/scrape": "true",
							"checksum/config":      "3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7",
						},
						"labels": map[string]interface{}{
							"app.kubernetes.io/name":     "nats",
							"app.kubernetes.io/instance": "nats-js",
						},
					},
					"spec": map[string]interface{}{
						// Common volumes for the containers.
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "config-volume",
								"configMap": map[string]interface{}{
									"name": "nats-js-config",
								},
							},
							// Local volume shared with the reloader.
							map[string]interface{}{
								"name":     "pid",
								"emptyDir": map[string]interface{}{},
							},
						},
						//################
						//               #
						//  TLS Volumes  #
						//               #
						//################

						"serviceAccountName": "nats-js",
						// Required to be able to HUP signal and apply config
						// reload to the server without restarting the pod.
						"shareProcessNamespace": true,
						//################
						//               #
						//  NATS Server  #
						//               #
						//################
						"terminationGracePeriodSeconds": 60,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "nats",
								"image":           "nats:2.10.12-alpine",
								"imagePullPolicy": "IfNotPresent",
								"resources":       map[string]interface{}{},
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 4222,
										"name":          "client",
									},
									map[string]interface{}{
										"containerPort": 6222,
										"name":          "cluster",
									},
									map[string]interface{}{
										"containerPort": 8222,
										"name":          "monitor",
									},
								},
								"command": []interface{}{
									"nats-server",
									"--config",
									"/etc/nats-config/nats.conf",
								},
								// Required to be able to define an environment variable
								// that refers to other environment variables.  This env var
								// is later used as part of the configuration file.
								"env": []interface{}{
									map[string]interface{}{
										"name": "POD_NAME",
										"valueFrom": map[string]interface{}{
											"fieldRef": map[string]interface{}{
												"fieldPath": "metadata.name",
											},
										},
									},
									map[string]interface{}{
										"name":  "SERVER_NAME",
										"value": "$(POD_NAME)",
									},
									map[string]interface{}{
										"name": "POD_NAMESPACE",
										"valueFrom": map[string]interface{}{
											"fieldRef": map[string]interface{}{
												"fieldPath": "metadata.namespace",
											},
										},
									},
									map[string]interface{}{
										"name":  "CLUSTER_ADVERTISE",
										"value": "$(POD_NAME).nats-js.$(POD_NAMESPACE).svc.cluster.local",
									},
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "config-volume",
										"mountPath": "/etc/nats-config",
									},
									map[string]interface{}{
										"name":      "pid",
										"mountPath": "/var/run/nats",
									},
								},
								//######################
								//                     #
								// Healthcheck Probes  #
								//                     #
								//######################
								//"livenessProbe": map[string]interface{}{
								//	"failureThreshold": 3,
								//	"httpGet": map[string]interface{}{
								//		"path": "/",
								//		"port": 8222,
								//	},
								//	"initialDelaySeconds": 10,
								//	"periodSeconds":       30,
								//	"successThreshold":    1,
								//	"timeoutSeconds":      5,
								//},
								"readinessProbe": map[string]interface{}{
									"failureThreshold": 3,
									"httpGet": map[string]interface{}{
										"path": "/",
										"port": 8222,
									},
									"initialDelaySeconds": 10,
									"periodSeconds":       10,
									"successThreshold":    1,
									"timeoutSeconds":      5,
								},
								"startupProbe": map[string]interface{}{
									// for NATS server versions >=2.7.1, /healthz will be enabled
									// startup probe checks that the JS server is enabled, is current with the meta leader,
									// and that all streams and consumers assigned to this JS server are current
									"failureThreshold": 30,
									"httpGet": map[string]interface{}{
										"path": "/healthz",
										"port": 8222,
									},
									"initialDelaySeconds": 10,
									"periodSeconds":       10,
									"successThreshold":    1,
									"timeoutSeconds":      5,
								},
								// Gracefully stop NATS Server on pod deletion or image upgrade.
								//
								"lifecycle": map[string]interface{}{
									"preStop": map[string]interface{}{
										"exec": map[string]interface{}{
											// send the lame duck shutdown signal to trigger a graceful shutdown
											// nats-server will ignore the TERM signal it receives after this
											//
											"command": []interface{}{
												"nats-server",
												"-sl=ldm=/var/run/nats/nats.pid",
											},
										},
									},
								},
							},
							//################################
							//                               #
							//  NATS Configuration Reloader  #
							//                               #
							//################################
							map[string]interface{}{
								"name":            "reloader",
								"image":           "natsio/nats-server-config-reloader:0.14.1",
								"imagePullPolicy": "IfNotPresent",
								"resources":       nil,
								"command": []interface{}{
									"nats-server-config-reloader",
									"-pid",
									"/var/run/nats/nats.pid",
									"-config",
									"/etc/nats-config/nats.conf",
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "config-volume",
										"mountPath": "/etc/nats-config",
									},
									map[string]interface{}{
										"name":      "pid",
										"mountPath": "/var/run/nats",
									},
									map[string]interface{}{
										"name":      "datadir",
										"mountPath": "/data",
									},
								},
							},
							//#############################
							//                            #
							//  NATS Prometheus Exporter  #
							//                            #
							//#############################
							map[string]interface{}{
								"name":            "metrics",
								"image":           "natsio/prometheus-nats-exporter:0.14.0",
								"imagePullPolicy": "IfNotPresent",
								"resources":       map[string]interface{}{},
								"args": []interface{}{
									"-connz",
									"-routez",
									"-subz",
									"-varz",
									"-prefix=nats",
									"-use_internal_server_id",
									"-jsz=all",
									"http://localhost:8222/",
								},
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 7777,
										"name":          "metrics",
									},
								},
							},
						},
					},
				},
				"volumeClaimTemplates": []interface{}{
					map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": "datadir",
						},
						"spec": cpi.getVolClaimTemplateSpec(infraProvider, "5Gi"),
					},
				},
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(natsStatefulSet, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	// asdf
	var dbCertsSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      dbCredsSecretName,
				"namespace": cpi.Opts.Namespace,
			},
			"stringData": map[string]interface{}{
				"ca.crt":               dbCreds.AuthConfig.CAPemEncoded,
				"node.crt":             dbCreds.NodeCert,
				"node.key":             dbCreds.NodeKey,
				"client.root.crt":      dbCreds.RootCert,
				"client.root.key":      dbCreds.RootKey,
				"client.threeport.crt": dbCreds.ThreeportCert,
				"client.threeport.key": dbCreds.ThreeportKey,
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(dbCertsSecret, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create DB certs secret: %w", err)
	}

	var crdbPDB = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "PodDisruptionBudget",
			"apiVersion": "policy/v1",
			"metadata": map[string]interface{}{
				"name":      "crdb-budget",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":     "cockroachdb",
					"app.kubernetes.io/instance": "crdb",
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name":      "cockroachdb",
						"app.kubernetes.io/instance":  "crdb",
						"app.kubernetes.io/component": "cockroachdb",
					},
				},
				"maxUnavailable": 1,
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(crdbPDB, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var crdbService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata": map[string]interface{}{
				"name":      database.ThreeportDatabaseHost,
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":      "cockroachdb",
					"app.kubernetes.io/instance":  "crdb",
					"app.kubernetes.io/component": "cockroachdb",
				},
				"annotations": map[string]interface{}{
					// Use this annotation in addition to the actual field below because the
					// annotation will stop being respected soon, but the field is broken in
					// some versions of Kubernetes:
					// https://github.com/kubernetes/kubernetes/issues/58662
					"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
					// Enable automatic monitoring of all instances when Prometheus is running
					// in the cluster.
					"prometheus.io/scrape": "true",
					"prometheus.io/path":   "_status/vars",
					"prometheus.io/port":   "8080",
				},
			},
			"spec": map[string]interface{}{
				"clusterIP": "None",
				// We want all Pods in the StatefulSet to have their addresses published for
				// the sake of the other CockroachDB Pods even before they're ready, since they
				// have to be able to talk to each other in order to become ready.
				"publishNotReadyAddresses": true,
				"ports": []interface{}{
					// The main port, served by gRPC, serves Postgres-flavor SQL, inter-node
					// traffic and the CLI.
					map[string]interface{}{
						"name":       "grpc",
						"port":       26257,
						"targetPort": "grpc",
					},
					// The secondary port serves the UI as well as health and debug endpoints.
					map[string]interface{}{
						"name":       "http",
						"port":       8080,
						"targetPort": "http",
					},
				},
				"selector": map[string]interface{}{
					"app.kubernetes.io/name":      "cockroachdb",
					"app.kubernetes.io/instance":  "crdb",
					"app.kubernetes.io/component": "cockroachdb",
				},
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(crdbService, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	var crdbStatefulSet = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "StatefulSet",
			"apiVersion": "apps/v1",
			"metadata": map[string]interface{}{
				"name":      "crdb",
				"namespace": cpi.Opts.Namespace,
				"labels": map[string]interface{}{
					"helm.sh/chart":                "cockroachdb-10.0.2",
					"app.kubernetes.io/name":       "cockroachdb",
					"app.kubernetes.io/instance":   "crdb",
					"app.kubernetes.io/managed-by": "Helm",
					"app.kubernetes.io/component":  "cockroachdb",
				},
			},
			"spec": map[string]interface{}{
				"serviceName": "crdb",
				"replicas":    1,
				"updateStrategy": map[string]interface{}{
					"type": "RollingUpdate",
				},
				"podManagementPolicy": "Parallel",
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name":      "cockroachdb",
						"app.kubernetes.io/instance":  "crdb",
						"app.kubernetes.io/component": "cockroachdb",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name":      "cockroachdb",
							"app.kubernetes.io/instance":  "crdb",
							"app.kubernetes.io/component": "cockroachdb",
						},
					},
					"spec": map[string]interface{}{
						"affinity": map[string]interface{}{
							"podAntiAffinity": map[string]interface{}{
								"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
									map[string]interface{}{
										"weight": 100,
										"podAffinityTerm": map[string]interface{}{
											"topologyKey": "kubernetes.io/hostname",
											"labelSelector": map[string]interface{}{
												"matchLabels": map[string]interface{}{
													"app.kubernetes.io/name":      "cockroachdb",
													"app.kubernetes.io/instance":  "crdb",
													"app.kubernetes.io/component": "cockroachdb",
												},
											},
										},
									},
								},
							},
						},
						"topologySpreadConstraints": []interface{}{
							map[string]interface{}{
								"labelSelector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"app.kubernetes.io/name":      "cockroachdb",
										"app.kubernetes.io/instance":  "crdb",
										"app.kubernetes.io/component": "cockroachdb",
									},
								},
								"maxSkew":           1,
								"topologyKey":       "topology.kubernetes.io/zone",
								"whenUnsatisfiable": "ScheduleAnyway",
							},
						},
						// No pre-stop hook is required, a SIGTERM plus some time is all that's
						// needed for graceful shutdown of a node.
						"terminationGracePeriodSeconds": 60,
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "db",
								"image":           fmt.Sprintf("cockroachdb/cockroach:%s", DatabaseImageTag),
								"imagePullPolicy": "IfNotPresent",
								"args": []interface{}{
									"shell",
									"-ecx",
									// The use of qualified `hostname -f` is crucial:
									// Other nodes aren't able to look up the unqualified hostname.
									//
									// `--join` CLI flag is hardcoded to exactly 3 Pods, because:
									// 1. Having `--join` value depending on `statefulset.replicas`
									//    will trigger undesired restart of existing Pods when
									//    StatefulSet is scaled up/down. We want to scale without
									//    restarting existing Pods.
									// 2. At least one Pod in `--join` is enough to successfully
									//    join CockroachDB cluster and gossip with all other existing
									//    Pods, even if there are 3 or more Pods.
									// 3. It's harmless for `--join` to have 3 Pods even for 1-Pod
									//    clusters, while it gives us opportunity to scale up even if
									//    some Pods of existing cluster are down (for whatever reason).
									// See details explained here:
									// https://github.com/helm/charts/pull/18993#issuecomment-558795102
									"exec /cockroach/cockroach start-single-node --certs-dir=/cockroach/cockroach-certs --advertise-host=$(hostname).${STATEFULSET_FQDN} --http-port=8080 --port=26257 --cache=25% --max-sql-memory=25% --logtostderr=INFO",
								},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "STATEFULSET_NAME",
										"value": "crdb",
									},
									map[string]interface{}{
										"name":  "STATEFULSET_FQDN",
										"value": "crdb.threeport-control-plane.svc.cluster.local",
									},
									map[string]interface{}{
										"name":  "COCKROACH_CHANNEL",
										"value": "kubernetes-secure",
									},
								},
								"ports": []interface{}{
									map[string]interface{}{
										"name":          "grpc",
										"containerPort": 26257,
										"protocol":      "TCP",
									},
									map[string]interface{}{
										"name":          "http",
										"containerPort": 8080,
										"protocol":      "TCP",
									},
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "datadir",
										"mountPath": "/cockroach/cockroach-data/",
									},
									map[string]interface{}{
										"name":      "db-certs",
										"mountPath": "/cockroach/cockroach-certs",
									},
								},
								"readinessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/health?ready=1",
										"port": "http",
									},
									"initialDelaySeconds": 10,
									"periodSeconds":       5,
									"failureThreshold":    2,
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "datadir",
								"persistentVolumeClaim": map[string]interface{}{
									"claimName": "datadir",
								},
							},
							map[string]interface{}{
								"name": "db-certs",
								"secret": map[string]interface{}{
									"secretName":  dbCredsSecretName,
									"defaultMode": 256,
								},
							},
						},
					},
				},
				"volumeClaimTemplates": []interface{}{
					map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": "datadir",
							"labels": map[string]interface{}{
								"app.kubernetes.io/name":     "cockroachdb",
								"app.kubernetes.io/instance": "crdb",
							},
						},
						"spec": cpi.getVolClaimTemplateSpec(infraProvider, "20Gi"),
					},
				},
			},
		},
	}

	if err := cpi.CreateOrUpdateKubeResource(crdbStatefulSet, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server secret for workload controller: %w", err)
	}

	// configure threeport api service
	apiServicePortName, apiServicePort := cpi.GetAPIServicePort()

	// configure node port based on infra provider
	port := map[string]interface{}{
		"name":       apiServicePortName,
		"port":       apiServicePort,
		"protocol":   "TCP",
		"targetPort": 1323,
	}
	if cpi.Opts.InfraProvider == "kind" && !cpi.Opts.InThreeport {
		port["nodePort"] = 30000
	}
	var apiService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":        cpi.Opts.RestApiInfo.ServiceResourceName,
				"namespace":   cpi.Opts.Namespace,
				"annotations": cpi.getAPIServiceAnnotations(),
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app.kubernetes.io/name": cpi.Opts.RestApiInfo.ServiceResourceName,
				},
				"ports": []interface{}{
					port,
				},
				"type": cpi.getAPIServiceType(),
			},
		},
	}
	if err := cpi.CreateOrUpdateKubeResource(apiService, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create/update API server service: %w", err)
	}

	return nil
}

// getVolClaimTemplateSpec returns the spec for volume claim template for the
// specified provider with the specified storage amount.
func (cpi *ControlPlaneInstaller) getVolClaimTemplateSpec(
	infraProvider string,
	storage string,
) map[string]interface{} {
	volClaimTemplateSpec := map[string]interface{}{
		"accessModes": []interface{}{
			"ReadWriteOnce",
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": storage,
			},
		},
	}

	if infraProvider == "eks" {
		volClaimTemplateSpec["storageClassName"] = "gp2"
	}

	return volClaimTemplateSpec
}
