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
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ConfigureControlPlaneWithOkeConfig configures the control plane with the OKE config.
func ConfigureControlPlaneWithOkeConfig(
	cpi *threeport.ControlPlaneInstaller,
	uninstaller *Uninstaller,
	apiClient *http.Client,
	threeportAPIEndpoint *string,
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
	fmt.Printf("Loading OCI config from: %s\n", ociConfigPath)

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
		TenancyID:      &tenancyOCID,
		DefaultAccount: util.Ptr(true),
		DefaultRegion:  &kubernetesRuntimeInfraOKE.Region,
		KeyFingerprint: &keyFingerprint,
		PrivateKey:     util.Ptr(privateKeyToPEM(privateKey)),
	}

	_, err = client.CreateOciAccount(
		apiClient,
		*threeportAPIEndpoint,
		&ociAccount,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new default OCI account", err)
	}

	// create aws eks k8s runtime definition
	eksRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	// kubernetesRuntimeInfraOKE := (*kubernetesRuntimeInfra).(*provider.KubernetesRuntimeInfraOKE)
	ociOkeKubernetesRuntimeDef := v0.OciOkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &eksRuntimeDefName,
		},
		OciAccountID:                  ociAccount.ID,
		WorkerNodeShape:               &kubernetesRuntimeInfraOKE.WorkerNodeShape,
		WorkerNodeInitialCount:        util.Ptr(int32(kubernetesRuntimeInfraOKE.WorkerNodeInitialCount)),
		WorkerNodeMinCount:            util.Ptr(int32(kubernetesRuntimeInfraOKE.WorkerNodeMinCount)),
		WorkerNodeMaxCount:            util.Ptr(int32(kubernetesRuntimeInfraOKE.WorkerNodeMaxCount)),
		AvailabilityDomainCount:       util.Ptr(int32(kubernetesRuntimeInfraOKE.AvailabilityDomainCount)),
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
	}
	createdociOkeKubernetesRuntimeDef, err := client.CreateOciOkeKubernetesRuntimeDefinition(
		apiClient,
		*threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeDef,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new OCI OKEkubernetes runtime definition for control plane cluster", err)
	}

	eksRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	reconciled := true

	clusterOCID, err := kubernetesRuntimeInfraOKE.GetClusterOCID(configProvider)
	if err != nil {
		return fmt.Errorf("failed to get cluster OCID: %w", err)
	}

	ociOkeKubernetesRuntimeInstance := v0.OciOkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &eksRuntimeInstName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: &reconciled,
		},
		OciOkeKubernetesRuntimeDefinitionID: createdociOkeKubernetesRuntimeDef.ID,
		KubernetesRuntimeInstanceID:         kubernetesRuntimeInstResult.ID,
		ClusterOCID:                         &clusterOCID,
	}
	_, err = client.CreateOciOkeKubernetesRuntimeInstance(
		apiClient,
		*threeportAPIEndpoint,
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
