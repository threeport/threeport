package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// ErrModuleInstallRunning is returned when a module install job runs a different
// image than the one requested, so the request stays pending until that job ends.
var ErrModuleInstallRunning = errors.New("module install is still running")

// ErrModuleInstallFailed is returned when a module install job has failed as many
// times as the controller retries, so the request is cleared and a corrected one
// is accepted.
var ErrModuleInstallFailed = errors.New("module install failed")

const (
	// moduleInstallMaxAttempts is how many jobs the controller runs for one request.
	moduleInstallMaxAttempts = 3

	// moduleInstallDeadlineSeconds is how long one install job may run. A pod that
	// cannot pull its image never fails on its own, so the deadline fails the job.
	moduleInstallDeadlineSeconds = int64(900)

	// moduleInstallAttemptsAnnotation records which attempt a job is.
	moduleInstallAttemptsAnnotation = "threeport.io/module-install-attempts"

	// moduleInstallAbandonedAnnotation marks a failed job the controller gave up on.
	moduleInstallAbandonedAnnotation = "threeport.io/module-install-abandoned"
)

// ModuleInstallJobEnv names the environment variable the install job sets so the
// module plugin installs on the control plane it is connected to.
const ModuleInstallJobEnv = "THREEPORT_MODULE_INSTALL_JOB"

const (
	moduleInstallConfigKey  = "config.yaml"
	moduleInstallConfigPath = "/root/.threeport/config.yaml"
	moduleWorkloadPrefix    = "threeport-"

	// podNamespacePath is where Kubernetes mounts a pod's own namespace.
	podNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// readParentNamespace returns the namespace this controller runs in, which is the
// parent control plane's. The install job runs there under the controller's own
// account, so it needs no cluster role of its own. It is a variable so a test can
// replace the read of the pod namespace file.
var readParentNamespace = func() (string, error) {
	namespace, err := os.ReadFile(podNamespacePath)
	if err != nil {
		return "", fmt.Errorf("failed to read the controller namespace: %w", err)
	}
	trimmed := strings.TrimSpace(string(namespace))
	if trimmed == "" {
		return "", errors.New("controller namespace is empty")
	}

	return trimmed, nil
}

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
		// a core module has no image of its own
		if module.Core != nil && *module.Core {
			continue
		}

		// bring a stopped module back even when it has no recorded image
		kebab := moduleKebab(util.DerefString(module.Name))
		workloadNamespace := moduleWorkloadNamespace(&module)
		imageNamespace, imageTag := recordedModuleImages(&module)
		controlPlaneNamespace := util.DerefString(inst.Namespace)
		if _, err := refreshModuleTrust(
			kubeClient, workloadNamespace, controlPlaneNamespace, inst,
		); err != nil {
			return err
		}
		if err := retargetModuleDeployment(
			kubeClient, workloadNamespace, moduleAPIDeploymentName(kebab), controlPlaneNamespace,
		); err != nil {
			return err
		}
		if imageNamespace == "" || imageTag == "" {
			if err := resumeModuleByName(
				kubeClient, workloadNamespace, moduleAPIDeploymentName(kebab),
			); err != nil {
				return err
			}
		} else if err := retagModuleAPI(
			kubeClient, mapper, inst, cpi, &module, imageNamespace, imageTag, controlPlaneNamespace,
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
				kubeClient, mapper, &controller, kebab, imageNamespace, imageTag, controlPlaneNamespace,
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
	imageNamespace, imageTag, controlPlaneNamespace string,
) error {
	// read the API deployment
	kebab := moduleKebab(util.DerefString(module.Name))
	namespace := moduleWorkloadNamespace(module)
	name := moduleAPIDeploymentName(kebab)
	deployment, err := kube.GetResource(
		"apps", "v1", "Deployment", namespace, name, kubeClient, *mapper,
	)
	if k8serrors.IsNotFound(err) {
		_, installErr := EnsureModuleInstallJob(kubeClient, mapper, inst, cpi, module)
		return installErr
	}
	if err != nil {
		return fmt.Errorf("failed to get module API deployment %s/%s: %w", namespace, name, err)
	}
	if err := resumeStoppedDeployment(kubeClient, namespace, deployment); err != nil {
		return err
	}

	// leave a deployment whose containers are missing or already current
	apiImage := moduleRestAPIImage(imageNamespace, kebab, imageTag)
	migratorImage := moduleMigratorImage(imageNamespace, kebab, imageTag)
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
		controlPlaneNamespace,
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
	kebab, imageNamespace, imageTag, controlPlaneNamespace string,
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
	container := moduleControllerContainerName(name)
	image := moduleControllerImage(imageNamespace, name, imageTag)
	current, ok := namedContainerImage(deployment, "containers", container)
	if !ok || current == image {
		return nil
	}

	// restart onto the recorded image
	return restartModuleDeployment(
		kubeClient,
		namespace,
		name,
		controlPlaneNamespace,
		map[string]string{"containers": image},
		map[string]string{"containers": container},
		time.Now().UTC(),
	)
}

// ModuleInstallAfterRun returns the request the controller stores after a run.
// The controller keeps a stored value that differs from the request it ran.
func ModuleInstallAfterRun(stored string, started string) string {
	// keep a stored request that is not the one this run started
	if stored != started {
		return stored
	}
	return ""
}

// ModuleInstallRequest is the value stored for the parent controller to run.
func ModuleInstallRequest(module *api.ModuleApi) string {
	// keep the three fields in an order the controller can split
	return strings.Join([]string{
		util.DerefString(module.Name),
		util.DerefString(module.ImageNamespace),
		util.DerefString(module.ImageTag),
	}, " ")
}

// moduleFromInstallRequest reads the stored name, image namespace, and tag.
func moduleFromInstallRequest(raw string) (*api.ModuleApi, error) {
	// reject a request that is missing one of the three fields
	parts := strings.SplitN(raw, " ", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf("module install request %q is missing a field", raw)
	}
	return &api.ModuleApi{
		Name:           &parts[0],
		ImageNamespace: &parts[1],
		ImageTag:       &parts[2],
	}, nil
}

// InstallRequestedModule drives the install job for the stored request with the
// caller's kube client, which the controller already uses for the control plane.
// It reports whether the install finished, so the controller keeps the request
// stored and checks again until the job succeeds.
func InstallRequestedModule(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
) (bool, error) {
	// a row that is not asking for an install has nothing left to do
	raw := util.DerefString(inst.ModuleInstall)
	if raw == "" {
		return true, nil
	}
	module, err := moduleFromInstallRequest(raw)
	if err != nil {
		return false, err
	}
	return EnsureModuleInstallJob(kubeClient, mapper, inst, cpi, module)
}

// EnsureModuleInstallJob moves a module install toward its requested image and
// reports whether it finished. A missing job is created. A running job for the
// same image is left to finish, and one for another image is an error, so the
// request stays stored until that job ends. A succeeded job for the same image
// is removed and reported as finished. A failed job, or a finished job for
// another image, is replaced.
func EnsureModuleInstallJob(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
	module *api.ModuleApi,
) (bool, error) {
	// read the job for this module
	kebab := moduleKebab(util.DerefString(module.Name))
	childNamespace := controlPlaneNamespace(inst, cpi)
	namespace, err := readParentNamespace()
	if err != nil {
		return false, err
	}
	name := moduleInstallJobName(kebab, childNamespace)
	secretName := moduleInstallSecretName(kebab, childNamespace)
	wanted := modulePluginImage(
		util.DerefString(module.ImageNamespace),
		kebab,
		util.DerefString(module.ImageTag),
	)
	existing, err := kube.GetResource(
		"batch", "v1", "Job", namespace, name, kubeClient, *mapper,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, fmt.Errorf("failed to get module install job %s: %w", name, err)
	}

	// act on the job that exists
	attempts := int64(1)
	if err == nil {
		image, _ := namedContainerImage(existing, "containers", "install")
		switch {
		case !jobFinished(existing) && image != wanted:
			return false, fmt.Errorf("%w: job %s runs %s, not %s", ErrModuleInstallRunning, name, image, wanted)
		case !jobFinished(existing):
			return false, nil
		case jobSucceeded(existing) && image == wanted:
			// remove the job and the credentials it mounted
			if err := deleteModuleInstallJob(kubeClient, namespace, name); err != nil {
				return false, err
			}
			if err := deleteModuleInstallConfig(kubeClient, namespace, secretName); err != nil {
				return false, err
			}
			return true, nil
		}

		// count another attempt for a job that failed on the same image, and give up at the limit
		if jobFailed(existing) && image == wanted && !jobAbandoned(existing) {
			previous := jobAttempts(existing)
			if previous >= moduleInstallMaxAttempts {
				// keep the job for its logs and remove the credentials it mounted
				if err := abandonModuleInstallJob(kubeClient, namespace, existing); err != nil {
					return false, err
				}
				if err := deleteModuleInstallConfig(kubeClient, namespace, secretName); err != nil {
					return false, err
				}
				return false, fmt.Errorf("%w: job %s failed %d times", ErrModuleInstallFailed, name, previous)
			}
			attempts = previous + 1
		}

		// replace a failed job, or one for another image, so its pods are removed
		if err := deleteModuleInstallJob(kubeClient, namespace, name); err != nil {
			return false, err
		}
	}

	// write the config the install reads, then create the job
	if err := ensureModuleInstallConfig(
		kubeClient, mapper, inst, cpi, namespace, secretName,
	); err != nil {
		return false, err
	}
	_, err = kube.CreateResource(
		moduleInstallJob(namespace, childNamespace, kebab, module, attempts), kubeClient, *mapper,
	)
	if err != nil {
		return false, fmt.Errorf("failed to create module install job %s: %w", name, err)
	}

	return false, nil
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
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(
		schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		meta.RESTScopeRoot,
	)
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

	return moduleWorkloadPrefix + moduleKebab(util.DerefString(module.Name))
}

// moduleInstallJobName returns the install job name for one module on one child.
// Jobs from every child share the parent namespace, so the name carries the child.
func moduleInstallJobName(kebab, childNamespace string) string {
	return "module-install-" + kebab + "-" + childNamespace
}

// moduleInstallSecretName returns the config secret name for one module on one child.
func moduleInstallSecretName(kebab, childNamespace string) string {
	return moduleInstallJobName(kebab, childNamespace) + "-config"
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

// jobFinished reports a job that succeeded or failed.
func jobFinished(job *unstructured.Unstructured) bool {
	succeeded, _, _ := unstructured.NestedInt64(job.Object, "status", "succeeded")
	if succeeded > 0 {
		return true
	}
	return jobFailed(job)
}

// jobAttempts returns which attempt a job is, one for a job with no record.
func jobAttempts(job *unstructured.Unstructured) int64 {
	attempts, err := strconv.ParseInt(job.GetAnnotations()[moduleInstallAttemptsAnnotation], 10, 64)
	if err != nil || attempts < 1 {
		return 1
	}

	return attempts
}

// jobAbandoned reports a failed job the controller already gave up on.
func jobAbandoned(job *unstructured.Unstructured) bool {
	return job.GetAnnotations()[moduleInstallAbandonedAnnotation] == "true"
}

// abandonModuleInstallJob marks a failed job so a later request starts over. The
// job stays, so its pod logs remain readable.
func abandonModuleInstallJob(kubeClient dynamic.Interface, namespace string, job *unstructured.Unstructured) error {
	annotations := job.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[moduleInstallAbandonedAnnotation] = "true"
	job.SetAnnotations(annotations)
	_, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Update(context.TODO(), job, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to mark module install job %s abandoned: %w", job.GetName(), err)
	}

	return nil
}

// jobSucceeded reports whether a job finished with a succeeded pod.
func jobSucceeded(job *unstructured.Unstructured) bool {
	succeeded, _, _ := unstructured.NestedInt64(job.Object, "status", "succeeded")
	return succeeded > 0
}

// jobFailed reports whether a job has failed for good. Kubernetes sets the Failed
// condition once the job has used its retries. A pod that failed while a retry
// is still to start leaves the job running, so a failed count alone does not
// count.
func jobFailed(job *unstructured.Unstructured) bool {
	// a succeeded job stays
	succeeded, _, _ := unstructured.NestedInt64(job.Object, "status", "succeeded")
	if succeeded > 0 {
		return false
	}

	// a Failed condition means no retry is left
	conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if condition["type"] == "Failed" && condition["status"] == "True" {
			return true
		}
	}

	return false
}

// deleteModuleInstallConfig removes the install config secret, which holds the
// child's credentials. A secret already gone is not an error.
func deleteModuleInstallConfig(kubeClient dynamic.Interface, namespace, name string) error {
	err := kubeClient.Resource(schema.GroupVersionResource{
		Version: "v1", Resource: "secrets",
	}).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete module install config %s: %w", name, err)
	}

	return nil
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

// ensureModuleInstallConfig writes the config secret the install job mounts, in
// the namespace the job runs in. The config holds the child's credentials, so
// the job removes it when the install ends.
func ensureModuleInstallConfig(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	inst *api.ControlPlaneInstance,
	cpi *ControlPlaneInstaller,
	jobNamespace, secretName string,
) error {
	// build the config the CLI reads from its home directory
	configYAML, err := moduleInstallConfig(kubeClient, mapper, inst, cpi)
	if err != nil {
		return err
	}
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      secretName,
			"namespace": jobNamespace,
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
) ([]byte, error) {
	// read the decoded encryption key the CLI stores in its config from the child namespace
	key, err := cpi.EncryptionKeyFromNamespace(kubeClient, mapper, controlPlaneNamespace(inst, cpi))
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
func moduleInstallJob(
	namespace, childNamespace, kebab string,
	module *api.ModuleApi,
	attempts int64,
) *unstructured.Unstructured {
	// label the pod the database admits, mount the config, and run the install
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      moduleInstallJobName(kebab, childNamespace),
			"namespace": namespace,
			"annotations": map[string]interface{}{
				moduleInstallAttemptsAnnotation: strconv.FormatInt(attempts, 10),
			},
		},
		"spec": map[string]interface{}{
			"backoffLimit":          int64(1),
			"activeDeadlineSeconds": moduleInstallDeadlineSeconds,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{LabelMaintenance: "true"},
				},
				"spec": map[string]interface{}{
					"restartPolicy":      "Never",
					"serviceAccountName": ThreeportControlPlaneControllerName,
					"containers": []interface{}{
						map[string]interface{}{
							"name":    "install",
							"command": []interface{}{"/" + kebab},
							"image": modulePluginImage(
								util.DerefString(module.ImageNamespace),
								kebab,
								util.DerefString(module.ImageTag),
							),
							"args": []interface{}{
								"install",
								"--control-plane-image-namespace",
								util.DerefString(module.ImageNamespace),
								"--control-plane-image-tag",
								util.DerefString(module.ImageTag),
							},
							"securityContext": map[string]interface{}{"runAsUser": int64(0)},
							"env": []interface{}{
								map[string]interface{}{"name": "HOME", "value": "/root"},
								map[string]interface{}{"name": ModuleInstallJobEnv, "value": "true"},
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
								"secretName": moduleInstallSecretName(kebab, childNamespace),
							},
						},
					},
				},
			},
		},
	}}
}
