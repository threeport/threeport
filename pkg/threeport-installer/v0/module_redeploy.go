package v0

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	api "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// RedeployInstalledModules points each non-core module at this control plane.
// The previous control plane and its namespace stay in place.
func (cpi *ControlPlaneInstaller) RedeployInstalledModules(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	if mapper == nil {
		return fmt.Errorf("failed to retarget installed modules: no REST mapper")
	}

	// list the module APIs restored with the database
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module APIs: %w", err)
	}

	// list the controllers separately because the API list does not preload them
	moduleControllers, err := client.GetModuleControllers(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module controllers: %w", err)
	}
	controllersByAPI := map[uint][]string{}
	if moduleControllers != nil {
		for _, moduleController := range *moduleControllers {
			if moduleController.ModuleApiID == nil || moduleController.DeploymentName == nil {
				continue
			}
			controllersByAPI[*moduleController.ModuleApiID] = append(
				controllersByAPI[*moduleController.ModuleApiID],
				*moduleController.DeploymentName,
			)
		}
	}

	// retarget each non-core module
	if moduleApis == nil {
		return nil
	}
	for _, moduleAPI := range *moduleApis {
		if err := cpi.redeployInstalledModule(
			kubeClient,
			*mapper,
			moduleAPI,
			controllersByAPI,
			time.Now(),
		); err != nil {
			return err
		}
	}

	return nil
}

// redeployInstalledModule copies connection secrets, admits the module to the
// database, and restarts its workloads. A core module is left as installed.
func (cpi *ControlPlaneInstaller) redeployInstalledModule(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
	moduleAPI api.ModuleApi,
	controllersByAPI map[uint][]string,
	restartedAt time.Time,
) error {
	// leave a core module on the control plane installer
	if moduleAPI.Core != nil && *moduleAPI.Core {
		return nil
	}

	// read the module namespace from its registered endpoint
	apiServer, ok := moduleApiServerFromEndpoint(moduleAPI.Endpoint)
	if !ok {
		return fmt.Errorf(
			"module %s has no cluster endpoint to retarget",
			util.DerefString(moduleAPI.Name),
		)
	}
	kebab := ModuleKebab(util.DerefString(moduleAPI.Name))
	if kebab == "" {
		return fmt.Errorf(
			"module %s has no image name",
			util.DerefString(moduleAPI.Name),
		)
	}

	// copy the secrets the installer copies
	for _, secretName := range ModuleCopiedSecretNames() {
		if err := CopySecret(
			kubeClient,
			mapper,
			secretName,
			cpi.Opts.Namespace,
			apiServer.Namespace,
		); err != nil {
			return fmt.Errorf(
				"failed to copy secret %s into %s: %w",
				secretName,
				apiServer.Namespace,
				err,
			)
		}
	}

	// write the module API config for this namespace
	if _, err := kube.CreateOrUpdateResource(
		ModuleApiConfigSecret(apiServer.Namespace, apiServer.Service, cpi.Opts.AuthEnabled),
		kubeClient,
		mapper,
	); err != nil {
		return fmt.Errorf("failed to write module API config in %s: %w", apiServer.Namespace, err)
	}

	// admit the module pods to this control plane database
	service, err := kubeClient.Resource(serviceGVR).Namespace(apiServer.Namespace).Get(
		context.Background(),
		apiServer.Service,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf(
			"failed to get module API service %s/%s: %w",
			apiServer.Namespace,
			apiServer.Service,
			err,
		)
	}
	selector, _, err := unstructured.NestedStringMap(service.Object, "spec", "selector")
	if err != nil {
		return fmt.Errorf(
			"failed to read the selector of service %s/%s: %w",
			apiServer.Namespace,
			apiServer.Service,
			err,
		)
	}
	if err := InstallModuleDatabasePolicy(
		kubeClient,
		mapper,
		cpi.Opts.Namespace,
		apiServer.Namespace,
		apiServer.Service,
		selector,
	); err != nil {
		return fmt.Errorf("failed to install the module database policy: %w", err)
	}

	// set images when both are recorded, then restart
	imageNamespace, imageTag := recordedModuleImages(&moduleAPI)
	if err := restartModuleDeployment(
		kubeClient,
		apiServer.Namespace,
		ModuleAPIDeploymentName(kebab),
		map[string]string{
			"containers":     ModuleRestAPIImage(imageNamespace, kebab, imageTag),
			"initContainers": ModuleMigratorImage(imageNamespace, kebab, imageTag),
		},
		map[string]string{
			"containers":     moduleAPIContainerName,
			"initContainers": moduleMigratorContainerName,
		},
		restartedAt,
	); err != nil {
		return err
	}

	// restart each controller
	if moduleAPI.ID == nil {
		return nil
	}
	for _, deploymentName := range controllersByAPI[*moduleAPI.ID] {
		namespace, recorded, splitErr := splitDeploymentName(deploymentName)
		if splitErr != nil {
			return fmt.Errorf("module %s: %w", util.DerefString(moduleAPI.Name), splitErr)
		}
		name, err := installedModuleDeployment(kubeClient, namespace, recorded, kebab)
		if err != nil {
			return fmt.Errorf("module %s: %w", util.DerefString(moduleAPI.Name), err)
		}
		if err := restartModuleDeployment(
			kubeClient,
			namespace,
			name,
			map[string]string{
				"containers": ModuleControllerImage(imageNamespace, name, imageTag),
			},
			map[string]string{
				"containers": ModuleControllerContainerName(name),
			},
			restartedAt,
		); err != nil {
			return err
		}
	}

	return nil
}

// recordedModuleImages returns the repository and tag stored on a module API.
// A core module, or a module missing either value, returns empty strings.
func recordedModuleImages(moduleAPI *api.ModuleApi) (string, string) {
	// leave the deployment image unless both coordinates are recorded
	if moduleAPI == nil || (moduleAPI.Core != nil && *moduleAPI.Core) {
		return "", ""
	}

	namespace := strings.TrimSpace(util.DerefString(moduleAPI.ImageNamespace))
	tag := strings.TrimSpace(util.DerefString(moduleAPI.ImageTag))
	if namespace == "" || tag == "" {
		return "", ""
	}

	return namespace, tag
}

// splitDeploymentName splits a recorded namespace/name deployment reference.
func splitDeploymentName(deploymentName string) (string, string, error) {
	namespace, name, ok := strings.Cut(deploymentName, "/")
	if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("deployment %q is not namespace/name", deploymentName)
	}

	return namespace, name, nil
}

// restartModuleDeployment sets the named container images and restarts the deployment.
// An empty image leaves that container unchanged.
func restartModuleDeployment(
	kubeClient dynamic.Interface,
	namespace string,
	name string,
	images map[string]string,
	containerNames map[string]string,
	restartedAt time.Time,
) error {
	// read the deployment
	deployment, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(),
		name,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	// bring a deployment stopped for the backup back to its recorded count
	if err := restoreStoppedReplicas(deployment); err != nil {
		return fmt.Errorf("failed to restore replicas on deployment %s/%s: %w", namespace, name, err)
	}

	// set each recorded image
	for listKey, image := range images {
		if err := SetNamedContainerImage(deployment, listKey, containerNames[listKey], image); err != nil {
			return fmt.Errorf("failed to set the image on deployment %s/%s: %w", namespace, name, err)
		}
	}

	// restart so the init containers run against this database
	if err := RestartDeployment(deployment, restartedAt); err != nil {
		return fmt.Errorf("failed to restart deployment %s/%s: %w", namespace, name, err)
	}
	if _, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Update(
		context.Background(),
		deployment,
		metav1.UpdateOptions{},
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to update deployment %s/%s: %w", namespace, name, err)
	}

	return nil
}

// installedModuleDeployment returns the Deployment a module install created.
// The recorded name omits the module, so the installed name is tried second.
func installedModuleDeployment(
	kubeClient dynamic.Interface,
	namespace string,
	recorded string,
	kebab string,
) (string, error) {
	// try the recorded name, then the name the install actually created
	var lastErr error
	for _, name := range moduleDeploymentNames(recorded, kebab) {
		_, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
			context.Background(), name, metav1.GetOptions{},
		)
		if err == nil {
			return name, nil
		}
		if !k8serrors.IsNotFound(err) {
			return "", err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("deployment %s/%s was not found", namespace, recorded)
	}
	return "", fmt.Errorf("failed to get deployment %s/%s: %w", namespace, recorded, lastErr)
}

// moduleDeploymentNames is the recorded name, then the installed name.
func moduleDeploymentNames(recorded, kebab string) []string {
	names := []string{recorded}
	const prefix = "threeport-"
	if kebab == "" || !strings.HasPrefix(recorded, prefix) {
		return names
	}
	prefixed := prefix + kebab + "-" + strings.TrimPrefix(recorded, prefix)
	if prefixed == recorded {
		return names
	}
	return append(names, prefixed)
}

// restoreStoppedReplicas sets a stopped Deployment back to its recorded count.
// A missing record comes back at one replica.
func restoreStoppedReplicas(deployment *unstructured.Unstructured) error {
	// leave a running deployment at its current count
	replicas, _, err := unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	if err != nil {
		return err
	}
	if replicas > 0 {
		return nil
	}

	// use the count recorded before the stop
	count := int64(1)
	recorded := deployment.GetAnnotations()[replicasBeforeRestoreAnnotation]
	if parsed, parseErr := strconv.ParseInt(recorded, 10, 64); parseErr == nil && parsed > 0 {
		count = parsed
	}
	if err := unstructured.SetNestedField(deployment.Object, count, "spec", "replicas"); err != nil {
		return err
	}
	annotations := deployment.GetAnnotations()
	delete(annotations, replicasBeforeRestoreAnnotation)
	deployment.SetAnnotations(annotations)
	return nil
}

// controlPlaneHost matches a control plane service address in a restored manifest.
var controlPlaneHost = regexp.MustCompile(
	`(crdb(?:-0\.crdb)?|nats-js|threeport-api-server)\.[a-z0-9-]+\.svc\.cluster\.local`,
)

// retargetControlPlaneHost points a restored address at the replacement plane.
func retargetControlPlaneHost(value, namespace string) string {
	if namespace == "" {
		return value
	}
	return controlPlaneHost.ReplaceAllString(value, "${1}."+namespace+".svc.cluster.local")
}

// retargetHostsIn rewrites control plane addresses in a deployment object.
func retargetHostsIn(value interface{}, namespace string) bool {
	switch typed := value.(type) {
	case string:
		return false
	case map[string]interface{}:
		changed := false
		for key, item := range typed {
			if text, ok := item.(string); ok {
				next := retargetControlPlaneHost(text, namespace)
				if next != text {
					typed[key] = next
					changed = true
				}
				continue
			}
			if retargetHostsIn(item, namespace) {
				changed = true
			}
		}
		return changed
	case []interface{}:
		changed := false
		for i, item := range typed {
			if text, ok := item.(string); ok {
				next := retargetControlPlaneHost(text, namespace)
				if next != text {
					typed[i] = next
					changed = true
				}
				continue
			}
			if retargetHostsIn(item, namespace) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

// trustBundle joins the API CA and the broker CA the module client verifies.
func trustBundle(apiCA, natsCA string) (string, error) {
	if natsCA == "" || natsCA == apiCA {
		return apiCA, nil
	}
	apiPEM, err := base64.StdEncoding.DecodeString(apiCA)
	if err != nil {
		return "", err
	}
	natsPEM, err := base64.StdEncoding.DecodeString(natsCA)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(append(append(apiPEM, '\n'), natsPEM...)), nil
}

// refreshModuleTrust copies the replacement plane's trust into the module namespace.
// A module without those secrets is left alone.
func refreshModuleTrust(
	kubeClient dynamic.Interface,
	moduleNamespace, controlPlaneNamespace string,
	inst *api.ControlPlaneInstance,
) (bool, error) {
	// the API CA is the one the replacement server presents
	apiCA, err := secretData(kubeClient, controlPlaneNamespace, "api-ca", "tls.crt")
	if err != nil || apiCA == "" {
		return false, err
	}
	natsCA, err := secretData(kubeClient, controlPlaneNamespace, "nats-ca-cert", "ca.crt")
	if err != nil {
		return false, err
	}
	bundle, err := trustBundle(apiCA, natsCA)
	if err != nil {
		return false, fmt.Errorf("failed to build the module trust bundle: %w", err)
	}
	changed, err := setSecretData(kubeClient, moduleNamespace, "threeport-api-ca", "tls.crt", bundle)
	if err != nil {
		return false, err
	}

	// copy the broker CA the module mounts, then the client pair in one write
	if natsCA != "" {
		if _, err := setSecretData(kubeClient, moduleNamespace, brokerCASecret, brokerCAKey, natsCA); err != nil {
			return false, err
		}
	}
	cert := util.DerefString(inst.ClientCert)
	key := util.DerefString(inst.ClientKey)
	if cert == "" || key == "" {
		return changed, nil
	}
	if err := clientCredential(cert, key); err != nil {
		return false, err
	}
	clientChanged, err := setSecretEntries(kubeClient, moduleNamespace, "threeport-api-client-cert", map[string]string{
		"tls.crt": cert,
		"tls.key": key,
	})
	if err != nil {
		return false, err
	}
	return changed || clientChanged, nil
}

// clientCredential checks that the stored certificate and key are one pair.
func clientCredential(cert, key string) error {
	// decode the stored values and refuse a pair that will not load
	pemCert, err := base64.StdEncoding.DecodeString(cert)
	if err != nil {
		return fmt.Errorf("failed to decode the module client certificate: %w", err)
	}
	pemKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return fmt.Errorf("failed to decode the module client key: %w", err)
	}
	if _, err := tls.X509KeyPair(pemCert, pemKey); err != nil {
		return fmt.Errorf("module client certificate does not match its key: %w", err)
	}
	return nil
}

// secretData reads one key from a secret.
func secretData(kubeClient dynamic.Interface, namespace, name, key string) (string, error) {
	secret, err := kubeClient.Resource(secretGVR).Namespace(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}
	value, _, err := unstructured.NestedString(secret.Object, "data", key)
	return value, err
}

// setSecretData writes one secret key when the stored value differs.
func setSecretData(kubeClient dynamic.Interface, namespace, name, key, value string) (bool, error) {
	return setSecretEntries(kubeClient, namespace, name, map[string]string{key: value})
}

// setSecretEntries writes every key in one update when a stored value differs.
func setSecretEntries(
	kubeClient dynamic.Interface, namespace, name string, values map[string]string,
) (bool, error) {
	secret, err := kubeClient.Resource(secretGVR).Namespace(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}

	// replace only the keys that differ, then write the secret once
	changed := false
	for key, value := range values {
		current, _, err := unstructured.NestedString(secret.Object, "data", key)
		if err != nil {
			return false, err
		}
		if current == value {
			continue
		}
		if err := unstructured.SetNestedField(secret.Object, value, "data", key); err != nil {
			return false, err
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	if _, err := kubeClient.Resource(secretGVR).Namespace(namespace).Update(
		context.Background(), secret, metav1.UpdateOptions{},
	); err != nil {
		return false, fmt.Errorf("failed to update secret %s/%s: %w", namespace, name, err)
	}
	return true, nil
}

const (
	brokerCASecret     = "nats-ca-cert"
	brokerCAKey        = "ca.crt"
	brokerCAVolume     = "nats-ca"
	brokerCAMount      = "/etc/threeport/nats-ca"
	brokerCASystemFile = "/etc/ssl/certs/threeport-nats-ca.crt"
)

// ensureBrokerCA mounts the broker CA for a module that verifies the server itself.
// A module built before that file still uses the system pool, so the same CA is mounted there too.
func ensureBrokerCA(deployment *unstructured.Unstructured) bool {
	spec, _, err := unstructured.NestedMap(deployment.Object, "spec", "template", "spec")
	if err != nil || spec == nil {
		return false
	}

	// add the secret volume when the deployment does not already have it
	changed := false
	volumes, _, _ := unstructured.NestedSlice(spec, "volumes")
	if !namedObject(volumes, brokerCAVolume) {
		volumes = append(volumes, map[string]interface{}{
			"name": brokerCAVolume,
			"secret": map[string]interface{}{
				"secretName": brokerCASecret,
			},
		})
		_ = unstructured.SetNestedSlice(spec, volumes, "volumes")
		changed = true
	}

	// mount the CA where the current module reads it and where the system pool scans
	containers, _, _ := unstructured.NestedSlice(spec, "containers")
	for i, item := range containers {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		mounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
		added := false
		if !mountNamed(mounts, brokerCAMount) {
			mounts = append(mounts, map[string]interface{}{
				"name":      brokerCAVolume,
				"mountPath": brokerCAMount,
				"readOnly":  true,
			})
			added = true
		}
		if !mountNamed(mounts, brokerCASystemFile) {
			mounts = append(mounts, map[string]interface{}{
				"name":      brokerCAVolume,
				"mountPath": brokerCASystemFile,
				"subPath":   brokerCAKey,
				"readOnly":  true,
			})
			added = true
		}
		if !added {
			continue
		}
		_ = unstructured.SetNestedSlice(container, mounts, "volumeMounts")
		containers[i] = container
		changed = true
	}
	if !changed {
		return false
	}
	_ = unstructured.SetNestedSlice(spec, containers, "containers")
	_ = unstructured.SetNestedMap(deployment.Object, spec, "spec", "template", "spec")
	return true
}

// namedObject reports whether a list already holds an object with that name.
func namedObject(items []interface{}, name string) bool {
	for _, item := range items {
		object, ok := item.(map[string]interface{})
		if ok && object["name"] == name {
			return true
		}
	}
	return false
}

// mountNamed reports whether a container already mounts that path.
func mountNamed(items []interface{}, path string) bool {
	for _, item := range items {
		object, ok := item.(map[string]interface{})
		if ok && object["mountPath"] == path {
			return true
		}
	}
	return false
}

// retargetModuleDeployment points a module Deployment at the replacement plane.
// A missing Deployment is left alone.
func retargetModuleDeployment(
	kubeClient dynamic.Interface,
	namespace, name, controlPlaneNamespace string,
) error {
	// read the deployment the install created
	deployment, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	// write the deployment when an address or the broker CA mount is missing
	changed := retargetHostsIn(deployment.Object, controlPlaneNamespace)
	if ensureBrokerCA(deployment) {
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = kubeClient.Resource(deploymentGVR).Namespace(namespace).Update(
		context.Background(), deployment, metav1.UpdateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to retarget deployment %s/%s: %w", namespace, name, err)
	}
	return nil
}

// resumeStoppedDeployment writes a stopped Deployment back to its recorded count.
// A running Deployment is left as it is.
func resumeStoppedDeployment(
	kubeClient dynamic.Interface,
	namespace string,
	deployment *unstructured.Unstructured,
) error {
	// leave a deployment that is already running
	replicas, _, err := unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	if err != nil {
		return err
	}
	if replicas > 0 {
		return nil
	}

	// write the restored count
	if err := restoreStoppedReplicas(deployment); err != nil {
		return err
	}
	_, err = kubeClient.Resource(deploymentGVR).Namespace(namespace).Update(
		context.Background(), deployment, metav1.UpdateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to resume deployment %s/%s: %w", namespace, deployment.GetName(), err)
	}
	return nil
}

// resumeModuleByName brings one stopped module Deployment back up.
// A missing Deployment is left alone.
func resumeModuleByName(kubeClient dynamic.Interface, namespace, name string) error {
	// read the deployment the install created
	deployment, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}
	return resumeStoppedDeployment(kubeClient, namespace, deployment)
}

// resumeRecordedController brings the installed controller Deployment back up.
// A missing controller is left alone.
func resumeRecordedController(
	kubeClient dynamic.Interface,
	controller *api.ModuleController,
	kebab string,
) error {
	// find the deployment the install created
	namespace, recorded, err := splitDeploymentName(util.DerefString(controller.DeploymentName))
	if err != nil {
		return nil
	}
	name, err := installedModuleDeployment(kubeClient, namespace, recorded, kebab)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return resumeModuleByName(kubeClient, namespace, name)
}
