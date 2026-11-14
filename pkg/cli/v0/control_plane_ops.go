package v0

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/version"
	api "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ErrClusterClientUnavailable is returned when a command needs the target
// cluster and no wake column was added for the controller to do the work.
var ErrClusterClientUnavailable = errors.New(
	"cluster client is unavailable and no wake column was added",
)

// ControlPlaneTarget is the control plane a command operates on.
// The API client reaches that control plane, and the kube client is its cluster.
type ControlPlaneTarget struct {
	// The control plane instance row
	Instance *api.ControlPlaneInstance

	// The HTTP client for the target API
	APIClient *http.Client

	// The target API endpoint
	APIEndpoint string

	// The dynamic client for the target cluster
	Kube dynamic.Interface

	// The REST mapper for the target cluster
	Mapper *meta.RESTMapper

	// The kubernetes runtime name
	RuntimeName *string
}

// ResolveControlPlane loads a control plane from the current API.
// An empty name selects the control plane serving the request.
func ResolveControlPlane(
	currentClient *http.Client,
	currentEndpoint string,
	name string,
	allowChildSelf bool,
) (*ControlPlaneTarget, error) {
	// load the named row, or the row serving this request
	var (
		inst *api.ControlPlaneInstance
		err  error
	)
	if name == "" {
		inst, err = client.GetSelfControlPlaneInstance(currentClient, currentEndpoint)
	} else {
		inst, err = client.GetControlPlaneInstanceByName(currentClient, currentEndpoint, name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane instance: %w", err)
	}

	// reject a child acting on itself when the caller does not allow it
	if !allowChildSelf {
		if err := api.ChildControlPlaneRequest(inst); err != nil {
			return nil, err
		}
	}

	// talk to the current API for itself and to the stored endpoint for a child
	target := &ControlPlaneTarget{Instance: inst}
	if name == "" || inst.IsSelf == nil || *inst.IsSelf {
		target.APIClient = currentClient
		target.APIEndpoint = currentEndpoint
		return target, nil
	}
	httpClient, err := installer.HTTPClientForInstance(inst)
	if err != nil {
		return nil, fmt.Errorf("failed to build control plane api client: %w", err)
	}
	endpoint := util.DerefString(inst.ApiServerEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("failed to resolve control plane api: endpoint is empty")
	}
	target.APIClient = httpClient
	target.APIEndpoint = endpoint
	return target, nil
}

// clusterClient loads the kube client for the target from the current API.
func (target *ControlPlaneTarget) clusterClient(
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
) error {
	// the runtime record lives on the API that stored the control plane
	if target.Instance.KubernetesRuntimeInstanceID == nil {
		return fmt.Errorf("%w: kubernetes runtime instance is unset", ErrClusterClientUnavailable)
	}
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		currentClient,
		currentEndpoint,
		*target.Instance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrClusterClientUnavailable, err)
	}

	// decrypt the stored runtime credentials with the current API key
	kubeClient, mapper, err := kube.GetClient(
		runtime,
		false,
		currentClient,
		currentEndpoint,
		encryptionKey,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrClusterClientUnavailable, err)
	}
	target.Kube = kubeClient
	target.Mapper = mapper
	target.RuntimeName = runtime.Name
	return nil
}

// InstallModule creates the module install job on the target cluster.
// It does not create the module API row. The plugin install does that.
func InstallModule(
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	controlPlaneName string,
	moduleName string,
	imageNamespace string,
	imageTag string,
) error {
	// resolve the control plane and its cluster before creating anything
	target, err := ResolveControlPlane(currentClient, currentEndpoint, controlPlaneName, false)
	if err != nil {
		return err
	}
	if err := target.clusterClient(currentClient, currentEndpoint, encryptionKey); err != nil {
		return err
	}

	// run the install job from the image coordinates, without writing a module row
	module := &api.ModuleApi{
		Name:           &moduleName,
		ImageNamespace: &imageNamespace,
		ImageTag:       &imageTag,
	}
	cpi := installer.NewInstaller(
		installer.Namespace(util.DerefString(target.Instance.Namespace)),
	)
	return installer.EnsureModuleInstallJob(
		target.Kube,
		target.Mapper,
		target.Instance,
		cpi,
		module,
	)
}

// UpgradeModule records a module image.
// wake is false when a blue-green install still has to set the source namespace.
func UpgradeModule(
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	controlPlaneName string,
	moduleName string,
	imageNamespace string,
	imageTag string,
	wake bool,
) error {
	// reject a child connection before writing
	self, err := client.GetSelfControlPlaneInstance(currentClient, currentEndpoint)
	if err != nil {
		return fmt.Errorf("failed to get control plane instance: %w", err)
	}
	if err := api.ChildControlPlaneRequest(self); err != nil {
		return err
	}

	// load the named child, or the connected control plane when the name is empty
	target, err := ResolveControlPlane(currentClient, currentEndpoint, controlPlaneName, false)
	if err != nil {
		return err
	}

	// record the tag and, when set, the namespace
	module, err := client.GetModuleApiByName(target.APIClient, target.APIEndpoint, moduleName)
	if err != nil {
		return fmt.Errorf("failed to get module api: %w", err)
	}
	module.ImageTag = &imageTag
	if imageNamespace != "" {
		module.ImageNamespace = &imageNamespace
	}
	if _, err := client.UpdateModuleApi(target.APIClient, target.APIEndpoint, module); err != nil {
		return fmt.Errorf("failed to update module api: %w", err)
	}

	// wake the row the parent controller reconciles, including a named child
	if !wake {
		return nil
	}
	if _, err := client.UpdateControlPlaneInstance(
		currentClient,
		currentEndpoint,
		target.Instance,
	); err != nil {
		return fmt.Errorf("failed to update control plane instance: %w", err)
	}
	return nil
}

// UninstallModule deletes the module workload namespace and its API row.
func UninstallModule(
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	controlPlaneName string,
	moduleName string,
) error {
	// fail before deleting anything when the cluster cannot be reached
	target, err := ResolveControlPlane(currentClient, currentEndpoint, controlPlaneName, false)
	if err != nil {
		return err
	}
	if err := target.clusterClient(currentClient, currentEndpoint, encryptionKey); err != nil {
		return err
	}
	module, err := client.GetModuleApiByName(target.APIClient, target.APIEndpoint, moduleName)
	if err != nil {
		return fmt.Errorf("failed to get module api: %w", err)
	}

	return installer.UninstallModule(
		target.Kube,
		target.Mapper,
		target.APIClient,
		target.APIEndpoint,
		module,
	)
}

// BackupNamedControlPlane backs up the named control plane. A child may back itself up.
// An empty namespace uses the instance namespace.
// The writers are brought back up or left stopped, as writers says.
func BackupNamedControlPlane(
	local *ControlPlane,
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	controlPlaneName string,
	namespace string,
	bucket string,
	writers BackupWriters,
) error {
	// a child backing itself up is allowed
	target, err := ResolveControlPlane(currentClient, currentEndpoint, controlPlaneName, true)
	if err != nil {
		return err
	}
	if err := target.clusterClient(currentClient, currentEndpoint, encryptionKey); err != nil {
		return err
	}
	if namespace == "" {
		namespace = util.DerefString(target.Instance.Namespace)
	}

	// use the local install's bucket unless the caller named one
	resolved, err := RestoreBackupBucket(local, bucket)
	if err != nil {
		return fmt.Errorf("failed to resolve backup bucket: %w", err)
	}
	// back up, then resume the writers when the caller asked
	_, _, err = BackupControlPlaneWithClient(
		namespace,
		resolved,
		target.APIClient,
		target.APIEndpoint,
		target.Kube,
		writers,
	)
	return err
}

// RestoreNamedControlPlane restores the named control plane.
// A child may not restore itself. A running restore retargets installed modules.
func RestoreNamedControlPlane(
	local *ControlPlane,
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	controlPlaneName string,
	sourceBucket string,
	backup string,
	namespace string,
	fromNamespace string,
	stopControllers bool,
) error {
	// a child restoring itself is rejected
	target, err := ResolveControlPlane(currentClient, currentEndpoint, controlPlaneName, false)
	if err != nil {
		return err
	}
	if err := target.clusterClient(currentClient, currentEndpoint, encryptionKey); err != nil {
		return err
	}
	if namespace == "" {
		namespace = util.DerefString(target.Instance.Namespace)
	}

	// restore the database, then retarget modules once the API is running
	resolved, err := RestoreBackupBucket(local, sourceBucket)
	if err != nil {
		return fmt.Errorf("failed to resolve backup bucket: %w", err)
	}
	if err := RestoreControlPlaneDatabaseWithClient(
		local,
		resolved,
		backup,
		namespace,
		fromNamespace,
		stopControllers,
		target.Kube,
	); err != nil {
		return err
	}
	if stopControllers {
		return nil
	}

	// retarget installed modules on the restored control plane
	cpi := installer.NewInstaller(installer.Namespace(namespace))
	return cpi.RedeployInstalledModules(
		target.Kube,
		target.Mapper,
		target.APIClient,
		target.APIEndpoint,
	)
}

// RedeployRestoredModules retargets installed modules after a restore of the current control plane.
func RedeployRestoredModules(
	controlPlane *ControlPlane,
	namespace string,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// the provider path supplies the cluster client for the current control plane
	kubeClient, err := controlPlaneKubeClient(controlPlane)
	if err != nil {
		return fmt.Errorf("failed to get control plane kube client: %w", err)
	}
	mapper := installer.WorkloadRESTMapper()
	var opts []installer.InstallerOption
	if namespace != "" {
		opts = append(opts, installer.Namespace(namespace))
	}
	cpi := installer.NewInstaller(opts...)
	return cpi.RedeployInstalledModules(kubeClient, &mapper, apiClient, apiEndpoint)
}

// UpgradeBlueGreen stops the source writers, backs up once, and installs a new control plane from that backup.
// A named source is a child, and the control plane controller installs that one.
func UpgradeBlueGreen(
	threeportConfig *ThreeportConfig,
	local *ControlPlane,
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	sourceName string,
	newName string,
	childTag string,
	imageNamespace string,
	imageTag string,
) error {
	rejectChild, updateConfig := blueGreenOptions(true)
	return runBlueGreen(
		threeportConfig,
		local,
		currentClient,
		currentEndpoint,
		encryptionKey,
		sourceName,
		newName,
		childTag,
		imageNamespace,
		imageTag,
		rejectChild,
		updateConfig,
	)
}

// InstallBlueGreen runs the same restore tptctl runs for a control plane that is not a child.
func InstallBlueGreen(
	local *ControlPlane,
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	source *ControlPlaneTarget,
	newName string,
) error {
	if source == nil || source.Instance == nil {
		return fmt.Errorf("control plane is unset")
	}
	rejectChild, updateConfig := blueGreenOptions(false)
	return runBlueGreen(
		nil,
		local,
		currentClient,
		currentEndpoint,
		encryptionKey,
		util.DerefString(source.Instance.Name),
		newName,
		"",
		"",
		"",
		rejectChild,
		updateConfig,
	)
}

// blueGreenOptions reports whether this caller refuses a child connection and rewrites the tptctl config.
// tptctl does both. The controller does neither.
func blueGreenOptions(tptctl bool) (rejectChild bool, updateConfig bool) {
	// one pair of flags for both callers
	return tptctl, tptctl
}

// runBlueGreen backs up a control plane and installs it under a new name.
func runBlueGreen(
	threeportConfig *ThreeportConfig,
	local *ControlPlane,
	currentClient *http.Client,
	currentEndpoint string,
	encryptionKey string,
	sourceName string,
	newName string,
	childTag string,
	imageNamespace string,
	imageTag string,
	rejectChild bool,
	updateConfig bool,
) error {
	// reject a child connection before anything is stopped
	self, err := client.GetSelfControlPlaneInstance(currentClient, currentEndpoint)
	if err != nil {
		return fmt.Errorf("failed to get control plane instance: %w", err)
	}
	if err := api.ChildControlPlaneRequest(self); err != nil {
		return fmt.Errorf("control plane %s: %w", util.DerefString(self.Name), err)
	}
	if rejectChild && sourceName != "" && sourceName != util.DerefString(self.Name) {
		return delegateBlueGreen(local, currentClient, currentEndpoint, sourceName, newName, childTag, imageNamespace, imageTag)
	}

	// the connected control plane is the source
	source, err := ResolveControlPlane(currentClient, currentEndpoint, sourceName, false)
	if err != nil {
		return err
	}
	if sourceName == "" {
		sourceName = util.DerefString(source.Instance.Name)
	}
	newNamespace := newName
	if rejectChild && source.Instance.ParentControlPlaneInstanceID != nil {
		return delegateBlueGreen(local, currentClient, currentEndpoint, sourceName, newName, childTag, imageNamespace, imageTag)
	}
	namespace := util.DerefString(source.Instance.FromNamespace)
	if namespace == "" {
		namespace = util.DerefString(source.Instance.Namespace)
	}
	if namespace == "" {
		return fmt.Errorf("control plane %s has no namespace", sourceName)
	}

	// connect while the API still answers
	if err := source.clusterClient(currentClient, currentEndpoint, encryptionKey); err != nil {
		return err
	}

	// back up the namespace serving this API, because a restored database keeps the old namespace
	serving, err := namespaceServingAPI(source.Kube, source.APIEndpoint)
	if err != nil {
		return err
	}
	if serving != "" {
		namespace = serving
	}

	// refuse a restore into the namespace whose writers this command stops
	if newNamespace == "" || newNamespace == namespace {
		return fmt.Errorf("new namespace must differ from source namespace %s", namespace)
	}

	// load the provider and auth while the API still answers
	if source.Instance.KubernetesRuntimeInstanceID == nil || source.Instance.ControlPlaneDefinitionID == nil {
		return fmt.Errorf("control plane %s has no runtime or definition", sourceName)
	}
	runtimeInst, err := client.GetKubernetesRuntimeInstanceByID(
		source.APIClient,
		source.APIEndpoint,
		*source.Instance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}
	if runtimeInst.KubernetesRuntimeDefinitionID == nil {
		return fmt.Errorf("kubernetes runtime %s has no definition", util.DerefString(runtimeInst.Name))
	}
	runtimeDef, err := client.GetKubernetesRuntimeDefinitionByID(
		source.APIClient,
		source.APIEndpoint,
		*runtimeInst.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
	}
	providerName := util.DerefString(runtimeDef.InfraProvider)
	definition, err := client.GetControlPlaneDefinitionByID(
		source.APIClient,
		source.APIEndpoint,
		*source.Instance.ControlPlaneDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get control plane definition: %w", err)
	}

	// a GKE restore needs the project before the API is stopped
	projectID := ""
	if providerName == api.KubernetesRuntimeInfraProviderGKE {
		gkeInst, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
			source.APIClient,
			source.APIEndpoint,
			*source.Instance.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get gke runtime: %w", err)
		}
		if gkeInst.GcpProviderID == nil {
			return fmt.Errorf("gke runtime %s has no provider", util.DerefString(runtimeInst.Name))
		}
		gcpProvider, err := client.GetGcpProviderByID(
			source.APIClient,
			source.APIEndpoint,
			*gkeInst.GcpProviderID,
		)
		if err != nil {
			return fmt.Errorf("failed to get gcp provider: %w", err)
		}
		projectID = util.DerefString(gcpProvider.ProjectID)
		if projectID == "" {
			return fmt.Errorf("gcp provider for %s has no project", util.DerefString(runtimeInst.Name))
		}
	}

	// stop the writers and take one backup
	bucket, err := RestoreBackupBucket(local, "")
	if err != nil {
		return err
	}
	replicas, modules, err := BackupControlPlaneWithClient(
		namespace,
		bucket,
		source.APIClient,
		source.APIEndpoint,
		source.Kube,
		LeaveWritersStopped,
	)
	if err != nil {
		resumeErr := resumeSourceWriters(source, namespace, replicas, modules)
		if resumeErr != nil {
			return fmt.Errorf("failed to back up %s: %w (resuming source writers: %v)", namespace, err, resumeErr)
		}
		return fmt.Errorf("failed to back up %s, source writers resumed: %w", namespace, err)
	}

	// install the new control plane from the kube client, the source API is down
	created := installer.NewInstaller(
		installer.Name(newName),
		installer.Namespace(newNamespace),
	)
	created.Opts.ControlPlaneName = newName
	created.Opts.FromNamespace = namespace
	created.Opts.ControlPlaneOnly = true
	created.Opts.InfraProvider = providerName
	created.Opts.AuthEnabled = definition.AuthEnabled == nil || *definition.AuthEnabled
	created.Opts.RestApiLoadBalancer = providerName != api.KubernetesRuntimeInfraProviderKind
	created.Opts.GcpProjectId = projectID
	created.Opts.GcpBackupBucket = bucket
	if tier := util.DerefString(definition.Tier); tier != "" {
		created.Opts.Tier = installer.ControlPlaneTier(tier)
	}

	// copy the stored controller groups onto the new control plane
	if apis := util.DerefString(source.Instance.Apis); apis != "" {
		created.Opts.Apis = apis
		if apis == "none" {
			created.Opts.ControllerList = nil
		} else {
			selected, err := installer.SelectControllersByGroup(
				installer.ParseApis(apis),
				created.Opts.ControllerList,
			)
			if err != nil {
				fmt.Printf("Info: controller selection failed, resuming writers in %s\n", namespace)
				resumeErr := resumeSourceWriters(source, namespace, replicas, modules)
				if resumeErr != nil {
					return fmt.Errorf(
						"failed to select controllers: %w (resuming source writers: %v)",
						err, resumeErr,
					)
				}
				return fmt.Errorf("failed to select controllers, source writers resumed: %w", err)
			}
			created.Opts.ControllerList = selected
		}
	}
	if imageNamespace != "" {
		created.SetAllImageRepo(imageNamespace)
	} else if storedNamespace := util.DerefString(source.Instance.ImageNamespace); storedNamespace != "" {
		created.SetAllImageRepo(storedNamespace)
	}
	tag := version.GetVersion()
	if imageTag != "" {
		tag = imageTag
	}
	created.SetAllImageTags(tag)
	var grant func() error
	if providerName == api.KubernetesRuntimeInfraProviderGKE {
		grant = func() error {
			// grant the new database account access to the backup bucket
			uid, err := created.DatabaseServiceAccountUID(source.Kube)
			if err != nil {
				return err
			}
			gke := &provider.KubernetesRuntimeInfraGKE{
				PulumiWorkspace: provider.PulumiWorkspace{
					RuntimeInstanceName: util.DerefString(runtimeInst.Name),
				},
				ProjectID:    projectID,
				BackupBucket: bucket,
			}
			return gke.GrantDatabaseBackupAccess(uid)
		}
	}
	apiUp, installErr := created.InstallRestored(
		source.Kube,
		source.Mapper,
		source.APIClient,
		bucket,
		grant,
	)
	if installErr != nil && !apiUp {
		fmt.Printf("Info: new control plane is unhealthy, resuming writers in %s\n", namespace)
		resumeErr := resumeSourceWriters(source, namespace, replicas, modules)
		if resumeErr != nil {
			return fmt.Errorf("failed to install restored control plane: %w (resuming source writers: %v)", installErr, resumeErr)
		}
		return fmt.Errorf("failed to install restored control plane, source writers resumed: %w", installErr)
	}

	// record the new control plane once its API answers
	if apiUp && updateConfig {
		if err := UpdateThreeportConfig(
			threeportConfig,
			blueGreenControlPlane(local, newName, created.Opts.ExternalApiEndpoint),
		); err != nil {
			if installErr != nil {
				return fmt.Errorf(
					"new control plane API is up, source writers left stopped: %w",
					errors.Join(installErr, fmt.Errorf("failed to record client config: %w", err)),
				)
			}
			return fmt.Errorf("failed to record client config: %w", err)
		}
	}
	if installErr != nil {
		return fmt.Errorf("new control plane API is up, source writers left stopped: %w", installErr)
	}

	// bring the modules back, then remove the control plane the restore came from
	if err := created.RestoreModuleScale(source.Kube, modules); err != nil {
		return fmt.Errorf("failed to restore module scale: %w", err)
	}
	if err := removeReplacedNamespace(source.Kube, source.Mapper, namespace, newNamespace); err != nil {
		return err
	}
	fmt.Printf("Info: control plane %s is up in %s, source namespace %s deleted\n", newName, newNamespace, namespace)
	return nil
}

// delegateBlueGreen records a child blue-green for the control plane controller.
// The new name is the deployment namespace, and the image namespace already stored is left as it is.
func delegateBlueGreen(
	local *ControlPlane,
	currentClient *http.Client,
	currentEndpoint string,
	sourceName string,
	newName string,
	childTag string,
	imageNamespace string,
	imageTag string,
) error {
	// load the child from the API this command is connected to
	inst, err := client.GetControlPlaneInstanceByName(currentClient, currentEndpoint, sourceName)
	if err != nil {
		return fmt.Errorf("failed to get control plane instance: %w", err)
	}
	namespace := util.DerefString(inst.Namespace)
	if newName == "" || newName == namespace {
		return fmt.Errorf("new namespace must differ from source namespace %s", namespace)
	}
	if inst.KubernetesRuntimeInstanceID == nil {
		return fmt.Errorf("control plane %s has no runtime or definition", sourceName)
	}

	// a restore needs a GKE runtime and the bucket recorded for this control plane
	runtimeInst, err := client.GetKubernetesRuntimeInstanceByID(
		currentClient,
		currentEndpoint,
		*inst.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}
	if runtimeInst.KubernetesRuntimeDefinitionID == nil {
		return fmt.Errorf("kubernetes runtime %s has no definition", util.DerefString(runtimeInst.Name))
	}
	runtimeDef, err := client.GetKubernetesRuntimeDefinitionByID(
		currentClient,
		currentEndpoint,
		*runtimeInst.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
	}
	providerName := util.DerefString(runtimeDef.InfraProvider)
	if providerName != api.KubernetesRuntimeInfraProviderGKE {
		return fmt.Errorf(
			"%w: database backups are not provisioned for control planes on %s",
			ErrNoBackupBucket, providerName,
		)
	}
	if _, err := RestoreBackupBucket(local, ""); err != nil {
		return err
	}

	// record the new name and this tptctl version, and leave the install to the controller
	tag := childTag
	if imageTag != "" {
		tag = imageTag
	} else if tag == "" {
		tag = version.GetVersion()
	}
	if _, err := client.UpdateControlPlaneInstance(
		currentClient,
		currentEndpoint,
		childBlueGreenUpdate(inst.ID, newName, namespace, tag, imageNamespace),
	); err != nil {
		return fmt.Errorf("failed to record control plane %s: %w", newName, err)
	}
	return nil
}

// childBlueGreenUpdate is the patch a child blue-green stores.
// Name and Namespace are the new control plane, and FromNamespace is the one it replaces.
func childBlueGreenUpdate(id *uint, newName, sourceNamespace, tag, imageNamespace string) *api.ControlPlaneInstance {
	// point the child row at the new control plane and the image the controller installs
	update := &api.ControlPlaneInstance{
		Common:        api.Common{ID: id},
		Instance:      api.Instance{Name: &newName},
		Namespace:     &newName,
		FromNamespace: &sourceNamespace,
		ImageTag:      &tag,
	}
	if imageNamespace != "" {
		update.ImageNamespace = &imageNamespace
	}
	return update
}

// removeReplacedNamespace deletes the namespace the restore came from.
// The shared support-services namespace is left in place.
func removeReplacedNamespace(
	kube dynamic.Interface,
	mapper *meta.RESTMapper,
	namespace, newNamespace string,
) error {
	// keep the namespace that holds the shared operator
	if namespace == "" || namespace == newNamespace || installer.KeepSourceNamespace(namespace) {
		return nil
	}
	if err := installer.DeleteNamespaces(kube, mapper, []string{namespace}); err != nil {
		return fmt.Errorf("failed to delete source namespace %s: %w", namespace, err)
	}
	return nil
}

// blueGreenControlPlane copies the source client entry onto the new name and address.
// The credential that matches the source keeps its certificate under the new name.
func blueGreenControlPlane(source *ControlPlane, name, endpoint string) *ControlPlane {
	// copy the source entry and point it at the new control plane
	entry := *source
	if entry.Provider == api.KubernetesRuntimeInfraProviderGKE && entry.GKEProviderConfig.ClusterName == "" {
		entry.GKEProviderConfig.ClusterName = gkeClusterName(source)
	}
	entry.Name = name
	if endpoint != "" {
		entry.APIServer = endpoint
	}

	// copy credentials so renaming one does not change the source
	if source.Credentials != nil {
		entry.Credentials = make([]Credential, len(source.Credentials))
		for i, credential := range source.Credentials {
			entry.Credentials[i] = credential
			if credential.Name == source.Name {
				entry.Credentials[i].Name = name
			}
		}
	}

	// copy cleanup records so a later edit does not change the source
	if source.PendingGcpCleanup != nil {
		entry.PendingGcpCleanup = append([]PendingGcpCleanup(nil), source.PendingGcpCleanup...)
	}
	return &entry
}

// namespaceServingAPI returns the namespace of the load balancer at the API address.
// No matching service leaves the caller on the stored namespace.
func namespaceServingAPI(kube dynamic.Interface, endpoint string) (string, error) {
	// list services and match the address this client is using
	host := apiHost(endpoint)
	if host == "" {
		return "", nil
	}
	listed, err := kube.Resource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "services",
	}).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list services: %w", err)
	}
	return namespaceForAPIHost(listed.Items, host)
}

// namespaceForAPIHost returns the namespace whose load balancer has the address.
// Two namespaces with that address is an error.
func namespaceForAPIHost(services []unstructured.Unstructured, host string) (string, error) {
	// keep the one namespace that publishes the address
	found := ""
	for _, service := range services {
		if !serviceHasAddress(&service, host) {
			continue
		}
		namespace := service.GetNamespace()
		if found != "" && found != namespace {
			return "", fmt.Errorf("api address %s must belong to one namespace", host)
		}
		found = namespace
	}
	return found, nil
}

// apiHost returns the host of an API endpoint.
func apiHost(endpoint string) string {
	// drop the scheme and the port
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if host, _, err := net.SplitHostPort(endpoint); err == nil {
		return host
	}
	return endpoint
}

// serviceHasAddress reports whether the load balancer has the host.
func serviceHasAddress(service *unstructured.Unstructured, host string) bool {
	// compare the load balancer address
	ingress, found, err := unstructured.NestedSlice(
		service.Object, "status", "loadBalancer", "ingress",
	)
	if err != nil || !found {
		return false
	}
	for _, item := range ingress {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if entry["ip"] == host || entry["hostname"] == host {
			return true
		}
	}
	return false
}

// resumeSourceWriters scales a stopped control plane back to the counts recorded before its backup.
func resumeSourceWriters(
	source *ControlPlaneTarget,
	namespace string,
	replicas map[string]int64,
	modules []installer.ModuleDeploymentScale,
) error {
	sourceInstaller := installer.NewInstaller(
		installer.Name(util.DerefString(source.Instance.Name)),
		installer.Namespace(namespace),
	)
	return sourceInstaller.ResumeWriters(source.Kube, replicas, modules)
}
