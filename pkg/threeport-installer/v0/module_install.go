package v0

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	api "github.com/threeport/threeport/pkg/api/v0"
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

// ErrModuleNamespaceExists is returned when a module install finds its namespace
// already present, so a second control plane does not install into it.
var ErrModuleNamespaceExists = errors.New("module namespace already exists")

// ModuleNamespaceFlag is the install flag that sets the module's namespace.
const ModuleNamespaceFlag = "module-namespace"

// ModuleNamespaceOwnerLabel records the control plane namespace that installed the module.
const ModuleNamespaceOwnerLabel = "threeport.io/module-owner"

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

	// moduleInstallLabel marks the job and the config secret so child deletion can
	// remove them without knowing the module name.
	moduleInstallLabel = "threeport.io/module-install"

	// moduleInstallNamePrefix is the start of every install job name.
	moduleInstallNamePrefix = "module-install-"

	// moduleInstallSecretSuffix is appended to the job name for its config secret.
	moduleInstallSecretSuffix = "-config"

	// moduleInstallNameLimit is the Kubernetes limit for a Job or Secret name.
	// The secret name is the job name plus moduleInstallSecretSuffix, so the job
	// name stops short of the limit by that many characters.
	moduleInstallNameLimit = 63

	// threeportWorkloadsClusterRoleName is the cluster role that grants workload access.
	threeportWorkloadsClusterRoleName = "threeport-controllers-threeportworkloads"

	// clusterAdminClusterRoleName is the cluster role that grants cluster admin.
	clusterAdminClusterRoleName = "cluster-admin"
)

// ModuleInstallJobEnv names the environment variable the install job sets so the
// module plugin installs on the control plane it is connected to.
const ModuleInstallJobEnv = "THREEPORT_MODULE_INSTALL_JOB"

const (
	moduleInstallConfigKey      = "config.yaml"
	moduleInstallConfigPath     = "/root/.threeport/config.yaml"
	moduleWorkloadPrefix        = "threeport-"
	moduleInstallServiceAccount = "module-install"
)

// moduleInstallBindingName returns the cluster role binding name for one control plane.
func moduleInstallBindingName(namespace string) string {
	return moduleInstallServiceAccount + "-" + namespace
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
// A namespace that differs from the default is stored after the image tag.
func ModuleInstallRequest(module *api.ModuleApi, moduleNamespace ...string) string {
	// keep the name, image namespace, and tag, then a module namespace when one is set
	parts := []string{
		util.DerefString(module.Name),
		util.DerefString(module.ImageNamespace),
		util.DerefString(module.ImageTag),
	}
	namespace := ""
	if len(moduleNamespace) > 0 {
		namespace = moduleNamespace[0]
	}
	if namespace = installNamespace(module, namespace); namespace != "" {
		parts = append(parts, namespace)
	}
	return strings.Join(parts, " ")
}

// moduleFromInstallRequest reads the stored name, image namespace, tag, and
// module namespace. The module namespace is empty when the request has none.
func moduleFromInstallRequest(raw string) (*api.ModuleApi, string, error) {
	// reject a request that is missing one of the three fields
	parts := strings.Split(raw, " ")
	if len(parts) < 3 || len(parts) > 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, "", fmt.Errorf("module install request %q is missing a field", raw)
	}

	// read a module namespace when the request stored one
	moduleNamespace := ""
	if len(parts) == 4 {
		if parts[3] == "" {
			return nil, "", fmt.Errorf("module install request %q is missing a field", raw)
		}
		moduleNamespace = parts[3]
	}
	return &api.ModuleApi{
		Name:           &parts[0],
		ImageNamespace: &parts[1],
		ImageTag:       &parts[2],
	}, moduleNamespace, nil
}

// moduleInstallTargetNamespace returns the namespace an install writes into.
func moduleInstallTargetNamespace(module *api.ModuleApi, moduleNamespace string) string {
	// a non-empty moduleNamespace replaces threeport- plus the module name
	if moduleNamespace != "" {
		return moduleNamespace
	}
	return moduleWorkloadNamespace(module)
}

// RejectExistingModuleNamespace returns an error when another control plane owns the namespace.
// A namespace this control plane labeled, or an unlabeled one whose API deployment names this database, is left for the install to update.
func RejectExistingModuleNamespace(
	kubeClient dynamic.Interface,
	namespace, owner, apiDeployment string,
) error {
	// read the namespace the install would create
	existing, err := kubeClient.Resource(namespaceGVR).Get(context.Background(), namespace, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
	}

	// allow a namespace this control plane already owns
	if owner != "" && existing.GetLabels()[ModuleNamespaceOwnerLabel] == owner {
		return nil
	}

	// adopt an unlabeled namespace whose API deployment names this database
	if existing.GetLabels()[ModuleNamespaceOwnerLabel] == "" {
		holds, holdsErr := namespaceHoldsAPIDeployment(kubeClient, namespace, apiDeployment, owner)
		if holdsErr != nil {
			return holdsErr
		}
		if holds {
			return nil
		}
	}

	// report a namespace owned by another control plane
	return fmt.Errorf("%w: %s", ErrModuleNamespaceExists, namespace)
}

// namespaceHoldsAPIDeployment reports whether the named deployment records this control plane's database.
func namespaceHoldsAPIDeployment(
	kubeClient dynamic.Interface, namespace, name, owner string,
) (bool, error) {
	// an empty name or owner has nothing to match
	if name == "" || owner == "" {
		return false, nil
	}

	// read the API deployment
	deployment, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(),
		name,
		metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	// match the database service this control plane's modules record
	return deploymentCommandContains(deployment, controlPlaneDatabaseHost(owner)), nil
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
	module, moduleNamespace, err := moduleFromInstallRequest(raw)
	if err != nil {
		return false, err
	}
	return EnsureModuleInstallJob(kubeClient, mapper, inst, cpi, module, moduleNamespace)
}

// ensureControlPlaneControllerAccount creates the account a module install job runs as.
// It also creates that account's cluster role bindings, and it creates no controller deployment.
func (cpi *ControlPlaneInstaller) ensureControlPlaneControllerAccount(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	namespace string,
) error {
	account := ThreeportControlPlaneControllerName

	// create the role the workloads binding grants
	role := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]interface{}{
			"name": threeportWorkloadsClusterRoleName,
		},
		"rules": []interface{}{
			map[string]interface{}{
				"apiGroups": []interface{}{"control-plane.threeport.io"},
				"resources": []interface{}{"threeportworkloads"},
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
	}}
	if _, err := kube.CreateOrUpdateResource(role, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create threeport controllers threeportworkloads cluster role: %w", err)
	}

	// create the account in the control plane namespace
	metadata := map[string]interface{}{
		"name":      account,
		"namespace": namespace,
	}
	if cpi.Opts.GcpServiceAccountEmail != "" {
		metadata["annotations"] = map[string]interface{}{
			"iam.gke.io/gcp-service-account": cpi.Opts.GcpServiceAccountEmail,
		}
	}
	serviceAccount := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata":   metadata,
	}}
	if _, err := kube.CreateOrUpdateResource(serviceAccount, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create controller service account: %w", err)
	}

	// grant the account the workloads role and cluster admin
	subjects := controllerAccountSubjects(cpi, namespace, account)
	if err := ensureControllerClusterRoleBinding(
		kubeClient,
		mapper,
		fmt.Sprintf("%s-%s-threeportworkloads", namespace, account),
		threeportWorkloadsClusterRoleName,
		subjects,
	); err != nil {
		return err
	}

	return ensureControllerClusterRoleBinding(
		kubeClient,
		mapper,
		fmt.Sprintf("%s-%s-%s", namespace, account, clusterAdminClusterRoleName),
		clusterAdminClusterRoleName,
		subjects,
	)
}

// controllerAccountSubjects returns the subjects for the control plane controller account.
// On GKE the apiserver authenticates the pod as a user, so the subjects include that user.
func controllerAccountSubjects(cpi *ControlPlaneInstaller, namespace, account string) []interface{} {
	// start with the account in the control plane namespace
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      account,
			"namespace": namespace,
		},
	}

	// add the user a GKE apiserver authenticates the pod as
	if cpi.Opts.InfraProvider == api.KubernetesRuntimeInfraProviderGKE && cpi.Opts.GcpProjectId != "" {
		subjects = append(subjects, map[string]interface{}{
			"kind":     "User",
			"name":     fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", cpi.Opts.GcpProjectId, namespace, account),
			"apiGroup": "rbac.authorization.k8s.io",
		})
	}

	return subjects
}

// ensureControllerClusterRoleBinding creates or updates one cluster role binding.
func ensureControllerClusterRoleBinding(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	name, role string,
	subjects []interface{},
) error {
	// write the binding
	binding := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata": map[string]interface{}{
			"name": name,
		},
		"roleRef": map[string]interface{}{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     role,
		},
		"subjects": subjects,
	}}
	if _, err := kube.CreateOrUpdateResource(binding, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create cluster role binding %s: %w", name, err)
	}

	return nil
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
	moduleNamespace string,
) (bool, error) {
	// read the job for this module
	kebab := moduleKebab(util.DerefString(module.Name))
	namespace := controlPlaneNamespace(inst, cpi)
	name := moduleInstallJobName(kebab, namespace)
	secretName := moduleInstallSecretName(kebab, namespace)
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
	replaceAbandoned := false
	if err == nil {
		image, _ := namedContainerImage(existing, "containers", "install")
		switch {
		case !jobFinished(existing) && image != wanted:
			return false, fmt.Errorf("%w: job %s runs %s, not %s", ErrModuleInstallRunning, name, image, wanted)
		case !jobFinished(existing):
			return false, nil
		case jobSucceeded(existing) && image == wanted:
			// remove the credentials first, so a failed delete leaves the finished job
			if err := deleteModuleInstallConfig(kubeClient, namespace, secretName); err != nil {
				return false, err
			}
			if err := deleteModuleInstallJob(kubeClient, namespace, name); err != nil {
				return false, err
			}
			return true, nil
		case jobFailed(existing) && image == wanted && jobAbandoned(existing):
			// a secret that is still there is the failed cleanup, so the job stays
			_, secretErr := kubeClient.Resource(schema.GroupVersionResource{
				Version: "v1", Resource: "secrets",
			}).Namespace(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if secretErr == nil {
				if err := deleteModuleInstallConfig(kubeClient, namespace, secretName); err != nil {
					return false, err
				}
				return false, fmt.Errorf("%w: job %s failed %d times", ErrModuleInstallFailed, name, jobAttempts(existing))
			}
			if !k8serrors.IsNotFound(secretErr) {
				return false, fmt.Errorf("failed to get module install config %s: %w", secretName, secretErr)
			}
			replaceAbandoned = true
			attempts = 1
		}

		// count another attempt for a job that failed on the same image, and give up at the limit.
		// an abandoned job whose secret is already gone is a new request, counted above
		if !replaceAbandoned && jobFailed(existing) && image == wanted {
			previous := jobAttempts(existing)
			if previous >= moduleInstallMaxAttempts {
				// mark the job before deleting the secret, so a failed delete does not start over
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

	// refuse a namespace another control plane owns
	if k8serrors.IsNotFound(err) {
		if err := RejectExistingModuleNamespace(
			kubeClient,
			moduleInstallTargetNamespace(module, moduleNamespace),
			namespace,
			moduleAPIDeploymentName(kebab),
		); err != nil {
			return false, err
		}
	}

	// create the account the job runs as, then write its config and the job
	if err := cpi.ensureControlPlaneControllerAccount(kubeClient, mapper, namespace); err != nil {
		return false, err
	}
	if err := ensureModuleInstallConfig(
		kubeClient, mapper, inst, cpi, namespace, secretName,
	); err != nil {
		return false, err
	}
	_, err = kube.CreateResource(
		moduleInstallJob(namespace, kebab, module, moduleNamespace, attempts), kubeClient, *mapper,
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
		schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
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

// installNamespace returns the namespace to pass to an install.
// The default namespace is left empty so an older plugin still runs.
func installNamespace(module *api.ModuleApi, moduleNamespace string) string {
	// use the recorded namespace when the caller did not pass one
	if moduleNamespace == "" {
		moduleNamespace = recordedModuleNamespace(module)
	}

	// drop the default namespace so the install flag stays off
	if moduleNamespace == defaultModuleNamespace(module) {
		return ""
	}
	return moduleNamespace
}

// defaultModuleNamespace returns the namespace an install uses when none is set.
func defaultModuleNamespace(module *api.ModuleApi) string {
	return moduleWorkloadPrefix + moduleKebab(util.DerefString(module.Name))
}

// recordedModuleNamespace returns the namespace the module endpoint records.
func recordedModuleNamespace(module *api.ModuleApi) string {
	// read the namespace the endpoint records
	if server, ok := moduleApiServerFromEndpoint(module.Endpoint); ok {
		return server.Namespace
	}
	return ""
}

// moduleWorkloadNamespace returns the namespace the module workloads run in.
func moduleWorkloadNamespace(module *api.ModuleApi) string {
	// prefer the namespace the module endpoint records
	if namespace := recordedModuleNamespace(module); namespace != "" {
		return namespace
	}

	return defaultModuleNamespace(module)
}

// moduleInstallJobName returns the install job name for one module on one control plane.
// The name keeps a short hash of both inputs and stays short enough that the
// config secret, which appends moduleInstallSecretSuffix, still fits in 63 characters.
func moduleInstallJobName(kebab, namespace string) string {
	sum := sha256.Sum256([]byte(kebab + "\n" + namespace))
	hash := hex.EncodeToString(sum[:4])
	room := moduleInstallNameLimit - len(moduleInstallSecretSuffix) - len(moduleInstallNamePrefix) - 1 - len(hash)
	readable := kebab
	if len(readable) > room {
		readable = strings.TrimRight(readable[:room], "-")
	}
	if readable == "" {
		return moduleInstallNamePrefix + hash
	}

	return moduleInstallNamePrefix + readable + "-" + hash
}

// moduleInstallSecretName returns the config secret name for one module on one control plane.
func moduleInstallSecretName(kebab, namespace string) string {
	return moduleInstallJobName(kebab, namespace) + moduleInstallSecretSuffix
}

// modulePluginCommand returns the path of the plugin binary in the image.
// The image build uses '_' wherever the module name uses '-'.
func modulePluginCommand(kebab string) string {
	// match the binary name the image copies in
	return "/" + strings.ReplaceAll(kebab, "-", "_")
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

// DeleteModuleInstallObjects removes the install jobs and config secrets in one namespace.
// Child deletion calls it before the namespace delete, so a running install stops.
func DeleteModuleInstallObjects(kubeClient dynamic.Interface, namespace string) error {
	// delete each labeled job, which removes its pods
	selector := moduleInstallLabel + "=true"
	jobs, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("failed to list module install jobs: %w", err)
	}
	for _, job := range jobs.Items {
		if err := deleteModuleInstallJob(kubeClient, namespace, job.GetName()); err != nil {
			return err
		}
	}

	// delete each labeled config secret
	secrets, err := kubeClient.Resource(schema.GroupVersionResource{
		Version: "v1", Resource: "secrets",
	}).Namespace(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("failed to list module install secrets: %w", err)
	}
	for _, secret := range secrets.Items {
		if err := deleteModuleInstallConfig(kubeClient, namespace, secret.GetName()); err != nil {
			return err
		}
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
			"labels": map[string]interface{}{
				moduleInstallLabel: "true",
			},
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
	namespace, kebab string,
	module *api.ModuleApi,
	moduleNamespace string,
	attempts int64,
) *unstructured.Unstructured {
	// pass a module namespace only when the request named one
	args := []interface{}{
		"install",
		"--control-plane-image-namespace",
		util.DerefString(module.ImageNamespace),
		"--control-plane-image-tag",
		util.DerefString(module.ImageTag),
	}
	if moduleNamespace != "" {
		args = append(args, "--"+ModuleNamespaceFlag, moduleNamespace)
	}

	// label the pod the database admits, mount the config, and run the install
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      moduleInstallJobName(kebab, namespace),
			"namespace": namespace,
			"labels": map[string]interface{}{
				moduleInstallLabel: "true",
			},
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
							"command": []interface{}{modulePluginCommand(kebab)},
							"image": modulePluginImage(
								util.DerefString(module.ImageNamespace),
								kebab,
								util.DerefString(module.ImageTag),
							),
							"args":            args,
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
								"secretName": moduleInstallSecretName(kebab, namespace),
							},
						},
					},
				},
			},
		},
	}}
}
