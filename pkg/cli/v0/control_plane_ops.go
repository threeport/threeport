package v0

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/version"
	api "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
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

// resolveControlPlane loads a control plane from the current API.
// An empty name selects the control plane serving the request.
func resolveControlPlane(
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
	target, err := resolveControlPlane(currentClient, currentEndpoint, controlPlaneName, false)
	if err != nil {
		return err
	}

	// record the tag and, when set, the namespace
	module, err := client.GetModuleApiByName(target.APIClient, target.APIEndpoint, moduleName)
	if err != nil {
		return fmt.Errorf("failed to get module api: %w", err)
	}
	if imageNamespace == "" && util.DerefString(module.ImageNamespace) == "" {
		return fmt.Errorf(
			"module %s has no recorded image namespace: pass --module-image-namespace",
			moduleName,
		)
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
	target, err := resolveControlPlane(currentClient, currentEndpoint, controlPlaneName, true)
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
	_, _, err = backupControlPlaneWithClient(
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
	target, err := resolveControlPlane(currentClient, currentEndpoint, controlPlaneName, false)
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
	if err := restoreControlPlaneDatabaseWithClient(
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
		util.DerefString(source.Instance.ImageNamespace),
		util.DerefString(source.Instance.ImageTag),
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
	source, err := resolveControlPlane(currentClient, currentEndpoint, sourceName, false)
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

	// the parent api holds the catalog ids, and the child api holds the database
	parentClient, parentEndpoint := currentClient, currentEndpoint
	childClient, childEndpoint := source.APIClient, source.APIEndpoint

	// connect while the API still answers
	if err := source.clusterClient(parentClient, parentEndpoint, encryptionKey); err != nil {
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
	// The runtime id belongs to the api that stored this control plane.
	if source.Instance.KubernetesRuntimeInstanceID == nil || source.Instance.ControlPlaneDefinitionID == nil {
		return fmt.Errorf("control plane %s has no runtime or definition", sourceName)
	}
	runtimeInst, err := client.GetKubernetesRuntimeInstanceByID(
		parentClient,
		parentEndpoint,
		*source.Instance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}
	if runtimeInst.KubernetesRuntimeDefinitionID == nil {
		return fmt.Errorf("kubernetes runtime %s has no definition", util.DerefString(runtimeInst.Name))
	}
	runtimeDef, err := client.GetKubernetesRuntimeDefinitionByID(
		parentClient,
		parentEndpoint,
		*runtimeInst.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
	}
	providerName := util.DerefString(runtimeDef.InfraProvider)
	definition, err := client.GetControlPlaneDefinitionByID(
		parentClient,
		parentEndpoint,
		*source.Instance.ControlPlaneDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get control plane definition: %w", err)
	}

	// a GKE restore needs the project before the API is stopped
	projectID := ""
	providerCredentials := ""
	if providerName == api.KubernetesRuntimeInfraProviderGKE {
		gkeInst, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
			parentClient,
			parentEndpoint,
			*source.Instance.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get gke runtime: %w", err)
		}
		if gkeInst.GcpProviderID == nil {
			return fmt.Errorf("gke runtime %s has no provider", util.DerefString(runtimeInst.Name))
		}
		gcpProvider, err := client.GetGcpProviderByID(
			parentClient,
			parentEndpoint,
			*gkeInst.GcpProviderID,
		)
		if err != nil {
			return fmt.Errorf("failed to get gcp provider: %w", err)
		}
		projectID = util.DerefString(gcpProvider.ProjectID)
		if projectID == "" {
			return fmt.Errorf("gcp provider for %s has no project", util.DerefString(runtimeInst.Name))
		}
		// decrypt the stored service account for the bucket grant
		providerCredentials, err = decryptedProviderCredentials(encryptionKey, gcpProvider.ServiceAccountCredentials)
		if err != nil {
			return err
		}
	}

	// stop the writers and take one backup
	bucket, err := RestoreBackupBucket(local, "")
	if err != nil {
		return err
	}

	// let the source database write the backup it is about to take, before
	// anything is stopped
	if providerName == api.KubernetesRuntimeInfraProviderGKE {
		if err := GrantSourceDatabaseBackup(
			namespace,
			source.Kube,
			util.DerefString(runtimeInst.Name),
			projectID,
			bucket,
			providerCredentials,
		); err != nil {
			return err
		}
	}

	replicas, modules, err := backupControlPlaneWithClient(
		namespace,
		bucket,
		childClient,
		childEndpoint,
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
	created.Opts.SourceName = util.DerefString(source.Instance.Name)
	created.Opts.ControlPlaneOnly = true
	created.Opts.InfraProvider = providerName
	created.Opts.AuthEnabled = definition.AuthEnabled == nil || *definition.AuthEnabled
	created.Opts.RestApiLoadBalancer = providerName != api.KubernetesRuntimeInfraProviderKind
	created.Opts.GcpProjectId = projectID
	created.Opts.GcpBackupBucket = bucket
	created.Opts.HoldReplicasForRestore = true
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
		// let the new namespace's controllers call Google as the cluster account
		workloadIdentity := &provider.KubernetesRuntimeInfraGKE{
			PulumiWorkspace: provider.PulumiWorkspace{
				RuntimeInstanceName: util.DerefString(runtimeInst.Name),
			},
			ProjectID:                 projectID,
			ServiceAccountCredentials: providerCredentials,
		}

		// use the account the runtime's credentials belong to, else the cluster's own
		var bindErr error
		if providerCredentials != "" {
			workloadIdentity.ServiceAccountEmail, bindErr = util.GcpServiceAccountEmail(providerCredentials)
		}
		if bindErr == nil {
			bindErr = workloadIdentity.BindControlPlaneWorkloadIdentity(newNamespace)
		}
		if bindErr != nil {
			fmt.Printf("Info: workload identity binding failed, resuming writers in %s\n", namespace)
			resumeErr := resumeSourceWriters(source, namespace, replicas, modules)
			if resumeErr != nil {
				return fmt.Errorf("failed to bind workload identity: %w (resuming source writers: %v)", bindErr, resumeErr)
			}
			return fmt.Errorf("failed to bind workload identity, source writers resumed: %w", bindErr)
		}
		created.Opts.GcpServiceAccountEmail = workloadIdentity.ServiceAccountEmail

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
				ProjectID:                 projectID,
				BackupBucket:              bucket,
				ServiceAccountCredentials: providerCredentials,
			}
			return gke.GrantDatabaseBackupAccess(uid)
		}
	}
	apiUp, installErr := created.InstallRestored(
		source.Kube,
		source.Mapper,
		childClient,
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

	// run the support services operator in the new namespace, which only a
	// control plane with no parent does
	if runsSupportServices(source.Instance) {
		if err := installer.InstallThreeportSupportServicesOperator(
			source.Kube,
			source.Mapper,
			installer.SupportServicesCloud(api.KubernetesRuntimeInfraProvider(providerName)),
			newNamespace,
		); err != nil {
			return fmt.Errorf("new control plane API is up, source writers left stopped: failed to install the support services operator: %w", err)
		}
	}

	// bring the modules back, then remove the control plane the restore came from
	if err := created.RestoreModuleScale(source.Kube, modules); err != nil {
		return fmt.Errorf("failed to restore module scale: %w", err)
	}
	if err := removeReplacedNamespace(
		source.Kube,
		source.Mapper,
		namespace,
		newNamespace,
		runsSupportServices(source.Instance),
	); err != nil {
		return err
	}
	fmt.Printf("Info: control plane %s is up in %s\n", newName, newNamespace)
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
	namespace := sourceNamespace(inst)
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

	// reject a name another control plane already holds, before the source is removed
	found, lookupErr := client.GetControlPlaneInstanceByName(currentClient, currentEndpoint, newName)
	sourceID := uint(0)
	if inst.ID != nil {
		sourceID = *inst.ID
	}
	if err := ReplacementNameTaken(sourceID, newName, found, lookupErr); err != nil {
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
		childBlueGreenUpdate(inst.ID, util.DerefString(inst.Name), newName, namespace, tag, imageNamespace),
	); err != nil {
		return fmt.Errorf("failed to record control plane %s: %w", newName, err)
	}
	fmt.Printf("Info: recorded blue-green request for %s\n", newName)
	return waitForChildBlueGreen(currentClient, currentEndpoint, *inst.ID, util.DerefString(inst.Name))
}

// sourceNamespace returns the namespace a blue-green request replaces. A
// request still pending on the instance already moved its namespace to the new
// name and recorded the replaced one as the source, so the source is read from
// there first.
func sourceNamespace(instance *api.ControlPlaneInstance) string {
	// a pending request names the namespace it replaces
	if from := util.DerefString(instance.FromNamespace); from != "" {
		return from
	}

	return util.DerefString(instance.Namespace)
}

// decryptedProviderCredentials returns the service account JSON the API stores encrypted.
func decryptedProviderCredentials(key string, stored *string) (string, error) {
	// an empty field leaves the caller on its process identity
	if stored == nil || *stored == "" {
		return "", nil
	}
	decrypted, err := encryption.Decrypt(key, *stored)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt gcp provider credentials: %w", err)
	}
	return decrypted, nil
}

// childBlueGreenWait matches how long the controller keeps retrying one reconcile.
const childBlueGreenWait = time.Hour

// waitForChildBlueGreen returns when the controller clears the recorded request.
// It returns an error when the wait passes an hour.
func waitForChildBlueGreen(apiClient *http.Client, apiEndpoint string, id uint, name string) error {
	// the controller clears FromNamespace after the new namespace is up
	deadline := time.Now().Add(childBlueGreenWait)
	for {
		inst, err := client.GetControlPlaneInstanceByID(apiClient, apiEndpoint, id)
		// a brief API error is not a failed upgrade; the controller keeps working
		if err != nil {
			if time.Now().After(deadline) {
				return fmt.Errorf("failed to get control plane %s: %w", name, err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		if inst.FromNamespace == nil || *inst.FromNamespace == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("control plane %s did not finish blue-green within %s", name, childBlueGreenWait)
		}
		time.Sleep(5 * time.Second)
	}
}

// ErrReplacementNameTaken is returned when another control plane already has the new name.
var ErrReplacementNameTaken = errors.New("control plane name is already in use")

// ReplacementNameTaken reports another row already holds newName.
// A missing row is free. The source row may already carry the name.
func ReplacementNameTaken(sourceID uint, newName string, found *api.ControlPlaneInstance, lookupErr error) error {
	// a missing name is free to take
	if lookupErr != nil {
		// a missing name is returned as a plain error, not ErrObjectNotFound
		if errors.Is(lookupErr, client_lib.ErrObjectNotFound) || strings.Contains(lookupErr.Error(), "no local control plane instance") {
			return nil
		}
		return lookupErr
	}
	if found != nil && found.ID != nil && *found.ID != sourceID {
		return fmt.Errorf("control plane %s: %w", newName, ErrReplacementNameTaken)
	}
	return nil
}

// childBlueGreenUpdate is the patch a child blue-green stores.
// Name stays the current control plane until the install finishes.
// Namespace is the new deployment, and FromNamespace is the one it replaces.
func childBlueGreenUpdate(id *uint, currentName, newName, sourceNamespace, tag, imageNamespace string) *api.ControlPlaneInstance {
	// record the new deployment and the image the controller installs
	update := &api.ControlPlaneInstance{
		Common:        api.Common{ID: id},
		Instance:      api.Instance{Name: &currentName},
		Namespace:     &newName,
		FromNamespace: &sourceNamespace,
		ImageTag:      &tag,
	}
	if imageNamespace != "" {
		update.ImageNamespace = &imageNamespace
	}
	return update
}

// GrantSourceDatabaseBackup grants the database account of a namespace being
// replaced access to the runtime's backup bucket, so its backup can be written.
// Every upgrade that backs up a source calls it, since a control plane installed
// before it had a backup bucket has no grant of its own. Credentials are the
// runtime's own, or empty to use the caller's identity.
func GrantSourceDatabaseBackup(
	sourceNamespace string,
	kube dynamic.Interface,
	runtimeName string,
	projectID string,
	bucket string,
	credentials string,
) error {
	// grant the account the source namespace's database runs as
	return grantSourceDatabase(
		installer.NewInstaller(installer.Namespace(sourceNamespace)),
		kube,
		func(uid string) error {
			return (&provider.KubernetesRuntimeInfraGKE{
				PulumiWorkspace: provider.PulumiWorkspace{
					RuntimeInstanceName: runtimeName,
				},
				ProjectID:                 projectID,
				BackupBucket:              bucket,
				ServiceAccountCredentials: credentials,
			}).GrantDatabaseBackupAccess(uid)
		},
	)
}

// grantSourceDatabase grants the database account of the control plane being
// replaced access to the backup bucket, so its backup can be written. A control
// plane installed before it had a backup bucket has no grant of its own.
func grantSourceDatabase(
	source *installer.ControlPlaneInstaller,
	kube dynamic.Interface,
	grant func(uid string) error,
) error {
	// read the source database account
	uid, err := source.DatabaseServiceAccountUID(kube)
	if err != nil {
		return fmt.Errorf("failed to read the source database account: %w", err)
	}

	// grant it object read and write on the bucket
	if err := grant(uid); err != nil {
		return fmt.Errorf("failed to grant backup access to the source database: %w", err)
	}

	return nil
}

// runsSupportServices reports whether a control plane runs the support
// services operator, which only a control plane with no parent does.
func runsSupportServices(instance *api.ControlPlaneInstance) bool {
	// a child control plane shares its parent's operator
	return instance != nil && instance.ParentControlPlaneInstanceID == nil
}

// removeReplacedNamespace deletes the namespace the restore came from.
// The original namespace is kept, and the StatefulSets of the replaced control
// plane in it are scaled to zero. Its support services operator is stopped only
// when the new control plane took the operator over.
func removeReplacedNamespace(
	kube dynamic.Interface,
	mapper *meta.RESTMapper,
	namespace, newNamespace string,
	stopOperator bool,
) error {
	// leave a namespace that is empty or is the new control plane's
	if namespace == "" || namespace == newNamespace {
		return nil
	}

	// keep the original namespace and stop what runs in it
	if installer.KeepSourceNamespace(namespace) {
		scaled, err := installer.ScaleStatefulSetsToZero(kube, namespace)
		if err != nil {
			return fmt.Errorf("failed to scale down the kept source namespace %s: %w", namespace, err)
		}
		fmt.Printf(
			"Info: kept source namespace %s and scaled its StatefulSets to zero: %s\n",
			namespace,
			strings.Join(scaled, ", "),
		)

		// stop the operator only when the new control plane runs its own
		if stopOperator {
			if err := installer.ScaleSupportServicesOperatorToZero(kube, namespace); err != nil {
				return fmt.Errorf("failed to stop the support services operator in %s: %w", namespace, err)
			}
			fmt.Printf("Info: stopped the support services operator in %s\n", namespace)
		}
		return nil
	}

	// delete the replaced control plane namespace
	fmt.Printf("Info: deleting source namespace %s\n", namespace)
	if err := installer.DeleteNamespaces(kube, mapper, []string{namespace}); err != nil {
		return fmt.Errorf("failed to delete source namespace %s: %w", namespace, err)
	}
	fmt.Printf("Info: deleted source namespace %s\n", namespace)
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
