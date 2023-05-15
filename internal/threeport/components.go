package threeport

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/version"
	"github.com/threeport/threeport/pkg/auth/v0"
	v0 "github.com/threeport/threeport/pkg/client/v0"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	ThreeportImageRepo               = "ghcr.io/threeport"
	ThreeportAPIImage                = "threeport-rest-api"
	ThreeportWorkloadControllerImage = "threeport-workload-controller"
	ThreeportAPIServiceResourceName  = "threeport-api-server"
	ThreeportAPIIngressResourceName  = "threeport-api-ingress"
	ThreeportLocalAPIEndpoint        = "localhost"
	ThreeportLocalAPIPort            = "443"
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
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
	infraProvider string,
) error {
	// install the API
	if err := InstallThreeportAPI(
		kubeClient,
		mapper,
		devEnvironment,
		apiHostname,
		customThreeportImageRepo,
		customThreeportImageTag,
		authConfig,
		infraProvider,
	); err != nil {
		return fmt.Errorf("failed to install threeport API server: %w", err)
	}

	// install the controllers
	if err := InstallThreeportControllers(
		kubeClient,
		mapper,
		devEnvironment,
		customThreeportImageRepo,
		customThreeportImageTag,
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
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
	infraProvider string,
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
				"name":      ThreeportAPIServiceResourceName,
				"namespace": ControlPlaneNamespace,
			},
			"annotations": apiServiceAnnotations,
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app.kubernetes.io/name": "threeport-api-server",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"name":       apiServicePortName,
						"port":       apiServicePort,
						"protocol":   "TCP",
						"targetPort": 1323,
						"nodePort":   30000,
					},
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

// InstallThreeportControlPlaneControllers installs the threeport controllers in
// a Kubernetes cluster
func InstallThreeportControllers(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	devEnvironment bool,
	customThreeportImageRepo string,
	customThreeportImageTag string,
	authConfig *auth.AuthConfig,
) error {
	workloadControllerImage := getWorkloadControllerImage(devEnvironment, customThreeportImageRepo, customThreeportImageTag)
	workloadControllerVols, workloadControllerVolMounts := getWorkloadControllerVolumes(devEnvironment, authConfig)
	workloadArgs := getWorkloadArgs(devEnvironment, authConfig)

	if authConfig != nil {

		// generate workload certificate
		workloadCertificate, workloadPrivateKey, err := auth.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
		if err != nil {
			return fmt.Errorf("failed to generate client certificate and private key: %w", err)
		}

		var workloadCa = getTLSSecret("workload-ca", authConfig.CAPemEncoded, "")
		if _, err := kube.CreateResource(workloadCa, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to create API server secret: %w", err)
		}

		var workloadCert = getTLSSecret("workload-cert", workloadCertificate, workloadPrivateKey)
		if _, err := kube.CreateResource(workloadCert, kubeClient, *mapper); err != nil {
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
				"API_SERVER":      "threeport-api-server",
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
		return nil, fmt.Errorf("failed to Kubernetes service resource for threeport API: %w", err)
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

// getWorkloadArgs returns the args that are passed to the workload controller.
func getWorkloadArgs(devEnvironment bool, authConfig *auth.AuthConfig) []interface{} {

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
		codePathVol, codePathVolMount := getCodePathVols()
		vols = append(vols, codePathVol)
		volMounts = append(volMounts, codePathVolMount)
	}

	return vols, volMounts
}

// getWorkloadControllerImage returns the proper container image to use for the
// workload controller.
func getWorkloadControllerImage(devEnvironment bool, customThreeportImageRepo, customThreeportImageTag string) string {
	if devEnvironment {
		devImages := ThreeportDevImages()
		return devImages["workload-controller"]
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
		ThreeportWorkloadControllerImage,
		imageTag,
	)

	return workloadControllerImage
}

// getWorkloadControllerVolumes returns the volumes and volume mounts for the workload
// controller.
func getWorkloadControllerVolumes(devEnvironment bool, authConfig *auth.AuthConfig) ([]interface{}, []interface{}) {
	vols := []interface{}{}
	volMounts := []interface{}{}

	if authConfig != nil {
		caVol, caVolMount := getSecretVols("workload-ca", "/etc/threeport/ca")
		certVol, certVolMount := getSecretVols("workload-cert", "/etc/threeport/cert")

		vols = append(vols, caVol)
		vols = append(vols, certVol)
		volMounts = append(volMounts, caVolMount)
		volMounts = append(volMounts, certVolMount)
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
