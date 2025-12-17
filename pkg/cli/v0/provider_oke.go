package v0

import (
	"fmt"
	"net/http"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"gorm.io/datatypes"
)

// DeployOkeInfra deploys the OKE infrastructure for the control plane.
func DeployOkeInfra(
	cpi *threeport.ControlPlaneInstaller,
	threeportControlPlaneConfig *ControlPlane,
	threeportConfig *ThreeportConfig,
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra,
	kubeConnectionInfo *kube.KubeConnectionInfo,
	uninstaller *Uninstaller,
) error {
	// create OKE infrastructure
	kubernetesRuntimeInfraOKE := provider.KubernetesRuntimeInfraOKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
			ProjectName:         "oke",
			ProjectDescription:  "Oracle Kubernetes Engine (OKE) cluster for Threeport",
			// StackConfigs set by LoadOCIConfig after region is resolved
		},
		WorkerNodeShape:        "VM.Standard.A1.Flex",
		Version:                provider.DefaultOKEKubernetesVersion,
		WorkerNodeInitialCount: int32(2),
		Region:                 cpi.Opts.OciRegion,
	}
	*kubernetesRuntimeInfra = &kubernetesRuntimeInfraOKE
	uninstaller.kubernetesRuntimeInfra = &kubernetesRuntimeInfraOKE

	// load OCI config and set overridden values if provided
	// by a command line flag
	if err := kubernetesRuntimeInfraOKE.LoadOCIConfig(
		cpi.Opts.OciRegion,
		cpi.Opts.OciConfigProfile,
		threeportControlPlaneConfig.OKEProviderConfig.OciCompartmentOcid,
	); err != nil {
		return fmt.Errorf("failed to load OCI config: %w", err)
	}

	// update threeport config with oke provider info
	var err error
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		existingCompartmentOcid := c.OKEProviderConfig.OciCompartmentOcid
		c.OKEProviderConfig = OKEProviderConfig{
			OciRegion:          cpi.Opts.OciRegion,
			OciConfigProfile:   cpi.Opts.OciConfigProfile,
			OciCompartmentOcid: existingCompartmentOcid,
		}
	}); err != nil {
		return fmt.Errorf("failed to update threeport config: %w", err)
	}

	if cpi.Opts.ControlPlaneOnly {
		// populate service user credentials from the OCI config provider
		// so the OCI provider record has valid credentials for token refresh
		if err := kubernetesRuntimeInfraOKE.LoadServiceCredentialsFromConfig(); err != nil {
			return fmt.Errorf("failed to load OCI service credentials from config: %w", err)
		}

		connectionInfo, err := kubernetesRuntimeInfraOKE.GetConnection()
		if err != nil {
			return fmt.Errorf("failed to get connection info for OKE kubernetes runtime: %w", err)
		}
		*kubeConnectionInfo = *connectionInfo
	} else {
		connectionInfo, err := (*kubernetesRuntimeInfra).Create()
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to create control plane infra for threeport", err)
		}
		*kubeConnectionInfo = *connectionInfo

		// update threeport config with compartment OCID after bootstrap creates it
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
			c.OKEProviderConfig.OciCompartmentOcid = kubernetesRuntimeInfraOKE.CompartmentOCID
		}); err != nil {
			return fmt.Errorf("failed to update threeport config with compartment OCID: %w", err)
		}
	}

	return nil
}

// ConfigureControlPlaneWithOkeConfig configures the control plane with the OKE config.
func ConfigureControlPlaneWithOkeConfig(
	cpi *threeport.ControlPlaneInstaller,
	uninstaller *Uninstaller,
	apiClient *http.Client,
	threeportAPIEndpoint string,
	kubernetesRuntimeDefResult *v0.KubernetesRuntimeDefinition,
	kubernetesRuntimeInstResult *v0.KubernetesRuntimeInstance,
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra,
) error {
	kubernetesRuntimeInfraOKE := (*kubernetesRuntimeInfra).(*provider.KubernetesRuntimeInfraOKE)

	// derive tenancy OCID from the config provider
	tenancyOCID, err := kubernetesRuntimeInfraOKE.ConfigProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID from config provider: %w", err)
	}

	// create OCI provider using the service user credentials generated during bootstrap.
	// CompartmentOCID stores the genesis compartment — workload clusters create
	// child compartments under it.
	ociProvider := v0.OciProvider{
		Name:            util.Ptr(kubernetesRuntimeInfraOKE.GetServiceUserName()),
		UserOCID:        &kubernetesRuntimeInfraOKE.ServiceUserOCID,
		TenancyOCID:     &tenancyOCID,
		CompartmentOCID: &kubernetesRuntimeInfraOKE.CompartmentOCID,
		DefaultProvider: util.Ptr(true),
		DefaultRegion:   &kubernetesRuntimeInfraOKE.Region,
		KeyFingerprint:  &kubernetesRuntimeInfraOKE.Fingerprint,
		PrivateKey:      &kubernetesRuntimeInfraOKE.PrivateKeyPEM,
	}

	if _, err = client.CreateOciProvider(
		apiClient,
		threeportAPIEndpoint,
		&ociProvider,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to create new default OCI provider", err)
	}

	// create oci oke k8s runtime definition
	okeRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	ociOkeKubernetesRuntimeDef := v0.OciOkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &okeRuntimeDefName,
		},
		WorkerNodeShape:               &kubernetesRuntimeInfraOKE.WorkerNodeShape,
		WorkerNodeInitialCount:        util.Ptr(kubernetesRuntimeInfraOKE.WorkerNodeInitialCount),
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
	}
	createdociOkeKubernetesRuntimeDef, err := client.CreateOciOkeKubernetesRuntimeDefinition(
		apiClient,
		threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeDef,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new OCI OKE kubernetes runtime definition for control plane cluster", err)
	}

	okeRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)

	clusterOCID, err := kubernetesRuntimeInfraOKE.GetClusterOCID(okeRuntimeInstName)
	if err != nil {
		return fmt.Errorf("failed to get cluster OCID: %w", err)
	}

	// get resource inventory from Pulumi state
	var resourceInventory *datatypes.JSON
	if resourceInventory, err = kubernetesRuntimeInfraOKE.GetStackState(); err != nil {
		return uninstaller.cleanOnCreateError("failed to get stack state: %w", err)
	}

	// create oci oke k8s runtime instance
	ociOkeKubernetesRuntimeInstance := v0.OciOkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &okeRuntimeInstName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
		OciProviderID:                       ociProvider.ID,
		OciOkeKubernetesRuntimeDefinitionID: createdociOkeKubernetesRuntimeDef.ID,
		KubernetesRuntimeInstanceID:         kubernetesRuntimeInstResult.ID,
		ClusterOCID:                         &clusterOCID,
		ResourceInventory:                   resourceInventory,
	}
	_, err = client.CreateOciOkeKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeInstance,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new OCI OKE kubernetes runtime instance for control plane cluster", err)
	}
	return nil
}
