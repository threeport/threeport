package v0

import (
	"errors"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GatewayDefinitionConfig contains the config for a gateway definition.
type GatewayDefinitionConfig struct {
	GatewayDefinition GatewayDefinitionValues `yaml:"GatewayDefinition"`
}

// GatewayDefinitionValues contains the attributes needed to manage a gateway.
type GatewayDefinitionValues struct {
	Name                 string                     `yaml:"Name"`
	TCPPort              int                        `yaml:"TCPPort"`
	TLSEnabled           bool                       `yaml:"TLSEnabled"`
	Path                 string                     `yaml:"Path"`
	ServiceName          string                     `yaml:"ServiceName"`
	DomainNameDefinition DomainNameDefinitionValues `yaml:"DomainNameDefinition"`
}

// GatewayInstanceConfig contains the config for a gateway instance.
type GatewayInstanceConfig struct {
	GatewayInstance GatewayInstanceValues `yaml:"GatewayInstance"`
}

// GatewayInstanceValues contains the attributes needed to manage a gateway
// instance.
type GatewayInstanceValues struct {
	GatewayDefinition         GatewayDefinitionValues         `yaml:"GatewayDefinition"`
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues          `yaml:"WorkloadInstance"`
}

// Validate validates the gateway definition values.
func (g *GatewayDefinitionValues) Validate() error {
	multiError := util.MultiError{}

	if g.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	if g.TCPPort == 0 {
		multiError.AppendError(errors.New("missing required field in config: TCPPort"))
	}

	if g.DomainNameDefinition.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: DomainNameDefinition.Name"))
	}

	if len(multiError.Errors) > 0 {
		return multiError.Error()
	}

	return nil
}

// Create creates a gateway definition.
func (g *GatewayDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.GatewayDefinition, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, g.DomainNameDefinition.Name)
	if err != nil {
		return nil, err
	}

	// construct gateway definition object
	gatewayDefinition := v0.GatewayDefinition{
		Definition: v0.Definition{
			Name: &g.Name,
		},
		TCPPort:                &g.TCPPort,
		TLSEnabled:             &g.TLSEnabled,
		Path:                   &g.Path,
		ServiceName:            &g.ServiceName,
		DomainNameDefinitionID: domainNameDefinition.ID,
	}

	// create gateway definition
	createdGatewayDefinition, err := client.CreateGatewayDefinition(apiClient, apiEndpoint, &gatewayDefinition)
	if err != nil {
		return nil, err
	}

	return createdGatewayDefinition, nil
}

// Delete deletes a gateway definition.
func (g *GatewayDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) error {
	// get domain name definition
	gatewayDefinition, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, g.Name)
	if err != nil {
		return err
	}

	_, err = client.DeleteGatewayDefinition(apiClient, apiEndpoint, *gatewayDefinition.ID)
	if err != nil {
		return err
	}

	return nil
}

// Validate validates the gateway definition values.
func (g *GatewayInstanceValues) Validate() error {
	multiError := util.MultiError{}

	if g.GatewayDefinition.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: GatewayDefinition.Name"))
	}

	if g.KubernetesRuntimeInstance.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: KubernetesRuntimeInstance.Name"))
	}

	if g.WorkloadInstance.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: WorkloadInstance.Name"))
	}

	if len(multiError.Errors) > 0 {
		return multiError.Error()
	}

	return nil
}

// Create creates a gateway instance.
func (g *GatewayInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.GatewayInstance, error) {
	// validate required fields
	if err := g.Validate(); err != nil {
		return nil, err
	}

	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, g.KubernetesRuntimeInstance.Name)
	if err != nil {
		return nil, err
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, g.WorkloadInstance.Name)
	if err != nil {
		return nil, err
	}

	// get gateway definition
	gatewayDefinition, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, g.GatewayDefinition.Name)
	if err != nil {
		return nil, err
	}

	// construct gateway instance object
	gatewayInstance := v0.GatewayInstance{
		Instance: v0.Instance{
			Name: &g.GatewayDefinition.Name,
		},
		GatewayDefinitionID:         gatewayDefinition.ID,
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		WorkloadInstanceID:          workloadInstance.ID,
	}

	// create gateway instance
	createdGatewayInstance, err := client.CreateGatewayInstance(apiClient, apiEndpoint, &gatewayInstance)
	if err != nil {
		return nil, err
	}

	return createdGatewayInstance, nil
}

// Delete deletes a gateway instance.
func (g *GatewayInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) error {
	// get gateway instance by name
	gatewayInstance, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, g.GatewayDefinition.Name)
	if err != nil {
		return err
	}

	// delete gateway instance
	_, err = client.DeleteGatewayInstance(apiClient, apiEndpoint, *gatewayInstance.ID)
	if err != nil {
		return err
	}

	// wait for gateway instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, g.GatewayDefinition.Name); err == nil {
			return errors.New("gateway instance not deleted")
		}
		return nil
	})

	return nil
}
