package v0

import (
	"context"
	"fmt"
	"net/http"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	api "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	moduleInstallConfigKey  = "config.yaml"
	moduleInstallConfigPath = "/root/.threeport/config.yaml"
	moduleWorkloadPrefix    = "threeport-"
)

// moduleInstallConfigFile is the config the install job reads.
// The field names match the CLI config, which has no yaml tags.
type moduleInstallConfigFile struct {
	CurrentControlPlane string
	ControlPlanes       []moduleInstallControlPlane
}

// moduleInstallControlPlane is one control plane entry in that config.
type moduleInstallControlPlane struct {
	Name          string
	AuthEnabled   bool
	APIServer     string
	CACert        string
	EncryptionKey string
	Credentials   []moduleInstallCredential
}

// moduleInstallCredential is the client certificate for that control plane.
type moduleInstallCredential struct {
	Name       string
	ClientCert string
	ClientKey  string
}

// HTTPClientForInstance builds an API client from the certificates stored on a control plane.
// An empty client certificate means auth is off.
func HTTPClientForInstance(inst *api.ControlPlaneInstance) (*http.Client, error) {
	// auth is off when the row has no client certificate
	if util.DerefString(inst.ClientCert) == "" {
		return client_lib.GetHTTPClient(false, "", "", "", "")
	}

	// decode the certificates the row stores as base64
	caCert, err := util.Base64Decode(util.DerefString(inst.CACert))
	if err != nil {
		return nil, fmt.Errorf("failed to decode ca certificate: %w", err)
	}
	clientCert, err := util.Base64Decode(*inst.ClientCert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client certificate: %w", err)
	}
	clientKey, err := util.Base64Decode(util.DerefString(inst.ClientKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decode client key: %w", err)
	}

	return client_lib.GetHTTPClient(true, caCert, clientCert, clientKey, "")
}

// ReconcileInstalledModuleImages applies recorded module images to the deployments that exist.
// A missing API deployment becomes an install job when the module names an image.
func ReconcileInstalledModuleImages(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	apiClient *http.Client,
	apiEndpoint string,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
) error {
	// list the modules and the controllers that belong to them
	modules, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module APIs: %w", err)
	}
	controllers, err := client.GetModuleControllers(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module controllers: %w", err)
	}
	byAPI := map[uint][]api.ModuleController{}
	if controllers != nil {
		for _, controller := range *controllers {
			if controller.ModuleApiID == nil {
				continue
			}
			byAPI[*controller.ModuleApiID] = append(byAPI[*controller.ModuleApiID], controller)
		}
	}

	// retag each module that has image coordinates
	if modules == nil {
		return nil
	}
	for _, module := range *modules {
		// bring a stopped module back even when it has no recorded image
		kebab := ModuleKebab(util.DerefString(module.Name))
		workloadNamespace := moduleWorkloadNamespace(&module)
		imageNamespace, imageTag := recordedModuleImages(&module)
		controlPlaneNamespace := util.DerefString(inst.Namespace)
		if _, err := refreshModuleTrust(
			kubeClient, workloadNamespace, controlPlaneNamespace, inst,
		); err != nil {
			return err
		}
		if err := retargetModuleDeployment(
			kubeClient, workloadNamespace, ModuleAPIDeploymentName(kebab), controlPlaneNamespace,
		); err != nil {
			return err
		}
		if imageNamespace == "" || imageTag == "" {
			if err := resumeModuleByName(
				kubeClient, workloadNamespace, ModuleAPIDeploymentName(kebab),
			); err != nil {
				return err
			}
		} else if err := retagModuleAPI(
			kubeClient, mapper, inst, cpi, &module, imageNamespace, imageTag,
		); err != nil {
			return err
		}
		if module.ID == nil {
			continue
		}
		for _, controller := range byAPI[*module.ID] {
			namespace, recorded, splitErr := splitDeploymentName(util.DerefString(controller.DeploymentName))
			if splitErr == nil {
				if name, nameErr := installedModuleDeployment(kubeClient, namespace, recorded, kebab); nameErr == nil {
					if err := retargetModuleDeployment(
						kubeClient, namespace, name, controlPlaneNamespace,
					); err != nil {
						return err
					}
				}
			}
			if imageNamespace == "" || imageTag == "" {
				if err := resumeRecordedController(kubeClient, &controller, kebab); err != nil {
					return err
				}
				continue
			}
			if err := retagModuleController(
				kubeClient, mapper, &controller, kebab, imageNamespace, imageTag,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// retagModuleAPI restarts the API deployment when its image differs.
// A missing deployment is installed instead.
func retagModuleAPI(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
	module *api.ModuleApi,
	imageNamespace, imageTag string,
) error {
	// read the API deployment
	kebab := ModuleKebab(util.DerefString(module.Name))
	namespace := moduleWorkloadNamespace(module)
	name := ModuleAPIDeploymentName(kebab)
	deployment, err := kube.GetResource(
		"apps", "v1", "Deployment", namespace, name, kubeClient, *mapper,
	)
	if k8serrors.IsNotFound(err) {
		return EnsureModuleInstallJob(kubeClient, mapper, inst, cpi, module)
	}
	if err != nil {
		return fmt.Errorf("failed to get module API deployment %s/%s: %w", namespace, name, err)
	}
	if err := resumeStoppedDeployment(kubeClient, namespace, deployment); err != nil {
		return err
	}

	// leave a deployment whose containers are missing or already current
	apiImage := ModuleRestAPIImage(imageNamespace, kebab, imageTag)
	migratorImage := ModuleMigratorImage(imageNamespace, kebab, imageTag)
	currentAPI, apiOK := namedContainerImage(deployment, "containers", moduleAPIContainerName)
	currentMigrator, migratorOK := namedContainerImage(
		deployment, "initContainers", moduleMigratorContainerName,
	)
	if !apiOK || !migratorOK || (currentAPI == apiImage && currentMigrator == migratorImage) {
		return nil
	}

	// restart onto the recorded images
	return restartModuleDeployment(
		kubeClient,
		namespace,
		name,
		map[string]string{
			"containers":     apiImage,
			"initContainers": migratorImage,
		},
		map[string]string{
			"containers":     moduleAPIContainerName,
			"initContainers": moduleMigratorContainerName,
		},
		time.Now().UTC(),
	)
}

// retagModuleController restarts one controller deployment when its image differs.
// A missing controller deployment is left alone.
func retagModuleController(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	controller *api.ModuleController,
	kebab, imageNamespace, imageTag string,
) error {
	// read the controller deployment the install created
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
	deployment, err := kube.GetResource(
		"apps", "v1", "Deployment", namespace, name, kubeClient, *mapper,
	)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get module controller %s/%s: %w", namespace, name, err)
	}
	if err := resumeStoppedDeployment(kubeClient, namespace, deployment); err != nil {
		return err
	}

	// leave a deployment whose container is missing or already current
	container := ModuleControllerContainerName(name)
	image := ModuleControllerImage(imageNamespace, name, imageTag)
	current, ok := namedContainerImage(deployment, "containers", container)
	if !ok || current == image {
		return nil
	}

	// restart onto the recorded image
	return restartModuleDeployment(
		kubeClient,
		namespace,
		name,
		map[string]string{"containers": image},
		map[string]string{"containers": container},
		time.Now().UTC(),
	)
}

// EnsureModuleInstallJob creates the install job when the module is not already installing.
// A failed job is replaced. A running or finished job is left in place.
func EnsureModuleInstallJob(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
	module *api.ModuleApi,
) error {
	// leave a job that is running or finished
	kebab := ModuleKebab(util.DerefString(module.Name))
	namespace := controlPlaneNamespace(inst, cpi)
	name := moduleInstallJobName(kebab)
	existing, err := kube.GetResource(
		"batch", "v1", "Job", namespace, name, kubeClient, *mapper,
	)
	if err == nil && !jobFailed(existing) {
		return nil
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to get module install job %s: %w", name, err)
	}

	// replace a failed job so its pods are removed
	if err == nil {
		if err := deleteModuleInstallJob(kubeClient, namespace, name); err != nil {
			return err
		}
	}

	// write the config the install reads, then create the job
	if err := ensureModuleInstallConfig(
		kubeClient, mapper, inst, cpi, namespace, kebab,
	); err != nil {
		return err
	}
	_, err = kube.CreateResource(
		moduleInstallJob(namespace, kebab, module), kubeClient, *mapper,
	)
	if err != nil {
		return fmt.Errorf("failed to create module install job %s: %w", name, err)
	}

	return nil
}

// UninstallModule deletes the module namespace and then the module API row.
func UninstallModule(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	apiClient *http.Client,
	apiEndpoint string,
	module *api.ModuleApi,
) error {
	// delete the workload namespace
	namespace := moduleWorkloadNamespace(module)
	ns := &unstructured.Unstructured{}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")
	ns.SetName(namespace)
	if err := kube.DeleteResource(ns, kubeClient, *mapper); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete module namespace %s: %w", namespace, err)
	}

	// delete the module API row
	if module.ID == nil {
		return fmt.Errorf("failed to delete module %s: id is empty", util.DerefString(module.Name))
	}
	if _, err := client.DeleteModuleApi(apiClient, apiEndpoint, *module.ID); err != nil {
		return fmt.Errorf("failed to delete module %s: %w", util.DerefString(module.Name), err)
	}

	return nil
}

// WorkloadRESTMapper returns a mapper for the workloads a control plane upgrade touches.
func WorkloadRESTMapper() meta.RESTMapper {
	// register the kinds the upgrade reads and writes
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		meta.RESTScopeNamespace,
	)
	mapper.Add(
		schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		meta.RESTScopeNamespace,
	)

	return mapper
}

// moduleWorkloadNamespace returns the namespace the module workloads run in.
func moduleWorkloadNamespace(module *api.ModuleApi) string {
	// prefer the namespace the module endpoint records
	if server, ok := moduleApiServerFromEndpoint(module.Endpoint); ok && server.Namespace != "" {
		return server.Namespace
	}

	return moduleWorkloadPrefix + ModuleKebab(util.DerefString(module.Name))
}

// moduleInstallJobName returns the install job name for one module.
func moduleInstallJobName(kebab string) string {
	return "module-install-" + kebab
}

// moduleInstallSecretName returns the config secret name for one module.
func moduleInstallSecretName(kebab string) string {
	return "module-install-" + kebab + "-config"
}

// controlPlaneNamespace returns the namespace the control plane runs in.
func controlPlaneNamespace(inst *api.ControlPlaneInstance, cpi *ControlPlaneInstaller) string {
	// the installer option wins when the caller set one
	if cpi != nil && cpi.Opts.Namespace != "" {
		return cpi.Opts.Namespace
	}

	return util.DerefString(inst.Namespace)
}

// namedContainerImage returns the image of one container.
func namedContainerImage(obj *unstructured.Unstructured, listKey, containerName string) (string, bool) {
	// walk the named container list
	list, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", listKey)
	if err != nil || !found {
		return "", false
	}
	for _, item := range list {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(container, "name")
		if name != containerName {
			continue
		}
		image, _, _ := unstructured.NestedString(container, "image")
		return image, image != ""
	}

	return "", false
}

// jobFailed reports whether a job has failed and is neither running nor finished.
func jobFailed(job *unstructured.Unstructured) bool {
	// a succeeded or active job stays
	succeeded, _, _ := unstructured.NestedInt64(job.Object, "status", "succeeded")
	if succeeded > 0 {
		return false
	}
	active, _, _ := unstructured.NestedInt64(job.Object, "status", "active")
	if active > 0 {
		return false
	}

	// only a failed count means replace the job
	failed, _, _ := unstructured.NestedInt64(job.Object, "status", "failed")
	return failed > 0
}

// deleteModuleInstallJob removes a failed install job and its pods.
func deleteModuleInstallJob(kubeClient dynamic.Interface, namespace, name string) error {
	// background deletion removes the pods with the job
	policy := metav1.DeletePropagationBackground
	err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete module install job %s: %w", name, err)
	}

	return nil
}

// ensureModuleInstallConfig writes the config secret the install job mounts.
func ensureModuleInstallConfig(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
	namespace, kebab string,
) error {
	// build the config the CLI reads from its home directory
	configYAML, err := moduleInstallConfig(kubeClient, mapper, inst, cpi, namespace)
	if err != nil {
		return err
	}
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      moduleInstallSecretName(kebab),
			"namespace": namespace,
		},
		"data": map[string]interface{}{
			moduleInstallConfigKey: util.Base64Encode(string(configYAML)),
		},
	}}
	if _, err := kube.CreateOrUpdateResource(secret, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to write module install config: %w", err)
	}

	return nil
}

// moduleInstallConfig renders the config file for one control plane.
func moduleInstallConfig(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
	namespace string,
) ([]byte, error) {
	// read the decoded encryption key the CLI stores in its config
	key, err := cpi.EncryptionKeyFromNamespace(kubeClient, mapper, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to read encryption key: %w", err)
	}
	name := util.DerefString(inst.Name)
	authEnabled := util.DerefString(inst.ClientCert) != ""
	config := moduleInstallConfigFile{
		CurrentControlPlane: name,
		ControlPlanes: []moduleInstallControlPlane{{
			Name:          name,
			AuthEnabled:   authEnabled,
			APIServer:     util.DerefString(inst.ApiServerEndpoint),
			CACert:        util.DerefString(inst.CACert),
			EncryptionKey: key,
			Credentials: []moduleInstallCredential{{
				Name:       name,
				ClientCert: util.DerefString(inst.ClientCert),
				ClientKey:  util.DerefString(inst.ClientKey),
			}},
		}},
	}

	return yaml.Marshal(config)
}

// moduleInstallJob returns the job that runs one module install.
func moduleInstallJob(namespace, kebab string, module *api.ModuleApi) *unstructured.Unstructured {
	// label the pod the database admits, mount the config, and run the install
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      moduleInstallJobName(kebab),
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"backoffLimit": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{LabelMaintenance: "true"},
				},
				"spec": map[string]interface{}{
					"restartPolicy": "Never",
					"containers": []interface{}{
						map[string]interface{}{
							"name": "install",
							"image": ModuleTptctlImage(
								util.DerefString(module.ImageNamespace),
								kebab,
								util.DerefString(module.ImageTag),
							),
							"args": []interface{}{kebab, "install"},
							"env": []interface{}{
								map[string]interface{}{"name": "HOME", "value": "/root"},
							},
							"volumeMounts": []interface{}{
								map[string]interface{}{
									"name":      "config",
									"mountPath": moduleInstallConfigPath,
									"subPath":   moduleInstallConfigKey,
								},
							},
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "config",
							"secret": map[string]interface{}{
								"secretName": moduleInstallSecretName(kebab),
							},
						},
					},
				},
			},
		},
	}}
}
