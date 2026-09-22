package v0

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
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
			RuntimeInstanceName: runtimeInstanceName(cpi.Opts),
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

	createdOciProvider, err := ensureOciProvider(
		apiClient,
		threeportAPIEndpoint,
		&ociProvider,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to register default OCI provider", err)
	}

	// register oci oke k8s runtime definition, looking up first on retry
	okeRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	ociOkeKubernetesRuntimeDef := v0.OciOkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &okeRuntimeDefName,
		},
		WorkerNodeShape:               &kubernetesRuntimeInfraOKE.WorkerNodeShape,
		WorkerNodeInitialCount:        util.Ptr(kubernetesRuntimeInfraOKE.WorkerNodeInitialCount),
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
	}
	createdociOkeKubernetesRuntimeDef, err := ensureOciOkeKubernetesRuntimeDefinition(
		apiClient,
		threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeDef,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to register OCI OKE kubernetes runtime definition for control plane cluster", err)
	}

	okeRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)

	clusterOCID, err := kubernetesRuntimeInfraOKE.GetClusterOCID(okeRuntimeInstName)
	if err != nil {
		return fmt.Errorf("failed to get cluster OCID: %w", err)
	}

	// get resource inventory from pulumi state unless control-plane-only
	var resourceInventory *datatypes.JSON
	if !cpi.Opts.ControlPlaneOnly {
		if resourceInventory, err = kubernetesRuntimeInfraOKE.GetStackState(); err != nil {
			return uninstaller.cleanOnCreateError("failed to get stack state: %w", err)
		}
	}

	// create oci oke k8s runtime instance
	ociOkeKubernetesRuntimeInstance := v0.OciOkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &okeRuntimeInstName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
		OciProviderID:                       createdOciProvider.ID,
		OciOkeKubernetesRuntimeDefinitionID: createdociOkeKubernetesRuntimeDef.ID,
		KubernetesRuntimeInstanceID:         kubernetesRuntimeInstResult.ID,
		ClusterOCID:                         &clusterOCID,
		ResourceInventory:                   resourceInventory,
	}
	if _, err = ensureOciOkeKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeInstance,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to register OCI OKE kubernetes runtime instance for control plane cluster", err)
	}
	return nil
}

// ensureOciProvider returns the named OCI provider, creating it when
// the API has no row for that name.
func ensureOciProvider(
	apiClient *http.Client,
	apiEndpoint string,
	ociProvider *v0.OciProvider,
) (*v0.OciProvider, error) {
	existing, err := client.GetOciProviderByName(apiClient, apiEndpoint, *ociProvider.Name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up oci provider by name: %w", err)
	}

	created, err := client.CreateOciProvider(apiClient, apiEndpoint, ociProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create oci provider: %w", err)
	}

	return created, nil
}

// ensureOciOkeKubernetesRuntimeDefinition returns the named OKE
// runtime definition, creating it when the API has no row for that name.
func ensureOciOkeKubernetesRuntimeDefinition(
	apiClient *http.Client,
	apiEndpoint string,
	definition *v0.OciOkeKubernetesRuntimeDefinition,
) (*v0.OciOkeKubernetesRuntimeDefinition, error) {
	existing, err := client.GetOciOkeKubernetesRuntimeDefinitionByName(
		apiClient,
		apiEndpoint,
		*definition.Name,
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up oci oke kubernetes runtime definition by name: %w", err)
	}

	created, err := client.CreateOciOkeKubernetesRuntimeDefinition(apiClient, apiEndpoint, definition)
	if err != nil {
		return nil, fmt.Errorf("failed to create oci oke kubernetes runtime definition: %w", err)
	}

	return created, nil
}

// ensureOciOkeKubernetesRuntimeInstance returns the named OKE runtime
// instance, creating it when the API has no row for that name.
func ensureOciOkeKubernetesRuntimeInstance(
	apiClient *http.Client,
	apiEndpoint string,
	instance *v0.OciOkeKubernetesRuntimeInstance,
) (*v0.OciOkeKubernetesRuntimeInstance, error) {
	existing, err := client.GetOciOkeKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		*instance.Name,
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up oci oke kubernetes runtime instance by name: %w", err)
	}

	created, err := client.CreateOciOkeKubernetesRuntimeInstance(apiClient, apiEndpoint, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to create oci oke kubernetes runtime instance: %w", err)
	}

	return created, nil
}
