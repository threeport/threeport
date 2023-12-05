package v0

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/iancoleman/strcase"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// DomainNameDefinitionConfig contains the config for a domain name definition.
type DomainNameDefinitionConfig struct {
	DomainNameDefinition DomainNameDefinitionValues `yaml:"DomainNameDefinition"`
}

// DomainNameDefinitionValues contains the attributes needed to manage a domain
// name definition.
type DomainNameDefinitionValues struct {
	Domain     string `yaml:"Domain"`
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
	if err := d.Validate(); err != nil {
		return nil, err
	}

	// check if domain name definition exists
	existingDomainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.Domain)
	if err == nil {
		return existingDomainNameDefinition, nil
	}

	// construct domain name definition object
	domainNameDefinition := v0.DomainNameDefinition{
		Definition: v0.Definition{
			Name: &d.Domain,
		},
		Domain:     &d.Domain,
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

// Validate validates the domain name definition values.
func (d *DomainNameDefinitionValues) Validate() error {

	multiError := util.MultiError{}

	if d.Domain == "" {
		multiError.AppendError(errors.New("missing required field in config: Domain"))
	}

	if d.Zone == "" {
		multiError.AppendError(errors.New("missing required field in config: Zone"))
	}

	if d.AdminEmail == "" {
		multiError.AppendError(errors.New("missing required field in config: AdminEmail"))
	}

	if len(multiError.Errors) > 0 {
		return multiError.Error()
	}

	return nil
}

// Delete deletes a domain name definition from the Threeport API.
func (d *DomainNameDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameDefinition, error) {
	// check if domain name definition exists
	existingDomainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.Domain)
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
	if err := d.Validate(); err != nil {
		return nil, err
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
	domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.DomainNameDefinition.Domain)
	if err != nil {
		return nil, err
	}

	// construct domain name instance object
	domainNameInstance := v0.DomainNameInstance{
		Instance: v0.Instance{
			Name: util.StringPtr(d.getDomainNameInstanceName()),
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
	existingDomainNameInstance, err := client.GetDomainNameInstanceByName(apiClient, apiEndpoint, d.getDomainNameInstanceName())
	if err != nil {
		return nil, nil
	}

	deletedDomainNameInstance, err := client.DeleteDomainNameInstance(apiClient, apiEndpoint, *existingDomainNameInstance.ID)
	if err != nil {
		return nil, err
	}

	return deletedDomainNameInstance, nil
}

// getDomainNameInstanceName returns the name of the domain name instance.
func (d *DomainNameInstanceValues) getDomainNameInstanceName() string {
	return fmt.Sprintf("%s-%s", d.WorkloadInstance.Name, strcase.ToKebab(d.DomainNameDefinition.Domain))
}

// Validate validates the domain name instance values.
func (d *DomainNameInstanceValues) Validate() error {
	multiError := util.MultiError{}

	if d.DomainNameDefinition.Domain == "" {
		multiError.AppendError(errors.New("missing required field in config: DomainNameDefinition.Name"))
	}

	if d.WorkloadInstance.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: WorklaodInstance.Name"))
	}

	if d.KubernetesRuntimeInstance.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: KubernetesRuntimeInstance.Name"))
	}

	if len(multiError.Errors) > 0 {
		return multiError.Error()
	}

	return nil
}
