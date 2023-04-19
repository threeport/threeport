package v0

import (
	"fmt"
	"io/ioutil"

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

// WorkloadValues contains the attributes needed to manager a workload
// definition and workload instance.
type WorkloadValues struct {
	Name            string                `yaml:"Name"`
	JSONDocument    string                `yaml:"JSONDocument"`
	ClusterInstance ClusterInstanceValues `yaml:"ClusterInstance"`
}

// WorkloadDefinitionConfig contains the config for a workload definition.
type WorkloadDefinitionConfig struct {
	WorkloadDefinition WorkloadDefinitionValues `yaml:"WorkloadDefinition"`
}

// WorkloadDefinitionValues contains the attributes needed to manage a workload
// definition.
type WorkloadDefinitionValues struct {
	Name         string `yaml:"Name"`
	JSONDocument string `yaml:"JSONDocument"`
	UserID       uint   `yaml:"UserID"`
}

// WorkloadInstanceConfig contains the config for a workload instance.
type WorkloadInstanceConfig struct {
	WorkloadInstance WorkloadInstanceValues `yaml:"WorkloadInstance"`
}

// WorkloadInstanceValues contains the attributes needed to manage a workload
// instance.
type WorkloadInstanceValues struct {
	Name               string                   `yaml:"Name"`
	ClusterInstance    ClusterInstanceValues    `yaml:"ClusterInstance"`
	WorkloadDefinition WorkloadDefinitionValues `yaml:"WorkloadDefinition"`
}

// Create creates a workload definition and instance in the Threeport API.
func (w *WorkloadValues) Create(apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {
	// create the workload definition
	workloadDefinition := WorkloadDefinitionValues{
		Name:         w.Name,
		JSONDocument: w.JSONDocument,
	}
	createdWorkloadDefinition, err := workloadDefinition.Create(apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workload definition: %w", err)
	}

	// create the workload instance
	workloadInstance := WorkloadInstanceValues{
		Name:            defaultWorkloadInstanceName(w.Name),
		ClusterInstance: w.ClusterInstance,
		WorkloadDefinition: WorkloadDefinitionValues{
			Name: w.Name,
		},
	}
	createdWorkloadInstance, err := workloadInstance.Create(apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workload instance: %w", err)
	}

	return createdWorkloadDefinition, createdWorkloadInstance, nil
}

// Delete deletes a workload definition and instance from the Threeport API.
func (w *WorkloadValues) Delete(apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {
	// get workload instance by name
	workloadInstName := defaultWorkloadInstanceName(w.Name)
	workloadInstance, err := client.GetWorkloadInstanceByName(workloadInstName, apiEndpoint, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workload instance with name %s: %w", workloadInstName, err)
	}

	// delete workload instance
	deletedWorkloadInstance, err := client.DeleteWorkloadInstance(*workloadInstance.ID, apiEndpoint, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(w.Name, apiEndpoint, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workload definition with name %s: %w", w.Name, err)
	}

	// delete workload definition
	deletedWorkloadDefinition, err := client.DeleteWorkloadDefinition(*workloadDefinition.ID, apiEndpoint, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return deletedWorkloadDefinition, deletedWorkloadInstance, nil
}

// Create creates a workload definition in the Threeport API.
func (wd *WorkloadDefinitionValues) Create(apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// load YAML document
	definitionContent, err := ioutil.ReadFile(wd.JSONDocument)
	if err != nil {
		return nil, fmt.Errorf("failed to read definition JSONDocument file %s: %w", wd.JSONDocument, err)
	}
	stringContent := string(definitionContent)

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name:   &wd.Name,
			UserID: &wd.UserID,
		},
		JSONDocument: &stringContent,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(&workloadDefinition, apiEndpoint, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create workload definition in threeport API: %w", err)
	}

	return createdWorkloadDefinition, nil
}

// Delete deletes a workload definition from the Threeport API.
func (wd *WorkloadDefinitionValues) Delete(apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(wd.Name, apiEndpoint, "")
	if err != nil {
		return nil, fmt.Errorf("failed to find workload definition with name %s: %w", wd.Name, err)
	}

	// delete workload definition
	deletedWorkloadDefinition, err := client.DeleteWorkloadDefinition(*workloadDefinition.ID, apiEndpoint, "")
	if err != nil {
		return nil, fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return deletedWorkloadDefinition, nil
}

// Create creates a workload instance in the Threeport API.
func (wi *WorkloadInstanceValues) Create(apiEndpoint string) (*v0.WorkloadInstance, error) {
	// get cluster instance by name if provided, otherwise default cluster
	var clusterInstance v0.ClusterInstance
	if wi.ClusterInstance.Name == "" {
		// get default cluster instance
		clusterInst, err := client.GetDefaultClusterInstance(apiEndpoint, "")
		if err != nil {
			return nil, fmt.Errorf("cluster instance not provided and failed to find default cluster instance: %w", err)
		}
		clusterInstance = *clusterInst
	} else {
		clusterInst, err := client.GetClusterInstanceByName(
			wi.ClusterInstance.Name,
			apiEndpoint,
			"",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to cluster instance by name %s: %w", wi.ClusterInstance.Name, err)
		}
		clusterInstance = *clusterInst
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(
		wi.WorkloadDefinition.Name,
		apiEndpoint,
		"",
	)
	if err != nil {
		return nil, err
	}

	// construct workload instance object
	workloadInstance := v0.WorkloadInstance{
		Instance: v0.Instance{
			Name: &wi.Name,
		},
		ClusterInstanceID:    clusterInstance.ID,
		WorkloadDefinitionID: workloadDefinition.ID,
	}

	// create workload instance
	createdWorkloadInstance, err := client.CreateWorkloadInstance(&workloadInstance, apiEndpoint, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create workload instance in threeport API: %w", err)
	}

	return createdWorkloadInstance, nil
}

// Delete deletes a workload instance from the Threeport API.
func (wd *WorkloadInstanceValues) Delete(apiEndpoint string) (*v0.WorkloadInstance, error) {
	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(wd.Name, apiEndpoint, "")
	if err != nil {
		return nil, fmt.Errorf("failed to find workload instance with name %s: %w", wd.Name, err)
	}

	// delete workload instance
	deletedWorkloadInstance, err := client.DeleteWorkloadInstance(*workloadInstance.ID, apiEndpoint, "")
	if err != nil {
		return nil, fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	return deletedWorkloadInstance, nil
}
