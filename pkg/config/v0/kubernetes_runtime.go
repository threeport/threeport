package v0

import (
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/threeport/threeport/internal/kubernetes-runtime/status"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// KubernetesRuntimeConfig contains the config for a kubernetes runtime which is an abstraction of
// a kubernetes runtime definition and kubernetes runtime instance.
type KubernetesRuntimeConfig struct {
	KubernetesRuntime KubernetesRuntimeValues `yaml:"KubernetesRuntime"`
}

// KubernetesRuntimeValues contains the attributes needed to manage a kubernetes runtime
// definition and kubernetes runtime instance.
type KubernetesRuntimeValues struct {
	Name                     string `yaml:"Name"`
	InfraProvider            string `yaml:"InfraProvider"`
	InfraProviderAccountName string `yaml:"InfraProviderAccountName"`
	HighAvailability         bool   `yaml:"HighAvailability"`
	Location                 string `yaml:"Location"`
	DefaultRuntime           bool   `yaml:"DefaultRuntime"`
	ThreeportAgentImage      string `yaml:"ThreeportAgentImage"`
}

// KubernetesRuntimeDefinitionConfig contains the config for a kubernetes runtime definition.
type KubernetesRuntimeDefinitionConfig struct {
	KubernetesRuntimeDefinition KubernetesRuntimeDefinitionValues `yaml:"KubernetesRuntimeDefinition"`
}

// KubernetesRuntimeDefinitionValues contains the attributes needed to manage a kubernetes runtime
// definition.
type KubernetesRuntimeDefinitionValues struct {
	Name                     string `yaml:"Name"`
	InfraProvider            string `yaml:"InfraProvider"`
	InfraProviderAccountName string `yaml:"InfraProviderAccountName"`
	HighAvailability         bool   `yaml:"HighAvailability"`
}

// KubernetesRuntimeInstanceConfig contains the config for a kubernetes runtime instance.
type KubernetesRuntimeInstanceConfig struct {
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
}

// KubernetesRuntimeInstanceValues contains the attributes needed to manage a kubernetes runtime
// instance.
type KubernetesRuntimeInstanceValues struct {
	Name                        string                            `yaml:"Name"`
	ThreeportControlPlaneHost   bool                              `yaml:"ThreeportControlPlaneHost"`
	DefaultRuntime              bool                              `yaml:"DefaultRuntime"`
	Location                    string                            `yaml:"Location"`
	ThreeportAgentImage         string                            `yaml:"ThreeportAgentImage"`
	KubernetesRuntimeDefinition KubernetesRuntimeDefinitionValues `yaml:"KubernetesRuntimeDefinition"`
}

// Create creates a kubernetes runtime definition and instance in the Threeport API.
func (kr *KubernetesRuntimeValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeDefinition, *v0.KubernetesRuntimeInstance, error) {
	// create the kubernetes runtime definition
	kubernetesRuntimeDefinition := KubernetesRuntimeDefinitionValues{
		Name:                     kr.Name,
		InfraProvider:            kr.InfraProvider,
		InfraProviderAccountName: kr.InfraProviderAccountName,
		HighAvailability:         kr.HighAvailability,
	}
	createdKubernetesRuntimeDefinition, err := kubernetesRuntimeDefinition.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes runtime definition: %w", err)
	}

	// create the kubernetes runtime instance
	kubernetesRuntimeInstance := KubernetesRuntimeInstanceValues{
		Name:                      kr.Name,
		Location:                  kr.Location,
		ThreeportControlPlaneHost: false,
		DefaultRuntime:            kr.DefaultRuntime,
		ThreeportAgentImage:       kr.ThreeportAgentImage,
		KubernetesRuntimeDefinition: KubernetesRuntimeDefinitionValues{
			Name: kr.Name,
		},
	}
	createdKubernetesRuntimeInstance, err := kubernetesRuntimeInstance.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes runtime instance: %w", err)
	}

	return createdKubernetesRuntimeDefinition, createdKubernetesRuntimeInstance, nil
}

// Delete deletes a kubernetes runtime definition and instance from the Threeport API.
func (kr *KubernetesRuntimeValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeDefinition, *v0.KubernetesRuntimeInstance, error) {
	// get kubernetes runtime instance by name
	kubernetesRuntimeInstName := kr.Name
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, kubernetesRuntimeInstName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find kubernetes runtime instance with name %s: %w", kubernetesRuntimeInstName, err)
	}

	// get kubernetes runtime definition by name
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, kr.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find kubernetes runtime definition with name %s: %w", kr.Name, err)
	}

	// ensure the kubernetes runtime definition has no more than one associated instance
	kubernetesRuntimeDefInsts, err := client.GetKubernetesRuntimeInstancesByKubernetesRuntimeDefinitionID(apiClient, apiEndpoint, *kubernetesRuntimeDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kubernetes runtime instances by kubernetes runtime definition with ID: %d: %w", kubernetesRuntimeDefinition.ID, err)
	}
	if len(*kubernetesRuntimeDefInsts) > 1 {
		err = errors.New("deletion using the kubernetes runtime abstraction is only permitted when there is a one-to-one kubernetes runtime definition and kubernetes runtime instance relationship")
		return nil, nil, fmt.Errorf("the kubernetes runtime definition has more than one kubernetes runtime instance associated: %w", err)
	}

	// delete kubernetes runtime instance
	deletedKubernetesRuntimeInstance, err := client.DeleteKubernetesRuntimeInstance(apiClient, apiEndpoint, *kubernetesRuntimeInstance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete kubernetes runtime instance from threeport API: %w", err)
	}

	// wait for kubernetes runtime instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, kr.Name); err == nil {
			return errors.New("kubernetes runtime instance not deleted")
		}
		return nil
	})

	// delete kubernetes runtime definition
	deletedKubernetesRuntimeDefinition, err := client.DeleteKubernetesRuntimeDefinition(apiClient, apiEndpoint, *kubernetesRuntimeDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete kubernetes runtime definition from threeport API: %w", err)
	}

	return deletedKubernetesRuntimeDefinition, deletedKubernetesRuntimeInstance, nil
}

func (krd *KubernetesRuntimeDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeDefinition, error) {
	// validate required fields
	if krd.Name == "" || krd.InfraProvider == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, InfraProvider")
	}

	// validate name length
	if utf8.RuneCountInString(krd.Name) > provider.RuntimeNameMaxLength {
		return nil, errors.New(fmt.Sprintf(
			"kubernetes runtime definition name too long - cannot exceed %d characters",
			provider.RuntimeNameMaxLength,
		))
	}

	// construct kubernetes runtime definition object
	kubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &krd.Name,
		},
		InfraProvider:            &krd.InfraProvider,
		HighAvailability:         &krd.HighAvailability,
		InfraProviderAccountName: &krd.InfraProviderAccountName,
	}

	// create kubernetes runtime definition
	createdKubernetesRuntimeDefinition, err := client.CreateKubernetesRuntimeDefinition(apiClient, apiEndpoint, &kubernetesRuntimeDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes runtime definition in threeport API: %w", err)
	}

	return createdKubernetesRuntimeDefinition, nil
}

// Describe returns details related to a kubernetes runtime definition.
func (k *KubernetesRuntimeDefinitionValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.KubernetesRuntimeDefinitionStatusDetail, error) {
	// get kubernetes runtime definition by name
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByName(
		apiClient,
		apiEndpoint,
		k.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find kubernetes runtime instance with name %s: %w", k.Name, err)
	}

	// get kubernetes runtime definition status
	statusDetail, err := status.GetKubernetesRuntimeDefinitionStatus(
		apiClient,
		apiEndpoint,
		kubernetesRuntimeDefinition,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for kubernetes runtime instance with name %s: %w", k.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes a kubernetes runtime definition from the Threeport API.
func (krd *KubernetesRuntimeDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeDefinition, error) {
	// get kubernetes runtime definition by name
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, krd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find kubernetes definition with name %s: %w", krd.Name, err)
	}

	// delete kubernetes definition
	deletedKubernetesRuntimeDefinition, err := client.DeleteKubernetesRuntimeDefinition(apiClient, apiEndpoint, *kubernetesRuntimeDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete kubernetes definition from threeport API: %w", err)
	}

	return deletedKubernetesRuntimeDefinition, nil
}

func (kri *KubernetesRuntimeInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeInstance, error) {
	// validate required fields
	if kri.Name == "" || kri.Location == "" || kri.KubernetesRuntimeDefinition.Name == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, Location, KubernetesRuntimeDefinition.Name")
	}

	// validate name length
	if utf8.RuneCountInString(kri.Name) > provider.RuntimeNameMaxLength {
		return nil, errors.New(fmt.Sprintf(
			"kubernetes runtime instance name too long - cannot exceed %d characters",
			provider.RuntimeNameMaxLength,
		))
	}

	// get kubernetes runtime definition by name
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, kri.KubernetesRuntimeDefinition.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find kubernetes definition with name %s: %w", kri.KubernetesRuntimeDefinition.Name, err)
	}

	// construct kubernetes runtime instance object
	kubernetesRuntimeInstance := v0.KubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &kri.Name,
		},
		KubernetesRuntimeDefinitionID: kubernetesRuntimeDefinition.ID,
		DefaultRuntime:                &kri.DefaultRuntime,
		Location:                      &kri.Location,
		ThreeportAgentImage:           &kri.ThreeportAgentImage,
	}

	// create kubernetes runtime instance
	createdKubernetesRuntimeInstance, err := client.CreateKubernetesRuntimeInstance(apiClient, apiEndpoint, &kubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes runtime instance in threeport API: %w", err)
	}

	return createdKubernetesRuntimeInstance, nil
}

// Describe returns details related to a kubernetes runtime instance.
func (k *KubernetesRuntimeInstanceValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.KubernetesRuntimeInstanceStatusDetail, error) {
	// get kubernetes runtime instance by name
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		k.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find kubernetes runtime instance with name %s: %w", k.Name, err)
	}

	// get kubernetes runtime instance status
	statusDetail, err := status.GetKubernetesRuntimeInstanceStatus(
		apiClient,
		apiEndpoint,
		kubernetesRuntimeInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for kubernetes runtime instance with name %s: %w", k.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes a kubernetes instance from the Threeport API.
func (kri *KubernetesRuntimeInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeInstance, error) {
	// get kubernetes instance by name
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, kri.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find kubernetes instance with name %s: %w", kri.Name, err)
	}

	// delete kubernetes instance
	deletedKubernetesRuntimeInstance, err := client.DeleteKubernetesRuntimeInstance(apiClient, apiEndpoint, *kubernetesRuntimeInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete kubernetes instance from threeport API: %w", err)
	}

	return deletedKubernetesRuntimeInstance, nil
}

// getKubernetesRuntimeInstanceForConfig takes the config values for a
// kubernetes runtime instance and returns the API object for a kubernetes
// runtime instance.
func setKubernetesRuntimeInstanceForConfig(
	runtimeInstanceVals *KubernetesRuntimeInstanceValues,
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.KubernetesRuntimeInstance, error) {
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance
	if runtimeInstanceVals == nil {
		// get default kubernetes runtime instance
		kubernetesRuntimeInst, err := client.GetDefaultKubernetesRuntimeInstance(apiClient, apiEndpoint)
		if err != nil {
			return nil, fmt.Errorf("kubernetes runtime instance not provided and failed to find default kubernetes runtime instance: %w", err)
		}
		kubernetesRuntimeInstance = *kubernetesRuntimeInst
	} else {
		kubernetesRuntimeInst, err := client.GetKubernetesRuntimeInstanceByName(
			apiClient,
			apiEndpoint,
			runtimeInstanceVals.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to find kubernetes runtime instance by name %s: %w", runtimeInstanceVals.Name, err)
		}
		kubernetesRuntimeInstance = *kubernetesRuntimeInst
	}

	return &kubernetesRuntimeInstance, nil
}
