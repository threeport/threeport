package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"gorm.io/datatypes"
)

// ConfigureControlPlaneWithGkeConfig creates the following objects in the Threeport API:
// - the default GCP provider that was used to create the Threeport control plane runtime environment
// - the GCP GKE kubernetes runtime definition that was used to create the GKE kubernetes runtime for the control plane
// - the GCP GKE kubernetes runtime instance that was used to create the GKE kubernetes runtime for the control plane
func ConfigureControlPlaneWithGkeConfig(
	cpi *threeport.ControlPlaneInstaller,
	uninstaller *Uninstaller,
	apiClient *http.Client,
	threeportAPIEndpoint string,
	kubernetesRuntimeDefResult *v0.KubernetesRuntimeDefinition,
	kubernetesRuntimeInstResult *v0.KubernetesRuntimeInstance,
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra,
) error {

	kubernetesRuntimeInfraGKE := (*kubernetesRuntimeInfra).(*provider.KubernetesRuntimeInfraGKE)

	// create default GCP provider
	gcpProvider := v0.GcpProvider{
		Name:            util.Ptr(provider.DefaultAccountName),
		ProjectID:       &kubernetesRuntimeInfraGKE.ProjectID,
		DefaultProvider: util.Ptr(true),
		DefaultRegion:   &kubernetesRuntimeInfraGKE.Region,
	}
	createdGcpProvider, err := ensureGcpProvider(
		apiClient,
		threeportAPIEndpoint,
		&gcpProvider,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to register default GCP provider", err)
	}

	// create GCP GKE kubernetes runtime definition
	gkeRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	// GKE uses regional clusters by default, which span 3 zones
	zoneCount := 3
	gcpGkeKubernetesRuntimeDef := v0.GcpGkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &gkeRuntimeDefName,
		},
		ZoneCount:                    &zoneCount,
		DefaultNodeGroupInstanceType: &kubernetesRuntimeInfraGKE.MachineType,
		DefaultNodeGroupInitialSize:  util.Ptr(int(kubernetesRuntimeInfraGKE.WorkerNodeInitialCount)),
		DefaultNodeGroupMinimumSize:  util.Ptr(int(kubernetesRuntimeInfraGKE.MinNodeCount)),
		DefaultNodeGroupMaximumSize:  util.Ptr(int(kubernetesRuntimeInfraGKE.MaxNodeCount)),
		PrivateApi:                   privateApiField(kubernetesRuntimeInfraGKE.PrivateApi),
		AllowedCidrs:                 allowedCidrsJSON(kubernetesRuntimeInfraGKE.AllowedCidrs),
		// the version tptctl created the cluster on, none for an existing one
		KubernetesVersion:             kubernetesRuntimeDefResult.KubernetesVersion,
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
	}
	// a cluster tptctl didn't create, with --control-plane-only, records none
	if !cpi.Opts.ControlPlaneOnly {
		recordGkeUpgradeSettings(
			&gcpGkeKubernetesRuntimeDef,
			kubernetesRuntimeInfraGKE.CreatedReleaseChannel(),
			kubernetesRuntimeInfraGKE,
		)
	}
	createdGcpGkeKubernetesRuntimeDef, err := ensureGcpGkeKubernetesRuntimeDefinition(
		apiClient,
		threeportAPIEndpoint,
		&gcpGkeKubernetesRuntimeDef,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to register GCP GKE kubernetes runtime definition for control plane cluster", err)
	}

	// get resource inventory from pulumi state unless control-plane-only
	var resourceInventory *datatypes.JSON
	if !cpi.Opts.ControlPlaneOnly {
		if resourceInventory, err = kubernetesRuntimeInfraGKE.GetStackState(); err != nil {
			return uninstaller.cleanOnCreateError("failed to get stack state: %w", err)
		}
	}

	// create GCP GKE kubernetes runtime instance
	gkeRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	gcpGkeKubernetesRuntimeInstance := v0.GcpGkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &gkeRuntimeInstName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
		GcpProviderID:                       createdGcpProvider.ID,
		Region:                              &kubernetesRuntimeInfraGKE.Region,
		GcpGkeKubernetesRuntimeDefinitionID: createdGcpGkeKubernetesRuntimeDef.ID,
		KubernetesRuntimeInstanceID:         kubernetesRuntimeInstResult.ID,
		ResourceInventory:                   resourceInventory,
		ReservedNatAddress:                  util.Ptr(kubernetesRuntimeInfraGKE.ReserveNatAddress),
	}
	if _, err = ensureGcpGkeKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		&gcpGkeKubernetesRuntimeInstance,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to register GCP GKE kubernetes runtime instance for control plane cluster", err)
	}

	return nil
}

// gkeReleaseChannel returns the release channel of the cluster tptctl up
// creates for a control plane of tier.
func gkeReleaseChannel(tier threeport.ControlPlaneTier) string {
	if tier == threeport.ControlPlaneTier(threeport.ControlPlaneTierProd) {
		return v0.ProductionGkeReleaseChannel
	}

	return v0.DefaultGkeReleaseChannel
}

// recordGkeUpgradeSettings records on a GKE definition the release channel
// and maintenance window tptctl up created its cluster with.  The channel is
// the infra's CreatedReleaseChannel: a create resumed on a stack that already
// held the cluster kept its channel, and one GKE chose, "", isn't recorded.
func recordGkeUpgradeSettings(
	definition *v0.GcpGkeKubernetesRuntimeDefinition,
	channel string,
	infra *provider.KubernetesRuntimeInfraGKE,
) {
	if channel != "" {
		definition.ReleaseChannel = &channel
	}
	definition.MaintenanceWindowDays = util.Ptr(v0.GkeMaintenanceWindowDaysOrDefault(&infra.MaintenanceWindowDays))
	definition.MaintenanceWindowStart = util.Ptr(v0.GkeMaintenanceWindowStartOrDefault(&infra.MaintenanceWindowStart))
	definition.MaintenanceWindowHours = util.Ptr(v0.GkeMaintenanceWindowHoursOrDefault(&infra.MaintenanceWindowHours))
}

// ensureGcpProvider returns the named GCP provider, creating it when
// the API has no row for that name.
func ensureGcpProvider(
	apiClient *http.Client,
	apiEndpoint string,
	gcpProvider *v0.GcpProvider,
) (*v0.GcpProvider, error) {
	existing, err := client.GetGcpProviderByName(apiClient, apiEndpoint, *gcpProvider.Name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up gcp provider by name: %w", err)
	}

	created, err := client.CreateGcpProvider(apiClient, apiEndpoint, gcpProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp provider: %w", err)
	}

	return created, nil
}

// ensureGcpGkeKubernetesRuntimeDefinition returns the named GKE
// runtime definition, creating it when the API has no row for that name.
func ensureGcpGkeKubernetesRuntimeDefinition(
	apiClient *http.Client,
	apiEndpoint string,
	definition *v0.GcpGkeKubernetesRuntimeDefinition,
) (*v0.GcpGkeKubernetesRuntimeDefinition, error) {
	existing, err := client.GetGcpGkeKubernetesRuntimeDefinitionByName(
		apiClient,
		apiEndpoint,
		*definition.Name,
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up gcp gke kubernetes runtime definition by name: %w", err)
	}

	created, err := client.CreateGcpGkeKubernetesRuntimeDefinition(apiClient, apiEndpoint, definition)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp gke kubernetes runtime definition: %w", err)
	}

	return created, nil
}

// ensureGcpGkeKubernetesRuntimeInstance returns the named GKE runtime
// instance, creating it when the API has no row for that name.
func ensureGcpGkeKubernetesRuntimeInstance(
	apiClient *http.Client,
	apiEndpoint string,
	instance *v0.GcpGkeKubernetesRuntimeInstance,
) (*v0.GcpGkeKubernetesRuntimeInstance, error) {
	existing, err := client.GetGcpGkeKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		*instance.Name,
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up gcp gke kubernetes runtime instance by name: %w", err)
	}

	created, err := client.CreateGcpGkeKubernetesRuntimeInstance(apiClient, apiEndpoint, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp gke kubernetes runtime instance: %w", err)
	}

	return created, nil
}

// gkeRecordedBackupBucket returns the backup bucket recorded for a GKE control plane.
func gkeRecordedBackupBucket(controlPlane *ControlPlane) string {
	return controlPlane.GKEProviderConfig.BackupBucket
}

// gkeClusterName returns the GKE cluster a control plane runs on.
// An empty recorded name is the cluster tptctl provisions, threeport- plus the control plane name.
func gkeClusterName(controlPlane *ControlPlane) string {
	// use the cluster recorded at install
	if controlPlane.GKEProviderConfig.ClusterName != "" {
		return controlPlane.GKEProviderConfig.ClusterName
	}

	// a provisioned cluster is named threeport- plus the control plane name
	return provider.ThreeportRuntimeName(controlPlane.Name)
}

// RequireGkeLocation refuses an empty GCP project or region, which would
// otherwise be filled from the caller's current Google defaults and could
// name another cluster.
func RequireGkeLocation(projectID, region string) error {
	if projectID == "" || region == "" {
		return fmt.Errorf("the GCP project %q and region %q must both be known", projectID, region)
	}

	return nil
}

// gkeInfraForTier builds the GKE infrastructure a fresh install creates for a tier.
// The caller sets the project, region, and endpoint access.
func gkeInfraForTier(
	clusterName string,
	tier threeport.ControlPlaneTier,
) (*provider.KubernetesRuntimeInfraGKE, error) {
	// take the node pool the tier gets
	gkeProfile, err := gkeControlPlaneProfile(tier)
	if err != nil {
		return nil, err
	}

	return &provider.KubernetesRuntimeInfraGKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: clusterName,
		},
		ReleaseChannel:         gkeReleaseChannel(tier),
		WorkerNodeInitialCount: gkeProfile.InitialNodeCount,
		MachineType:            gkeProfile.MachineType,
		MinNodeCount:           gkeProfile.MinNodeCount,
		MaxNodeCount:           gkeProfile.MaxNodeCount,
		LocationPolicy:         gkeProfile.LocationPolicy,
		// a production control plane backs its database up to a
		// bucket in another region
		ProvisionBackups: tier == threeport.ControlPlaneTier(threeport.ControlPlaneTierProd),
		// the control plane's outbound traffic comes from one address,
		// which clusters it manages admit
		ReserveNatAddress: true,
	}, nil
}

// GkeInfraForUpgrade builds the GKE infrastructure object for an in-place upgrade.
// It holds what a fresh install of the tier creates, so the preview shows every difference from the deployed stack.
// The deployed stack fills an empty project or region so Google defaults cannot select another cluster.
func GkeInfraForUpgrade(controlPlane *ControlPlane, tier string) (*provider.KubernetesRuntimeInfraGKE, error) {
	// refuse a missing control plane
	if controlPlane == nil {
		return nil, fmt.Errorf("control plane is missing")
	}

	// refuse a control plane that is not on gke
	if controlPlane.Provider != v0.KubernetesRuntimeInfraProviderGKE {
		return nil, fmt.Errorf("control plane %s is not a GKE control plane", controlPlane.Name)
	}

	// build the tier's infrastructure and copy the recorded project, region, and endpoint access
	infra, err := gkeInfraForTier(gkeClusterName(controlPlane), threeport.ControlPlaneTier(tier))
	if err != nil {
		return nil, err
	}
	infra.ProjectID = controlPlane.GKEProviderConfig.GcpProjectId
	infra.Region = controlPlane.GKEProviderConfig.GcpRegion
	infra.PrivateApi = controlPlane.GKEProviderConfig.PrivateApi != nil && *controlPlane.GKEProviderConfig.PrivateApi
	infra.AllowedCidrs = controlPlane.GKEProviderConfig.AllowedCidrs

	// read project and region from the stack when the config omits either
	if infra.ProjectID == "" || infra.Region == "" {
		if err := infra.LoadConfigFromStack(); err != nil {
			return nil, fmt.Errorf(
				"control plane %s records no GCP project and region, and they could not be read from its Pulumi stack: %w",
				controlPlane.Name, err,
			)
		}
	}

	// require a project and a region
	if err := RequireGkeLocation(infra.ProjectID, infra.Region); err != nil {
		return nil, fmt.Errorf("cannot find the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	return infra, nil
}

// KeepRecordedNatAddress copies the recorded NAT address setting onto an upgrade,
// so an existing cluster keeps the egress address it has.
func KeepRecordedNatAddress(
	infra *provider.KubernetesRuntimeInfraGKE,
	apiClient *http.Client,
	apiEndpoint string,
	clusterName string,
) error {
	// load the recorded instance
	gkeInstance, err := client.GetGcpGkeKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, clusterName)
	if err != nil {
		return fmt.Errorf("recorded GKE instance %s was not found: %w", clusterName, err)
	}

	// copy the recorded setting
	infra.ReserveNatAddress = util.PtrEqual(gkeInstance.ReservedNatAddress, true)

	return nil
}

// RecordGkePool writes the node pool an upgrade applied to the cluster's
// recorded definition, so the record matches the deployed cluster.
func RecordGkePool(
	infra *provider.KubernetesRuntimeInfraGKE,
	apiClient *http.Client,
	apiEndpoint string,
	clusterName string,
) error {
	// load the recorded definition
	gkeInstance, err := client.GetGcpGkeKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, clusterName)
	if err != nil {
		return fmt.Errorf("recorded GKE instance %s was not found: %w", clusterName, err)
	}
	runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *gkeInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("recorded runtime for %s was not found: %w", clusterName, err)
	}
	definition, err := client.GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef(apiClient, apiEndpoint, *runtime.KubernetesRuntimeDefinitionID)
	if err != nil {
		return fmt.Errorf("recorded GKE definition for %s was not found: %w", clusterName, err)
	}

	// write the applied machine type and sizes
	if _, err := client.UpdateGcpGkeKubernetesRuntimeDefinition(
		apiClient,
		apiEndpoint,
		&v0.GcpGkeKubernetesRuntimeDefinition{
			Common:                       v0.Common{ID: definition.ID},
			DefaultNodeGroupInstanceType: util.Ptr(infra.MachineType),
			DefaultNodeGroupInitialSize:  util.Ptr(int(infra.WorkerNodeInitialCount)),
			DefaultNodeGroupMinimumSize:  util.Ptr(int(infra.MinNodeCount)),
			DefaultNodeGroupMaximumSize:  util.Ptr(int(infra.MaxNodeCount)),
		},
	); err != nil {
		return fmt.Errorf("failed to record the node pool for %s: %w", clusterName, err)
	}

	return nil
}

// InstalledImageNamespace reads the image namespace the control plane's API
// server runs from, such as "us-central1-docker.pkg.dev/my-project/threeport".
func InstalledImageNamespace(controlPlane *ControlPlane, namespace string) (string, error) {
	// connect to the control plane's cluster
	kubeClient, err := gkeControlPlaneKubeClient(controlPlane)
	if err != nil {
		return "", fmt.Errorf("failed to connect to the control plane's cluster: %w", err)
	}

	// read the API server's image
	deployment, err := kubeClient.Resource(
		schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	).Namespace(namespace).Get(context.Background(), threeport.ThreeportAPIServiceResourceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get the API server in %s: %w", namespace, err)
	}
	containers, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	if err != nil || len(containers) == 0 {
		return "", fmt.Errorf("the API server in %s runs no container", namespace)
	}
	container, _ := containers[0].(map[string]interface{})
	image, _ := container["image"].(string)

	return imageNamespaceOf(image)
}

// imageNamespaceOf returns an image reference without its name and tag.
func imageNamespaceOf(image string) (string, error) {
	// drop a digest, then the name and tag after the last slash
	withoutDigest, _, _ := strings.Cut(image, "@")
	slash := strings.LastIndex(withoutDigest, "/")
	if slash < 0 {
		return "", fmt.Errorf("image %q names no namespace", image)
	}

	return withoutDigest[:slash], nil
}

// gkeControlPlaneKubeClient returns a client for the GKE cluster a control
// plane runs on, authenticated as the caller's own Google credentials.
func gkeControlPlaneKubeClient(controlPlane *ControlPlane) (dynamic.Interface, error) {
	infra := (&provider.KubernetesRuntimeInfraGKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: gkeClusterName(controlPlane),
		},
		ProjectID: controlPlane.GKEProviderConfig.GcpProjectId,
		Region:    controlPlane.GKEProviderConfig.GcpRegion,
	}).InitPulumiProject()

	// the cluster is found by project and region, which are left to the
	// caller's current Google defaults when empty, so a config from an
	// install that did not record them reads them from the cluster's own
	// Pulumi stack instead
	if infra.ProjectID == "" || infra.Region == "" {
		if err := infra.LoadConfigFromStack(); err != nil {
			return nil, fmt.Errorf(
				"control plane %s records no GCP project and region, and they could not be read from its Pulumi stack: %w",
				controlPlane.Name, err,
			)
		}
	}
	if err := RequireGkeLocation(infra.ProjectID, infra.Region); err != nil {
		return nil, fmt.Errorf("cannot find the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	connection, err := infra.GetConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	// a restore runs for longer than one access token lasts, so every
	// request to the cluster carries a current one
	tokenGenerator, err := infra.TokenGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to get Google credentials for the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}
	kubeClient, err := refreshingKubeClient(connection, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("failed to get a client for the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	return kubeClient, nil
}
