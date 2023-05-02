package threeport

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/version"
	v0 "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	ThreeportImageRepo               = "ghcr.io/threeport"
	ThreeportAPIImage                = "threeport-rest-api"
	ThreeportWorkloadControllerImage = "threeport-workload-controller"
	ThreeportAPIIngressResourceName  = "threeport-api-ingress"
	ThreeportLocalAPIEndpoint        = "localhost"
	ThreeportLocalAPIPort            = "1323"
)

// ThreeportDevImages returns a map of main package dirs to image names
func ThreeportDevImages() map[string]string {
	return map[string]string{
		"rest-api":            fmt.Sprintf("%s-dev:latest", ThreeportAPIImage),
		"workload-controller": fmt.Sprintf("%s-dev:latest", ThreeportWorkloadControllerImage),
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
	authConfig *config.AuthConfig,
) error {
	// install the API
	if err := InstallThreeportAPI(
		kubeClient,
		mapper,
		devEnvironment,
		apiHostname,
		customThreeportImageRepo,
		authConfig,
	); err != nil {
		return fmt.Errorf("failed to install threeport API server: %w", err)
	}

	// install the controllers
	if err := InstallThreeportControllers(
		kubeClient,
		mapper,
		devEnvironment,
		customThreeportImageRepo,
		authConfig,
	); err != nil {
		return fmt.Errorf("failed to install threeport controllers: %w", err)
	}

	return nil
}

// InstallThreeportControlPlaneAPI installs the threeport API in a Kubernetes
// cluster.
func InstallThreeportAPI(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	apiHostname string,
	customThreeportImageRepo string,
	authConfig *config.AuthConfig,
) error {
	apiImage := getAPIImage(devEnvironment, customThreeportImageRepo)
	apiArgs := getAPIArgs(devEnvironment)
	apiVols, apiVolMounts := getAPIVolumes(devEnvironment)
	// apiPort := getAPIPort(devEnvironment)

	if authConfig != nil {
		// generate server certificate
		serverCertificate, serverPrivateKey, err := config.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
		if err != nil {
			cli.Error("failed to generate server certificate and private key", err)
			fmt.Errorf("failed to generate random serial number: %w", err)
			os.Exit(1)
		}

		var tlsApiCA = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"type":       "kubernetes.io/tls",
				"metadata": map[string]interface{}{
					"name":      "tls-api-ca",
					"namespace": ControlPlaneNamespace,
				},
				"stringData": map[string]interface{}{
					"tls.crt": authConfig.CAPemEncoded,
					"tls.key": authConfig.CAPrivateKeyPemEncoded,
				},
			},
		}
		if _, err := kube.CreateResource(tlsApiCA, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
		}

		var tlsApiCert = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"type":       "kubernetes.io/tls",
				"metadata": map[string]interface{}{
					"name":      "tls-api-cert",
					"namespace": ControlPlaneNamespace,
				},
				"stringData": map[string]interface{}{
					"tls.crt": serverCertificate,
					"tls.key": serverPrivateKey,
				},
			},
		}
		if _, err := kube.CreateResource(tlsApiCert, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
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
										"name":          "api",
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
				"type": "NodePort",
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "http",
						"port":       1323,
						"protocol":   "TCP",
						"targetPort": 1323,
						"nodePort":   30000,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(apiService, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create API server service: %w", err)
	}

	return nil
}

// InstallThreeportControlPlaneControllers installs the threeport controllers in
// a Kubernetes cluster
func InstallThreeportControllers(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	customThreeportImageRepo string,
	authConfig *config.AuthConfig,
) error {
	workloadControllerImage := getWorkloadControllerImage(devEnvironment, customThreeportImageRepo)
	workloadControllerVols, workloadControllerVolMounts := getWorkloadControllerVolumes(devEnvironment)
	workloadArgs := getWorkloadArgs(devEnvironment)

	if authConfig != nil {

		// generate workload certificate
		workloadCertificate, workloadPrivateKey, err := config.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
		if err != nil {
			cli.Error("failed to generate client certificate and private key", err)
			os.Exit(1)
		}

		var tlsApiCA = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"type":       "kubernetes.io/tls",
				"metadata": map[string]interface{}{
					"name":      "tls-api-ca",
					"namespace": ControlPlaneNamespace,
				},
				"stringData": map[string]interface{}{
					"tls.crt": authConfig.CAPemEncoded,
					"tls.key": "",
				},
			},
		}
		if _, err := kube.CreateResource(tlsApiCA, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
		}

		var tlsApiCert = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"type":       "kubernetes.io/tls",
				"metadata": map[string]interface{}{
					"name":      "tls-api-cert",
					"namespace": ControlPlaneNamespace,
				},
				"stringData": map[string]interface{}{
					"tls.crt": workloadCertificate,
					"tls.key": workloadPrivateKey,
				},
			},
		}
		if _, err := kube.CreateResource(tlsApiCert, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
		}
	}

	var workloadControllerSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "workload-controller-config",
				"namespace": ControlPlaneNamespace,
			},
			"type": "Opaque",
			"stringData": map[string]interface{}{
				"API_SERVER":      "threeport-api-server:1323",
				"MSG_BROKER_HOST": "nats-js",
				"MSG_BROKER_PORT": "4222",
			},
		},
	}
	if _, err := kube.CreateResource(workloadControllerSecret, kubeClient, *mapper); err != nil {
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
								"args":            workloadArgs,
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "workload-controller-config",
										},
									},
								},
								"volumeMounts": workloadControllerVolMounts,
							},
						},
						"volumes": workloadControllerVols,
					},
				},
			},
		},
	}
	if _, err := kube.CreateResource(workloadControllerDeployment, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create workload controller deployment: %w", err)
	}

	return nil
}

// WaitForThreeportAPI waits for the threeport API to respond to a request.
func WaitForThreeportAPI(apiClient *http.Client, apiEndpoint string) error {
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
			fmt.Println(err)
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

// getAPIImage returns the proper container image to use for the API.
func getAPIImage(devEnvironment bool, customThreeportImageRepo string) string {
	if devEnvironment {
		devImages := ThreeportDevImages()
		return devImages["rest-api"]
	}

	imageRepo := ThreeportImageRepo
	if customThreeportImageRepo != "" {
		imageRepo = customThreeportImageRepo
	}
	apiImage := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		ThreeportAPIImage,
		version.GetVersion(),
	)

	return apiImage
}

// getAPIIngressAnnotations returns the annotaions for the API ingress resource.
func getAPIIngressAnnotations(devEnvironment bool) map[string]interface{} {
	if devEnvironment {
		return map[string]interface{}{}
	}

	return map[string]interface{}{
		"cert-manager.io/cluster-issuer": "letsencrypt-staging",
	}
}

// getAPIIngressTLS returns the proper API ingress TLS object.
func getAPIIngressTLS(devEnvironment bool, apiHostname string) []interface{} {
	if devEnvironment {
		return []interface{}{}
	}

	return []interface{}{
		map[string]interface{}{
			"hosts": []interface{}{
				apiHostname,
			},
		},
	}
}

// getAPIArgs returns the args that are passed to the API server.
func getAPIArgs(devEnvironment bool) []interface{} {
	if devEnvironment {
		return []interface{}{
			"-build.args_bin",
			"-auto-migrate=true -verbose=true -auth-enabled=true",
		}
	}

	return []interface{}{
		"-auto-migrate",
		"true",
	}
}

// getWorkloadArgs returns the args that are passed to the workload controller.
func getWorkloadArgs(devEnvironment bool) []interface{} {
	if devEnvironment {
		return []interface{}{
			"-build.args_bin",
			"-auth-enabled=true",
		}
	}

	return []interface{}{
		"-build.args_bin",
		"-auth-enabled=false",
	}
}

// getAPIVolumes returns volumes and volume mounts for the API server.
func getAPIVolumes(devEnvironment bool) ([]interface{}, []interface{}) {
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
		map[string]interface{}{
			"name": "tls-api-ca",
			"secret": map[string]interface{}{
				"secretName": "tls-api-ca",
			},
		},
		map[string]interface{}{
			"name": "tls-api-cert",
			"secret": map[string]interface{}{
				"secretName": "tls-api-cert",
			},
		},
	}

	volMounts := []interface{}{
		map[string]interface{}{
			"name":      "db-config",
			"mountPath": "/etc/threeport/",
		},
		map[string]interface{}{
			"name":      "tls-api-ca",
			"mountPath": "/etc/threeport/ca",
		},
		map[string]interface{}{
			"name":      "tls-api-cert",
			"mountPath": "/etc/threeport/cert",
		},
	}

	if devEnvironment {
		codePathVol, codePathVolMount := getCodePathVols()
		vols = append(vols, codePathVol)
		volMounts = append(volMounts, codePathVolMount)
	}

	return vols, volMounts
}

// getWorkloadControllerImage returns the proper container image to use for the
// workload controller.
func getWorkloadControllerImage(devEnvironment bool, customThreeportImageRepo string) string {
	if devEnvironment {
		devImages := ThreeportDevImages()
		return devImages["workload-controller"]
	}

	imageRepo := ThreeportImageRepo
	if customThreeportImageRepo != "" {
		imageRepo = customThreeportImageRepo
	}
	workloadControllerImage := fmt.Sprintf(
		"%s/%s:%s",
		imageRepo,
		ThreeportWorkloadControllerImage,
		version.GetVersion(),
	)

	return workloadControllerImage
}

// getWorkloadControllerVolumes returns the volumes and volume mounts for the workload
// controller.
func getWorkloadControllerVolumes(devEnvironment bool) ([]interface{}, []interface{}) {
	vols := []interface{}{
		map[string]interface{}{
			"name": "tls-api-ca",
			"secret": map[string]interface{}{
				"secretName": "tls-api-ca",
			},
		},
		map[string]interface{}{
			"name": "tls-api-cert",
			"secret": map[string]interface{}{
				"secretName": "tls-api-cert",
			},
		},
	}
	volMounts := []interface{}{
		map[string]interface{}{
			"name":      "tls-api-ca",
			"mountPath": "/etc/threeport/ca",
		},
		map[string]interface{}{
			"name":      "tls-api-cert",
			"mountPath": "/etc/threeport/cert",
		},
	}

	if devEnvironment {
		codePathVol, codePathVolMount := getCodePathVols()
		vols = append(vols, codePathVol)
		volMounts = append(volMounts, codePathVolMount)
	}

	return vols, volMounts
}

// getCodePathVols returns the volume and volume mount for dev environments to
// mount local codebase for live reloads.
func getCodePathVols() (map[string]interface{}, map[string]interface{}) {
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

	return codePathVol, codePathVolMount
}
