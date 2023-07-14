package v0

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"path/filepath"

	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// defaultWorkloadInstanceName generates a workload instance name for when the
// workload abstraction is used to create it.
func defaultWorkloadInstanceName(name string) string {
	return fmt.Sprintf("%s-0", name)
}

// WorkloadConfig contains the config for a workload which is an abstraction of
// a workload definition and workload instance.
type WorkloadConfig struct {
	Workload WorkloadValues `yaml:"Workload"`
}

// WorkloadValues contains the attributes needed to manage a workload
// definition and workload instance.
type WorkloadValues struct {
	Name                      string                          `yaml:"Name"`
	YAMLDocument              string                          `yaml:"YAMLDocument"`
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
}

// WorkloadDefinitionConfig contains the config for a workload definition.
type WorkloadDefinitionConfig struct {
	WorkloadDefinition WorkloadDefinitionValues `yaml:"WorkloadDefinition"`
}

// WorkloadDefinitionValues contains the attributes needed to manage a workload
// definition.
type WorkloadDefinitionValues struct {
	Name               string `yaml:"Name"`
	YAMLDocument       string `yaml:"YAMLDocument"`
	WorkloadConfigPath string `yaml:"WorkloadConfigPath"`
}

// WorkloadInstanceConfig contains the config for a workload instance.
type WorkloadInstanceConfig struct {
	WorkloadInstance WorkloadInstanceValues `yaml:"WorkloadInstance"`
}

// WorkloadInstanceValues contains the attributes needed to manage a workload
// instance.
type WorkloadInstanceValues struct {
	Name                      string                          `yaml:"Name"`
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadDefinition        WorkloadDefinitionValues        `yaml:"WorkloadDefinition"`
}

// Create creates a workload definition and instance in the Threeport API.
func (w *WorkloadValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {
	// create the workload definition
	workloadDefinition := WorkloadDefinitionValues{
		Name:               w.Name,
		YAMLDocument:       w.YAMLDocument,
		WorkloadConfigPath: w.WorkloadConfigPath,
	}
	createdWorkloadDefinition, err := workloadDefinition.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workload definition: %w", err)
	}

	// create the workload instance
	workloadInstance := WorkloadInstanceValues{
		Name:                      defaultWorkloadInstanceName(w.Name),
		KubernetesRuntimeInstance: w.KubernetesRuntimeInstance,
		WorkloadDefinition: WorkloadDefinitionValues{
			Name: w.Name,
		},
	}
	createdWorkloadInstance, err := workloadInstance.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workload instance: %w", err)
	}

	return createdWorkloadDefinition, createdWorkloadInstance, nil
}

// Delete deletes a workload definition and instance from the Threeport API.
func (w *WorkloadValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {
	// get workload instance by name
	workloadInstName := defaultWorkloadInstanceName(w.Name)
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, workloadInstName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workload instance with name %s: %w", workloadInstName, err)
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, w.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workload definition with name %s: %w", w.Name, err)
	}

	// ensure the workload definition has no more than one associated instance
	workloadDefInsts, err := client.GetWorkloadInstancesByWorkloadDefinitionID(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get workload instances by workload definition with ID: %d: %w", workloadDefinition.ID, err)
	}
	if len(*workloadDefInsts) > 1 {
		err = errors.New("deletion using the workload abstraction is only permitted when there is a one-to-one workload definition and workload instance relationship")
		return nil, nil, fmt.Errorf("the workload definition has more than one workload instance associated: %w", err)
	}

	// delete workload instance
	deletedWorkloadInstance, err := client.DeleteWorkloadInstance(apiClient, apiEndpoint, *workloadInstance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	// delete workload definition
	deletedWorkloadDefinition, err := client.DeleteWorkloadDefinition(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return deletedWorkloadDefinition, deletedWorkloadInstance, nil
}

// Create creates a workload definition in the Threeport API.
func (wd *WorkloadDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// build the path to the YAML document relative to the user's working
	// directory
	configPath, _ := filepath.Split(wd.WorkloadConfigPath)
	relativeYamlPath := path.Join(configPath, wd.YAMLDocument)

	// load YAML document
	//definitionContent, err := ioutil.ReadFile(wd.YAMLDocument)
	definitionContent, err := ioutil.ReadFile(relativeYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read definition YAMLDocument file %s: %w", wd.YAMLDocument, err)
	}
	stringContent := string(definitionContent)

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: &wd.Name,
		},
		YAMLDocument: &stringContent,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(apiClient, apiEndpoint, &workloadDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload definition in threeport API: %w", err)
	}

	return createdWorkloadDefinition, nil
}

// Delete deletes a workload definition from the Threeport API.
func (wd *WorkloadDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, wd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find workload definition with name %s: %w", wd.Name, err)
	}

	// delete workload definition
	deletedWorkloadDefinition, err := client.DeleteWorkloadDefinition(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return deletedWorkloadDefinition, nil
}

// Create creates a workload instance in the Threeport API.
func (wi *WorkloadInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadInstance, error) {
	// get kubernetes runtime instance by name if provided, otherwise default kubernetes runtime
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance
	if wi.KubernetesRuntimeInstance.Name == "" {
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
			wi.KubernetesRuntimeInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to find kubernetes runtime instance by name %s: %w", wi.KubernetesRuntimeInstance.Name, err)
		}
		kubernetesRuntimeInstance = *kubernetesRuntimeInst
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(
		apiClient,
		apiEndpoint,
		wi.WorkloadDefinition.Name,
	)
	if err != nil {
		return nil, err
	}

	// construct workload instance object
	workloadInstance := v0.WorkloadInstance{
		Instance: v0.Instance{
			Name: &wi.Name,
		},
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		WorkloadDefinitionID:        workloadDefinition.ID,
	}

	// create workload instance
	createdWorkloadInstance, err := client.CreateWorkloadInstance(apiClient, apiEndpoint, &workloadInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload instance in threeport API: %w", err)
	}

	return createdWorkloadInstance, nil
}

// Describe returns important failure events related to a workload instance.
func (wi *WorkloadInstanceValues) Describe(apiClient *http.Client, apiEndpoint string) (*status.WorkloadInstanceStatusDetail, error) {
	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, wi.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find workload instance with name %s: %w", wi.Name, err)
	}

	// get workload instance status
	statusDetail := status.GetWorkloadInstanceStatus(apiClient, apiEndpoint, workloadInstance)
	if statusDetail.Error != nil {
		return nil, fmt.Errorf("failed to get status for workload instance with name %s: %w", wi.Name, statusDetail.Error)
	}

	return statusDetail, nil
}

// Delete deletes a workload instance from the Threeport API.
func (wi *WorkloadInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadInstance, error) {
	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, wi.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find workload instance with name %s: %w", wi.Name, err)
	}

	// delete workload instance
	deletedWorkloadInstance, err := client.DeleteWorkloadInstance(apiClient, apiEndpoint, *workloadInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	return deletedWorkloadInstance, nil
}
