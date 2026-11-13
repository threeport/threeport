package v0

import (
	"errors"
	"fmt"
	"net/http"

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
		AuditLog:                      kubernetesRuntimeDefResult.AuditLog,
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
