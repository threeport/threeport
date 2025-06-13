package v0

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"gorm.io/datatypes"
)

// DeployOkeInfra deploys the OKE infrastructure for the control plane.
func DeployOkeInfra(
	cpi *threeport.ControlPlaneInstaller,
	threeportControlPlaneConfig *config.ControlPlane,
	threeportConfig *config.ThreeportConfig,
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra,
	kubeConnectionInfo *kube.KubeConnectionInfo,
	uninstaller *Uninstaller,
) error {
	// Create OKE infrastructure
	kubernetesRuntimeInfraOKE := provider.KubernetesRuntimeInfraOKE{
		RuntimeInstanceName:    provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
		WorkerNodeShape:        "VM.Standard.A1.Flex",
		Version:                "v1.32.1",
		WorkerNodeInitialCount: int32(2),
	}
	*kubernetesRuntimeInfra = &kubernetesRuntimeInfraOKE
	uninstaller.kubernetesRuntimeInfra = &kubernetesRuntimeInfraOKE

	// load OCI config and set overridden values if provided
	// by a command line flag
	if err := kubernetesRuntimeInfraOKE.LoadOCIConfig(
		cpi.Opts.OciRegion,
		cpi.Opts.OciConfigProfile,
		cpi.Opts.OciCompartmentOcid,
	); err != nil {
		return fmt.Errorf("failed to load OCI config: %w", err)
	}

	// update threeport config with eks provider info
	var err error
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
		c.OKEProviderConfig = config.OKEProviderConfig{
			OciRegion:          cpi.Opts.OciRegion,
			OciConfigProfile:   cpi.Opts.OciConfigProfile,
			OciCompartmentOcid: cpi.Opts.OciCompartmentOcid,
		}
	}); err != nil {
		return fmt.Errorf("failed to update threeport config: %w", err)
	}

	if cpi.Opts.ControlPlaneOnly {
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

	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Path to OCI config file
	ociConfigPath := filepath.Join(homeDir, ".oci", "config")

	// Check if config file exists
	if _, err := os.Stat(ociConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("OCI config file not found at %s", ociConfigPath)
	}

	// Load the configuration using the OCI SDK
	configProvider, err := common.ConfigurationProviderFromFile(ociConfigPath, "")
	if err != nil {
		return fmt.Errorf("failed to load OCI configuration: %w", err)
	}

	var privateKey *rsa.PrivateKey
	var userOCID, tenancyOCID, keyFingerprint string
	// get user ocid
	if userOCID, err = configProvider.UserOCID(); err != nil {
		return fmt.Errorf("failed to get user OCID: %w", err)
	}

	// get tenancy ocid
	if tenancyOCID, err = configProvider.TenancyOCID(); err != nil {
		return fmt.Errorf("failed to get tenancy OCID: %w", err)
	}

	// get key fingerprint
	if keyFingerprint, err = configProvider.KeyFingerprint(); err != nil {
		return fmt.Errorf("failed to get key fingerprint: %w", err)
	}

	// get private key
	if privateKey, err = configProvider.PrivateRSAKey(); err != nil {
		return fmt.Errorf("failed to get private key: %w", err)
	}

	ociAccount := v0.OciAccount{
		Name:           util.Ptr("default-account"),
		UserOCID:       &userOCID,
		TenancyOCID:    &tenancyOCID,
		DefaultAccount: util.Ptr(true),
		DefaultRegion:  &kubernetesRuntimeInfraOKE.Region,
		KeyFingerprint: &keyFingerprint,
		PrivateKey:     util.Ptr(privateKeyToPEM(privateKey)),
	}

	_, err = client.CreateOciAccount(
		apiClient,
		threeportAPIEndpoint,
		&ociAccount,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new default OCI account", err)
	}

	// create oci oke k8s runtime definition
	okeRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	ociOkeKubernetesRuntimeDef := v0.OciOkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &okeRuntimeDefName,
		},
		OciAccountID:                  ociAccount.ID,
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
		return uninstaller.cleanOnCreateError("failed to create new OCI OKEkubernetes runtime definition for control plane cluster", err)
	}

	okeRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)

	clusterOCID, err := kubernetesRuntimeInfraOKE.GetClusterOCID(
		okeRuntimeInstName,
		configProvider,
	)
	if err != nil {
		return fmt.Errorf("failed to get cluster OCID: %w", err)
	}

	// get resource inventory from Pulumi state
	var resourceInventory *datatypes.JSON
	if resourceInventory, err = kubernetesRuntimeInfraOKE.GetStackState(); err != nil {
		return fmt.Errorf("failed to get stack state: %w", err)
	}

	ociOkeKubernetesRuntimeInstance := v0.OciOkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &okeRuntimeInstName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
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
		return uninstaller.cleanOnCreateError("failed to create new OCI OKEkubernetes runtime instance for control plane cluster", err)
	}
	return nil
}

// privateKeyToPEM converts an RSA private key to a PEM-encoded string
func privateKeyToPEM(privateKey *rsa.PrivateKey) string {
	// Marshal the private key to PKCS#1 format
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)

	// Create a PEM block
	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)

	// Convert to string
	return string(privateKeyPEM)
}
