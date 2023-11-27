package v0

import (
	"errors"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// DomainNameDefinitionConfig contains the config for a domain name definition.
type DomainNameDefinitionConfig struct {
	DomainNameDefinition DomainNameDefinitionValues `yaml:"DomainNameDefinition"`
}

// DomainNameDefinitionValues contains the attributes needed to manage a domain
// name definition.
type DomainNameDefinitionValues struct {
	Name       string `yaml:"Name"`
	Zone       string `yaml:"Zone"`
	AdminEmail string `yaml:"AdminEmail"`
}

// DomainNameInstanceConfig contains the config for a domain name instance.
type DomainNameInstanceConfig struct {
	DomainNameInstance DomainNameInstanceValues `yaml:"DomainNameInstance"`
}

// DomainNameInstanceValues contains the attributes needed to manage a domain
// name instance.
type DomainNameInstanceValues struct {
	DomainNameDefinition      DomainNameDefinitionValues      `yaml:"DomainNameDefinition"`
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues          `yaml:"WorkloadInstance"`
}

// CreateIfNotExist creates a domain name definition if it does not exist in the Threeport
// API.
func (d *DomainNameDefinitionValues) CreateIfNotExist(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameDefinition, error) {
	// validate required fields
	if d.Name == "" || d.Zone == "" || d.AdminEmail == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, Zone, AdminEmail")
	}

	// check if domain name definition exists
	existingDomainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.Name)
	if err == nil {
		return existingDomainNameDefinition, nil
	}

	// construct domain name definition object
	domainNameDefinition := v0.DomainNameDefinition{
		Definition: v0.Definition{
			Name: &d.Name,
		},
		Domain:     &d.Name,
		Zone:       &d.Zone,
		AdminEmail: &d.AdminEmail,
	}

	// create domain name definition
	createdDomainNameDefinition, err := client.CreateDomainNameDefinition(apiClient, apiEndpoint, &domainNameDefinition)
	if err != nil {
		return nil, err
	}

	return createdDomainNameDefinition, nil
}

// Delete deletes a domain name definition from the Threeport API.
func (d *DomainNameDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameDefinition, error) {
	// check if domain name definition exists
	existingDomainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.Name)
	if err != nil {
		return nil, nil
	}

	deletedDomainNameDefinition, err := client.DeleteDomainNameDefinition(apiClient, apiEndpoint, *existingDomainNameDefinition.ID)
	if err != nil {
		return nil, err
	}

	return deletedDomainNameDefinition, nil
}

// Create creates a domain name instance in the Threeport API.
func (d *DomainNameInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameInstance, error) {
	// validate required fields
	if d.DomainNameDefinition.Name == "" || d.WorkloadInstance.Name == "" ||
		d.KubernetesRuntimeInstance.Name == "" {
		return nil, errors.New("missing required field/s in config - required fields: DomainNameDefinition.Name, WorkloadInstance.Name, KubernetesRuntimeInstance.Name")
	}

	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, d.KubernetesRuntimeInstance.Name)
	if err != nil {
		return nil, err
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, d.WorkloadInstance.Name)
	if err != nil {
		return nil, err
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.DomainNameDefinition.Name)
	if err != nil {
		return nil, err
	}

	// construct domain name instance object
	domainNameInstance := v0.DomainNameInstance{
		Instance: v0.Instance{
			Name: &d.DomainNameDefinition.Name,
		},
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		WorkloadInstanceID:          workloadInstance.ID,
		DomainNameDefinitionID:      domainNameDefinition.ID,
	}

	// create domain name instance
	createdDomainNameInstance, err := client.CreateDomainNameInstance(apiClient, apiEndpoint, &domainNameInstance)
	if err != nil {
		return nil, err
	}

	return createdDomainNameInstance, nil
}

// Delete deletes a domain name instance from the Threeport API.
func (d *DomainNameInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameInstance, error) {
	// check if domain name definition exists
	existingDomainNameInstance, err := client.GetDomainNameInstanceByName(apiClient, apiEndpoint, d.DomainNameDefinition.Name)
	if err != nil {
		return nil, nil
	}

	deletedDomainNameInstance, err := client.DeleteDomainNameInstance(apiClient, apiEndpoint, *existingDomainNameInstance.ID)
	if err != nil {
		return nil, err
	}

	return deletedDomainNameInstance, nil
}
