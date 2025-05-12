package v0

import (
	"net/http"

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

	ociAccount := v0.OciAccount{
		Name:           util.Ptr("default-account"),
		DefaultAccount: util.Ptr(true),
		DefaultRegion:  util.Ptr(kubernetesRuntimeInfraOKE.Region),
	}

	_, err := client.CreateOciAccount(
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
		// AwsAccountID:                  createdOciAccount.ID,
		// DefaultNodeGroupInstanceType:  &kubernetesRuntimeInfraOKE.DefaultNodeGroupInstanceType,
		// DefaultNodeGroupInitialSize:   util.Ptr(int(kubernetesRuntimeInfraOKE.DefaultNodeGroupInitialNodes)),
		// DefaultNodeGroupMinimumSize:   util.Ptr(int(kubernetesRuntimeInfraOKE.DefaultNodeGroupMinNodes)),
		// DefaultNodeGroupMaximumSize:   util.Ptr(int(kubernetesRuntimeInfraOKE.DefaultNodeGroupMaOKEdes)),
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
	}
	createdociOkeKubernetesRuntimeDef, err := client.CreateOciOkeKubernetesRuntimeDefinition(
		apiClient,
		*threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeDef,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new AWS EKS kubernetes runtime definition for control plane cluster", err)
	}

	eksRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	reconciled := true
	ociOkeKubernetesRuntimeInstance := v0.OciOkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &eksRuntimeInstName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: &reconciled,
		},
		OciOkeKubernetesRuntimeDefinitionID: createdociOkeKubernetesRuntimeDef.ID,
		KubernetesRuntimeInstanceID:         kubernetesRuntimeInstResult.ID,
		// ResourceInventory:                   &dbInventory,
	}
	_, err = client.CreateOciOkeKubernetesRuntimeInstance(
		apiClient,
		*threeportAPIEndpoint,
		&ociOkeKubernetesRuntimeInstance,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new AWS EKS kubernetes runtime instance for control plane cluster", err)
	}
	return nil
}
