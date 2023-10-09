package v0

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// ControlPlaneConfig contains the config for a control plane which is an abstraction of
// a control plane definition and control plane instance.
type ControlPlaneConfig struct {
	ControlPlane ControlPlaneValues `yaml:"ControlPlane"`
}

// ControlPlaneValues contains the attributes needed to manage a control plane
// definition and control plane instance.
type ControlPlaneValues struct {
	Name                      string                           `yaml:"Name"`
	Namespace                 string                           `yaml:"Namespace"`
	AuthEnabled               bool                             `yaml:"AuthEnabled"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	CustomComponentInfo       []*v0.ControlPlaneComponent      `yaml:"CustomComponentInfo"`
}

// ControlPlaneDefinitionConfig contains the config for a control plane definition.
type ControlPlaneDefinitionConfig struct {
	ControlPlaneDefinition ControlPlaneDefinitionValues `yaml:"ControlPlaneDefinition"`
}

// ControlPlaneDefinitionValues contains the attributes needed to manage a control plane
// definition.
type ControlPlaneDefinitionValues struct {
	Name        string `yaml:"Name"`
	AuthEnabled bool   `yaml:"AuthEnabled"`
}

// ControlPlaneInstanceConfig contains the config for a control plane instance.
type ControlPlaneInstanceConfig struct {
	ControlPlaneInstance ControlPlaneInstanceValues `yaml:"ControlPlaneInstance"`
}

// ControlPlaneInstanceValues contains the attributes needed to manage a control plane
// instance.
type ControlPlaneInstanceValues struct {
	Name                      string                           `yaml:"Name"`
	Namespace                 string                           `yaml:"Namespace"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	ControlPlaneDefinition    ControlPlaneDefinitionValues     `yaml:"ControlPlaneDefinition"`
	CustomInstallInfo         []*v0.ControlPlaneComponent      `yaml:"CustomInstallInfo"`
}

// Create creates a control plane definition and instance in the Threeport API.
func (c *ControlPlaneValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.ControlPlaneDefinition, *v0.ControlPlaneInstance, error) {
	// create the control plane definition
	controlPlaneDefinition := ControlPlaneDefinitionValues{
		Name:        c.Name,
		AuthEnabled: c.AuthEnabled,
	}
	createdControlPlaneDefinition, err := controlPlaneDefinition.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create control plane definition: %w", err)
	}

	// create the control plane instance
	controlPlaneInstance := ControlPlaneInstanceValues{
		Name:                      c.Name,
		Namespace:                 c.Namespace,
		KubernetesRuntimeInstance: c.KubernetesRuntimeInstance,
		CustomInstallInfo:         c.CustomComponentInfo,
		ControlPlaneDefinition: ControlPlaneDefinitionValues{
			Name: c.Name,
		},
	}
	createdControlPlaneInstance, err := controlPlaneInstance.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create control plane instance: %w", err)
	}

	return createdControlPlaneDefinition, createdControlPlaneInstance, nil
}

// Delete deletes a control plane definition and a control plane instance
// from the Threeport API.
func (c *ControlPlaneValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.ControlPlaneDefinition, *v0.ControlPlaneInstance, error) {
	// get control plane instance by name
	controlPlaneInstance, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, c.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find control plane instance with name %s: %w", c.Name, err)
	}

	// get control plane definition by name
	controlPlaneDefinition, err := client.GetControlPlaneDefinitionByName(apiClient, apiEndpoint, c.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find control plane definition with name %s: %w", c.Name, err)
	}

	// ensure the control plane definition has no more than one associated instance
	controlPlanDefInsts, err := client.GetControlPlaneInstancesByControlPlaneDefinitionID(apiClient, apiEndpoint, *controlPlaneDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get control plane def instances by control plane definition with ID: %d: %w", controlPlaneDefinition.ID, err)
	}
	if len(*controlPlanDefInsts) > 1 {
		err = errors.New("deletion using the controlplane abstraction is only permitted when there is a one-to-one control plane definition and control plane instance relationship")
		return nil, nil, fmt.Errorf("the control plane definition has more than one control plane instance associated: %w", err)
	}

	// delete control plane instance
	deletedControlPlaneInstance, err := client.DeleteControlPlaneInstance(apiClient, apiEndpoint, *controlPlaneInstance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete control plane instance from threeport API: %w", err)
	}

	// wait for control plane instance to be reconciled
	deletedCheckAttempts := 0
	deletedCheckAttemptsMax := 30
	deletedCheckDurationSeconds := 1
	controlPlaneInstanceDeleted := false
	for deletedCheckAttempts < deletedCheckAttemptsMax {
		_, err := client.GetControlPlaneInstanceByID(apiClient, apiEndpoint, *controlPlaneInstance.ID)
		if err != nil {
			if errors.Is(err, client.ErrorObjectNotFound) {
				controlPlaneInstanceDeleted = true
				break
			} else {
				return nil, nil, fmt.Errorf("failed to get control plane instance from API when checking deletion: %w", err)
			}
		}
		// no error means control plane instance was found - hasn't yet been deleted
		deletedCheckAttempts += 1
		time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
	}
	if !controlPlaneInstanceDeleted {
		return nil, nil, errors.New(fmt.Sprintf(
			"control plane instance not deleted after %d seconds",
			deletedCheckAttemptsMax*deletedCheckDurationSeconds,
		))
	}

	// delete control plane definition
	deletedControlPlaneDefinition, err := client.DeleteControlPlaneDefinition(apiClient, apiEndpoint, *controlPlaneDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete control plane definition from threeport API: %w", err)
	}

	return deletedControlPlaneDefinition, deletedControlPlaneInstance, nil
}

// Create creates a control plane definition in the Threeport API.
func (cd *ControlPlaneDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.ControlPlaneDefinition, error) {
	// validate required fields
	if cd.Name == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name")
	}

	// construct control plane definition object
	controlPlaneDefinition := v0.ControlPlaneDefinition{
		Definition: v0.Definition{
			Name: &cd.Name,
		},
		AuthEnabled: &cd.AuthEnabled,
	}

	// create control plane definition
	createdControlPlaneDefinition, err := client.CreateControlPlaneDefinition(apiClient, apiEndpoint, &controlPlaneDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create control plane definition in threeport API: %w", err)
	}

	return createdControlPlaneDefinition, nil
}

// Delete deletes a control plane definition from the Threeport API.
func (cd *ControlPlaneDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.ControlPlaneDefinition, error) {
	// get control plane definition by name
	controlPlaneDefinition, err := client.GetControlPlaneDefinitionByName(apiClient, apiEndpoint, cd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find control plane definition with name %s: %w", cd.Name, err)
	}

	// delete control plane definition
	deletedControlPlaneDefinition, err := client.DeleteControlPlaneDefinition(apiClient, apiEndpoint, *controlPlaneDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete control plane definition from threeport API: %w", err)
	}

	return deletedControlPlaneDefinition, nil
}

// Create creates a control plane instance in the Threeport API.
func (ci *ControlPlaneInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.ControlPlaneInstance, error) {
	// validate required fields
	if ci.Name == "" || ci.Namespace == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, ControlPlaneInstance.Namespace")
	}

	// get kubernetes runtime instance by name if provided, otherwise default kubernetes runtime
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance
	if ci.KubernetesRuntimeInstance == nil {
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
			ci.KubernetesRuntimeInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to find kubernetes runtime instance by name %s: %w", ci.KubernetesRuntimeInstance.Name, err)
		}
		kubernetesRuntimeInstance = *kubernetesRuntimeInst
	}

	// get control plane definition by name
	controlPlaneDefinition, err := client.GetControlPlaneDefinitionByName(
		apiClient,
		apiEndpoint,
		ci.ControlPlaneDefinition.Name,
	)
	if err != nil {
		return nil, err
	}

	// construct control plane instance object
	controlPlaneInstance := v0.ControlPlaneInstance{
		Instance: v0.Instance{
			Name: &ci.Name,
		},
		Namespace:                   &ci.Namespace,
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		CustomComponentInfo:         ci.CustomInstallInfo,
		ControlPlaneDefinitionID:    controlPlaneDefinition.ID,
	}

	// create control plane instance
	createdControlPlaneInstance, err := client.CreateControlPlaneInstance(apiClient, apiEndpoint, &controlPlaneInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create control plane instance in threeport API: %w", err)
	}

	return createdControlPlaneInstance, nil

}

// Describe returns important failure events related to a control plane instance.
func (wi *ControlPlaneInstanceValues) Describe(apiClient *http.Client, apiEndpoint string) (*status.WorkloadInstanceStatusDetail, error) {

	return nil, nil
}

// Delete deletes a control plane instance from the Threeport API.
func (ci *ControlPlaneInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.ControlPlaneInstance, error) {
	// get control plane instance by name
	controlPlaneInstance, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, ci.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find control plane instance with name %s: %w", ci.Name, err)
	}

	// delete control plane instance
	deletedControlPlaneInstance, err := client.DeleteControlPlaneInstance(apiClient, apiEndpoint, *controlPlaneInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete control plane instance from threeport API: %w", err)
	}

	return deletedControlPlaneInstance, nil
}
