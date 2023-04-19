package threeport

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/version"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	ThreeportImageRepo                   = "ghcr.io/threeport"
	ThreeportAPIImage                    = "threeport-rest-api"
	ThreeportWorkloadControllerImage     = "threeport-workload-controller"
	ThreeportEthereumNodeControllerImage = "threeport-ethereum-node-controller"
	ThreeportAPIIngressResourceName      = "threeport-api-ingress"
	ThreeportLocalAPIEndpoint            = "localhost"
	ThreeportLocalAPIProtocol            = "http"
)

// ThreeportDevImages returns a map of main package dirs to image names
func ThreeportDevImages() map[string]string {
	return map[string]string{
		"rest-api":                 fmt.Sprintf("%s-dev:latest", ThreeportAPIImage),
		"workload-controller":      fmt.Sprintf("%s-dev:latest", ThreeportWorkloadControllerImage),
		"ethereum-node-controller": fmt.Sprintf("%s-dev:latest", ThreeportEthereumNodeControllerImage),
	}
}

// InstallThreeportControlPlaneComponents installs the threeport API and
// controllers.
func InstallThreeportControlPlaneComponents(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	apiHostname string,
	customThreeportImageRepo string,
) error {
	var apiImage string
	var workloadControllerImage string
	var ethereumNodeControllerImage string
	var apiIngressAnnotations map[string]interface{}
	var apiIngressTLS []interface{}
	var apiArgs []interface{}
	apiVols, apiVolMounts := apiVolumes()
	controllerVols, controllerVolMounts := controllerVolumes()
	if devEnvironment {
		// set dev environment images
		devImages := ThreeportDevImages()
		apiImage = devImages["rest-api"]
		workloadControllerImage = devImages["workload-controller"]
		ethereumNodeControllerImage = devImages["ethereum-node-controller"]

		// set dev environment code mount into container
		codePathVol := map[string]interface{}{
			"name": "code-path",
			"hostPath": map[string]interface{}{
				"path": "/threeport",
				"type": "Directory",
			},
		}
		codePathVolMount := map[string]interface{}{
			"name":      "code-path",
			"mountPath": "/threeport",
		}
		apiVols = append(apiVols, codePathVol)
		apiVolMounts = append(apiVolMounts, codePathVolMount)
		controllerVols = append(controllerVols, codePathVol)
		controllerVolMounts = append(controllerVolMounts, codePathVolMount)
	} else {
		imageRepo := ThreeportImageRepo
		if customThreeportImageRepo != "" {
			imageRepo = customThreeportImageRepo
		}
		apiImage = fmt.Sprintf(
			"%s/%s:%s",
			imageRepo,
			ThreeportAPIImage,
			version.GetVersion(),
		)
		workloadControllerImage = fmt.Sprintf(
			"%s/%s:%s",
			imageRepo,
			ThreeportWorkloadControllerImage,
			version.GetVersion(),
		)
		ethereumNodeControllerImage = fmt.Sprintf(
			"%s/%s:%s",
			imageRepo,
			ThreeportEthereumNodeControllerImage,
			version.GetVersion(),
		)
		apiIngressAnnotations = map[string]interface{}{
			"cert-manager.io/cluster-issuer": "letsencrypt-staging",
		}
		apiIngressTLS = []interface{}{
			map[string]interface{}{
				"hosts": []interface{}{
					apiHostname,
				},
			},
		}
		apiArgs = []interface{}{
			"-auto-migrate",
			"true",
		}
	}

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
										"name":          "http",
										"protocol":      "TCP",
									},
								},
								"volumeMounts": apiVolMounts,
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

	var apiService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "threeport-api-server",
				"namespace": ControlPlaneNamespace,
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app.kubernetes.io/name": "threeport-api-server",
				},
				//"type": "LoadBalancer",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       80,
						"protocol":   "TCP",
						"targetPort": 1323,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(apiService, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API server service: %w", err)
	}

	var controllerSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "controller-config",
				"namespace": ControlPlaneNamespace,
			},
			"type": "Opaque",
			"stringData": map[string]interface{}{
				"API_SERVER":      "http://threeport-api-server",
				"MSG_BROKER_HOST": "nats-js",
				"MSG_BROKER_PORT": "4222",
			},
		},
	}
	if _, err := kube.CreateResource(controllerSecret, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create workload controller secret: %w", err)
	}

	var workloadControllerDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "threeport-workload-controller",
				"namespace": ControlPlaneNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": "threeport-workload-controller",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": "threeport-workload-controller",
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "workload-controller",
								"image":           workloadControllerImage,
								"imagePullPolicy": "IfNotPresent",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "controller-config",
										},
									},
								},
								"volumeMounts": controllerVolMounts,
							},
						},
						"volumes": controllerVols,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(workloadControllerDeployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create workload controller deployment: %w", err)
	}

	var ethereumNodeControllerDeployment = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "threeport-ethereum-node-controller",
				"namespace": ControlPlaneNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": "threeport-ethereum-node-controller",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": "threeport-ethereum-node-controller",
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":            "ethereum-node-controller",
								"image":           ethereumNodeControllerImage,
								"imagePullPolicy": "IfNotPresent",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "controller-config",
										},
									},
								},
								"volumeMounts": controllerVolMounts,
							},
						},
						"volumes": controllerVols,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(ethereumNodeControllerDeployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create ethereum node controller deployment: %w", err)
	}

	var apiIngress = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":        ThreeportAPIIngressResourceName,
				"namespace":   ControlPlaneNamespace,
				"annotations": apiIngressAnnotations,
			},
			"spec": map[string]interface{}{
				"ingressClassName": "kong",
				"rules": []interface{}{
					map[string]interface{}{
						"host": apiHostname,
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path":     "/",
									"pathType": "Prefix",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": "threeport-api-server",
											"port": map[string]interface{}{
												"number": 80,
											},
										},
									},
								},
							},
						},
					},
				},
				"tls": apiIngressTLS,
			},
		},
	}
	if _, err := kube.CreateResource(apiIngress, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API ingress: %w", err)
	}

	return nil
}

// WaitForThreeportAPI waits for the threeport API to respond to a request.
func WaitForThreeportAPI(apiEndpoint string) error {
	attempts := 0
	maxAttempts := 30
	waitSeconds := 10
	apiReady := false
	for attempts < maxAttempts {
		testResp, err := http.Get(fmt.Sprintf("%s/version", apiEndpoint))
		if err != nil {
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}
		if testResp.StatusCode != http.StatusOK {
			time.Sleep(time.Second * time.Duration(waitSeconds))
			attempts += 1
			continue
		}
		apiReady = true
		break
	}
	if !apiReady {
		return fmt.Errorf(
			"timed out waiting for threeport API to become ready: %w",
			errors.New(fmt.Sprintf("%d seconds elapsed without 200 response from threeport API", maxAttempts*waitSeconds)),
		)
	}

	return nil
}

func apiVolumes() ([]interface{}, []interface{}) {
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

	return vols, volMounts
}

func controllerVolumes() ([]interface{}, []interface{}) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	return vols, volMounts
}
