package v0

import (
	"net/http"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
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
	createdGcpProvider, err := client.CreateGcpProvider(
		apiClient,
		threeportAPIEndpoint,
		&gcpProvider,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new default GCP provider", err)
	}

	// create GCP GKE kubernetes runtime definition
	gkeRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	// GKE uses regional clusters by default, which span 3 zones
	zoneCount := 3
	gcpGkeKubernetesRuntimeDef := v0.GcpGkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &gkeRuntimeDefName,
		},
		ZoneCount:                     &zoneCount,
		DefaultNodeGroupInstanceType:  util.Ptr("e2-medium"),
		DefaultNodeGroupInitialSize:   util.Ptr(int(kubernetesRuntimeInfraGKE.WorkerNodeInitialCount)),
		DefaultNodeGroupMinimumSize:   util.Ptr(int(kubernetesRuntimeInfraGKE.WorkerNodeInitialCount)),
		DefaultNodeGroupMaximumSize:   util.Ptr(int(kubernetesRuntimeInfraGKE.WorkerNodeInitialCount)),
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
	}
	createdGcpGkeKubernetesRuntimeDef, err := client.CreateGcpGkeKubernetesRuntimeDefinition(
		apiClient,
		threeportAPIEndpoint,
		&gcpGkeKubernetesRuntimeDef,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new GCP GKE kubernetes runtime definition for control plane cluster", err)
	}

	// get resource inventory from Pulumi state
	var resourceInventory *datatypes.JSON
	if resourceInventory, err = kubernetesRuntimeInfraGKE.GetStackState(); err != nil {
		return uninstaller.cleanOnCreateError("failed to get stack state: %w", err)
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
	}
	_, err = client.CreateGcpGkeKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		&gcpGkeKubernetesRuntimeInstance,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new GCP GKE kubernetes runtime instance for control plane cluster", err)
	}

	return nil
}
