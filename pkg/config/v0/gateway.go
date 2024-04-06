package v0

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/gateway/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GatewayConfig contains the config for a gateway.
type GatewayConfig struct {
	Gateway GatewayValues `yaml:"Gateway"`
}

// GatewayValues contains the attributes needed to manage a gateway
// definition and gateway instance.
type GatewayValues struct {
	Name                      string                           `yaml:"Name"`
	HttpPorts                 []GatewayHttpPortValues          `yaml:"HttpPorts"`
	TcpPorts                  []GatewayTcpPortValues           `yaml:"TcpPorts"`
	ServiceName               string                           `yaml:"ServiceName"`
	SubDomain                 string                           `yaml:"SubDomain"`
	DomainNameDefinition      DomainNameDefinitionValues       `yaml:"DomainNameDefinition"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues           `yaml:"WorkloadInstance"`
}

// GatewayDefinitionConfig contains the config for a gateway definition.
type GatewayDefinitionConfig struct {
	GatewayDefinition GatewayDefinitionValues `yaml:"GatewayDefinition"`
}

// GatewayDefinitionValues contains the attributes needed to manage a gateway.
type GatewayDefinitionValues struct {
	Name                 string                     `yaml:"Name"`
	HttpPorts            []GatewayHttpPortValues    `yaml:"HttpPorts"`
	TcpPorts             []GatewayTcpPortValues     `yaml:"TcpPorts"`
	ServiceName          string                     `yaml:"ServiceName"`
	SubDomain            string                     `yaml:"SubDomain"`
	DomainNameDefinition DomainNameDefinitionValues `yaml:"DomainNameDefinition"`
}

// GatewayHttpPortValues contains the attributes needed to manage a gateway
// http port.
type GatewayHttpPortValues struct {
	Port          int    `yaml:"Port"`
	Path          string `yaml:"Path"`
	TLSEnabled    bool   `yaml:"TLSEnabled"`
	HTTPSRedirect bool   `yaml:"HTTPSRedirect"`
}

// GatewayTcpPortValues contains the attributes needed to manage a gateway
// tcp port.
type GatewayTcpPortValues struct {
	Port       int  `yaml:"Port"`
	TLSEnabled bool `yaml:"TLSEnabled"`
}

// GatewayInstanceConfig contains the config for a gateway instance.
type GatewayInstanceConfig struct {
	GatewayInstance GatewayInstanceValues `yaml:"GatewayInstance"`
}

// GatewayInstanceValues contains the attributes needed to manage a gateway
// instance.
type GatewayInstanceValues struct {
	Name                      string                           `yaml:"Name"`
	GatewayDefinition         GatewayDefinitionValues          `yaml:"GatewayDefinition"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues           `yaml:"WorkloadInstance"`
}

// DomainNameConfig contains the config for a domain name.
type DomainNameConfig struct {
	DomainName DomainNameValues `yaml:"DomainName"`
}

// DomainNameValues contains the attributes needed to manage a domain name
// definition and domain name instance.
type DomainNameValues struct {
	Name                      string                           `yaml:"Name"`
	Domain                    string                           `yaml:"Domain"`
	Zone                      string                           `yaml:"Zone"`
	AdminEmail                string                           `yaml:"AdminEmail"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues           `yaml:"WorkloadInstance"`
}

// DomainNameDefinitionConfig contains the config for a domain name definition.
type DomainNameDefinitionConfig struct {
	DomainNameDefinition DomainNameDefinitionValues `yaml:"DomainNameDefinition"`
}

// DomainNameDefinitionValues contains the attributes needed to manage a domain
// name definition.
type DomainNameDefinitionValues struct {
	Name       string `yaml:"Name"`
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
	Name                      string                           `yaml:"Name"`
	DomainNameDefinition      DomainNameDefinitionValues       `yaml:"DomainNameDefinition"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues           `yaml:"WorkloadInstance"`
}

// Create creates a gateway definition and instance in the Threeport API.
func (g *GatewayValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.GatewayDefinition, *v0.GatewayInstance, error) {

	// get operations
	operations, createdGatewayDefinition, createdGatewayInstance := g.GetOperations(apiClient, apiEndpoint)

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, fmt.Errorf("failed to execute create operations for gateway with name %s: %w", g.Name, err)
	}

	return createdGatewayDefinition, createdGatewayInstance, nil
}

// Delete deletes a gateway definition and instance from the Threeport API.
func (g *GatewayValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.GatewayDefinition, *v0.GatewayInstance, error) {

	// get operation
	operations, _, _ := g.GetOperations(apiClient, apiEndpoint)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute delete operations for gateway defined instance with name %s: %w",
			g.Name,
			err,
		)
	}

	return nil, nil, nil
}

// Validate validates gateway definition values.
func (g *GatewayDefinitionValues) Validate() error {
	multiError := util.MultiError{}

	// ensure name is set
	if g.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	// ensure http ports or tcp ports are set
	if g.HttpPorts == nil && g.TcpPorts == nil {
		multiError.AppendError(errors.New("missing required field in config: Must provide one of []HttpPorts or []TcpPorts"))
	}

	return multiError.Error()
}

// Validate validates gateway http port values.
func (g *GatewayHttpPortValues) Validate() error {

	multiError := util.MultiError{}

	// set path to default if not provided,
	// this is necessary because we can't tell if the user
	// didn't set the Path field or if they intended to set it to
	// a blank string
	if g.Path == "" {
		g.Path = "/"
	}

	// ensure TLS isn't enabled while HTTPSRedirect is also enabled
	if g.TLSEnabled && g.HTTPSRedirect {
		multiError.AppendError(errors.New("cannot set both TLSEnabled and HTTPSRedirect to true"))

	}

	// ensure port is set
	if g.Port == 0 {
		multiError.AppendError(errors.New("missing required field in config: Port"))
	}

	return multiError.Error()
}

// Describe returns details related to a gateway definition.
func (wd *GatewayDefinitionValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.GatewayDefinitionStatusDetail, error) {
	// get gateway definition by name
	gatewayDefinition, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, wd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway definition with name %s: %w", wd.Name, err)
	}

	// get gateway definition status
	statusDetail, err := status.GetGatewayDefinitionStatus(
		apiClient,
		apiEndpoint,
		*gatewayDefinition.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for gateway definition with name %s: %w", wd.Name, err)
	}

	return statusDetail, nil
}

// Create creates a gateway definition.
func (g *GatewayDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.GatewayDefinition, error) {
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate values for gateway definition with name %s: %w", g.Name, err)
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, g.DomainNameDefinition.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain name definition with name %s: %w", g.DomainNameDefinition.Name, err)
	}

	// construct list of http ports
	var httpPorts []*v0.GatewayHttpPort
	if g.HttpPorts != nil {
		for _, httpPort := range g.HttpPorts {

			// create copy of pointer
			currentHttpPort := httpPort

			// validate port config
			if err := currentHttpPort.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate values for http port %d: %w", currentHttpPort.Port, err)
			}

			httpPorts = append(httpPorts,
				&v0.GatewayHttpPort{
					Port:          &currentHttpPort.Port,
					Path:          &currentHttpPort.Path,
					TLSEnabled:    &currentHttpPort.TLSEnabled,
					HTTPSRedirect: &currentHttpPort.HTTPSRedirect,
				})
		}
	}

	// construct list of tcp ports
	var tcpPorts []*v0.GatewayTcpPort
	if g.TcpPorts != nil {
		for _, tcpPort := range g.TcpPorts {
			currentTcpPort := tcpPort
			tcpPorts = append(tcpPorts,
				&v0.GatewayTcpPort{
					Port:       &currentTcpPort.Port,
					TLSEnabled: &currentTcpPort.TLSEnabled,
				})
		}
	}

	// construct gateway definition object
	gatewayDefinition := v0.GatewayDefinition{
		Definition: v0.Definition{
			Name: &g.Name,
		},
		HttpPorts:              httpPorts,
		TcpPorts:               tcpPorts,
		SubDomain:              &g.SubDomain,
		ServiceName:            &g.ServiceName,
		DomainNameDefinitionID: domainNameDefinition.ID,
	}

	// create gateway definition
	createdGatewayDefinition, err := client.CreateGatewayDefinition(apiClient, apiEndpoint, &gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway definition with name %s: %w", g.Name, err)
	}

	return createdGatewayDefinition, nil
}

// Delete deletes a gateway definition.
func (g *GatewayDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.GatewayDefinition, error) {
	// get domain name definition
	gatewayDefinition, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, g.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway definition with name %s: %w", g.Name, err)
	}

	deletedGatewayDefinition, err := client.DeleteGatewayDefinition(apiClient, apiEndpoint, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete gateway definition with name %s: %w", g.Name, err)
	}

	return deletedGatewayDefinition, nil
}

// Validate validates the gateway definition values.
func (g *GatewayInstanceValues) Validate() error {
	multiError := util.MultiError{}

	if g.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	if g.GatewayDefinition.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: GatewayDefinition.Name"))
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
		return nil, fmt.Errorf("failed to validate values for gateay instance with name %s: %w", g.Name, err)
	}

	// get kubernetes runtime instance API object
	kubernetesRuntimeInstance, err := setKubernetesRuntimeInstanceForConfig(
		g.KubernetesRuntimeInstance,
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to set kubernetes runtime instance with name %s: %w",
			g.KubernetesRuntimeInstance.Name,
			err,
		)
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, g.WorkloadInstance.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload instance with name %s: %w", g.WorkloadInstance.Name, err)
	}

	// get gateway definition
	gatewayDefinition, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, g.GatewayDefinition.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway definition with name %s: %w", g.GatewayDefinition.Name, err)
	}

	// construct gateway instance object
	gatewayInstance := v0.GatewayInstance{
		Instance: v0.Instance{
			Name: &g.Name,
		},
		GatewayDefinitionID:         gatewayDefinition.ID,
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		WorkloadInstanceID:          workloadInstance.ID,
	}

	// create gateway instance
	createdGatewayInstance, err := client.CreateGatewayInstance(apiClient, apiEndpoint, &gatewayInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway instance with name %s: %w", g.Name, err)
	}

	return createdGatewayInstance, nil
}

// Describe returns details related to a gateway instance.
func (k *GatewayInstanceValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.GatewayInstanceStatusDetail, error) {
	// get gateway instance by name
	gatewayInstance, err := client.GetGatewayInstanceByName(
		apiClient,
		apiEndpoint,
		k.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway instance with name %s: %w", k.Name, err)
	}

	// get gateway instance status
	statusDetail, err := status.GetGatewayInstanceStatus(
		apiClient,
		apiEndpoint,
		gatewayInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for gateway instance with name %s: %w", k.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes a gateway instance.
func (g *GatewayInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.GatewayInstance, error) {
	// get gateway instance by name
	gatewayInstance, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, g.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway instance with name %s: %w", g.Name, err)
	}

	// delete gateway instance
	deletedGatewayInstance, err := client.DeleteGatewayInstance(apiClient, apiEndpoint, *gatewayInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete gateway instance with name %s: %w", g.Name, err)
	}

	// wait for gateway instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, g.GatewayDefinition.Name); err == nil {
			return errors.New("gateway instance not deleted")
		}
		return nil
	})

	return deletedGatewayInstance, nil
}

// GetOperations returns a slice of operations used to create or delete a
// gateway.
func (g *GatewayValues) GetOperations(
	apiClient *http.Client,
	apiEndpoint string,
) (*util.Operations, *v0.GatewayDefinition, *v0.GatewayInstance) {

	var err error
	var createdGatewayInstance v0.GatewayInstance
	var createdGatewayDefinition v0.GatewayDefinition

	operations := util.Operations{}

	// add gateway definition operation
	gatewayDefinitionValues := GatewayDefinitionValues{
		Name:                 g.Name,
		HttpPorts:            g.HttpPorts,
		TcpPorts:             g.TcpPorts,
		ServiceName:          g.ServiceName,
		SubDomain:            g.SubDomain,
		DomainNameDefinition: g.DomainNameDefinition,
	}
	operations.AppendOperation(util.Operation{
		Name: "gateway definition",
		Create: func() error {
			gatewayDefinition, err := gatewayDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create gateway definition with name %s: %w", g.Name, err)
			}
			createdGatewayDefinition = *gatewayDefinition
			return nil
		},
		Delete: func() error {
			_, err = gatewayDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete gateway definition with name %s: %w", g.Name, err)
			}
			return nil
		},
	})

	// add gateway instance operation
	gatewayInstanceValues := GatewayInstanceValues{
		Name:                      g.Name,
		KubernetesRuntimeInstance: g.KubernetesRuntimeInstance,
		WorkloadInstance:          g.WorkloadInstance,
		GatewayDefinition: GatewayDefinitionValues{
			Name: g.Name,
		},
	}
	operations.AppendOperation(util.Operation{
		Name: "gateway instance",
		Create: func() error {
			gatewayInstance, err := gatewayInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create gateway instance with name %s: %w", g.Name, err)
			}
			createdGatewayInstance = *gatewayInstance
			return nil
		},
		Delete: func() error {
			_, err = gatewayInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete gateway instance with name %s: %w", g.Name, err)
			}
			return nil
		},
	})

	return &operations, &createdGatewayDefinition, &createdGatewayInstance
}

// Create creates a domain name definition and instance in the Threeport API.
func (n *DomainNameValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.DomainNameDefinition, *v0.DomainNameInstance, error) {

	// get operations
	operations, createdDomainNameDefinition, createdDomainNameInstance := n.GetOperations(apiClient, apiEndpoint)

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute create operations for domain name defined instance with name %s: %w",
			n.Name,
			err,
		)
	}

	return createdDomainNameDefinition, createdDomainNameInstance, nil
}

// Delete deletes a domain name definition and instance from the Threeport API.
func (n *DomainNameValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.DomainNameDefinition, *v0.DomainNameInstance, error) {

	// get operation
	operations, _, _ := n.GetOperations(apiClient, apiEndpoint)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute delete operations for domain name defined instance with name %s: %w",
			n.Name,
			err,
		)
	}

	return nil, nil, nil
}

// Create creates a domain name definition if it does not exist in the Threeport
// API.
func (d *DomainNameDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameDefinition, error) {
	// validate required fields
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate values for domain name definition with name %s: %w", d.Name, err)
	}

	// check if domain name definition exists
	existingDomainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.Domain)
	if err == nil {
		return existingDomainNameDefinition, nil
	}

	// construct domain name definition object
	domainNameDefinition := v0.DomainNameDefinition{
		Definition: v0.Definition{
			Name: &d.Name,
		},
		Domain:     &d.Domain,
		Zone:       &d.Zone,
		AdminEmail: &d.AdminEmail,
	}

	// create domain name definition
	createdDomainNameDefinition, err := client.CreateDomainNameDefinition(apiClient, apiEndpoint, &domainNameDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create domain name definition with name %s: %w", d.Name, err)
	}

	return createdDomainNameDefinition, nil
}

// Validate validates the domain name definition values.
func (d *DomainNameDefinitionValues) Validate() error {

	multiError := util.MultiError{}

	if d.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

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

// Describe returns details related to a domain name definition.
func (wd *DomainNameDefinitionValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.DomainNameDefinitionStatusDetail, error) {
	// get domain name definition by name
	domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, wd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find domain name definition with name %s: %w", wd.Name, err)
	}

	// get domain name definition status
	statusDetail, err := status.GetDomainNameDefinitionStatus(
		apiClient,
		apiEndpoint,
		*domainNameDefinition.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for domain name definition with name %s: %w", wd.Name, err)
	}

	return statusDetail, nil
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
		return nil, fmt.Errorf("failed to delete domain name definition with name %s: %w", d.Name, err)
	}

	return deletedDomainNameDefinition, nil
}

// Create creates a domain name instance in the Threeport API.
func (d *DomainNameInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.DomainNameInstance, error) {
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf(
			"failed to validate values for domain name instance %s: %w",
			d.getDomainNameInstanceName(),
			err,
		)
	}

	// get kubernetes runtime instance API object
	kubernetesRuntimeInstance, err := setKubernetesRuntimeInstanceForConfig(
		d.KubernetesRuntimeInstance,
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set kubernetes runtime instance: %w", err)
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, d.WorkloadInstance.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload instance with name %s: %w", d.WorkloadInstance.Name, err)
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, d.DomainNameDefinition.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain name definition with name %s: %w", d.DomainNameDefinition.Name, err)
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
		return nil, fmt.Errorf(
			"failed to create domain name instance with name%s: %w",
			d.getDomainNameInstanceName(),
			err,
		)
	}

	return createdDomainNameInstance, nil
}

// Describe returns details related to a domain name instance.
func (k *DomainNameInstanceValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.DomainNameInstanceStatusDetail, error) {
	// get domain name instance by name
	domainNameInstance, err := client.GetDomainNameInstanceByName(
		apiClient,
		apiEndpoint,
		k.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find domain name instance with name %s: %w", k.Name, err)
	}

	// get domain name instance status
	statusDetail, err := status.GetDomainNameInstanceStatus(
		apiClient,
		apiEndpoint,
		domainNameInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for domain name instance with name %s: %w", k.Name, err)
	}

	return statusDetail, nil
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
		return nil, fmt.Errorf("failed to delete domain name instance %s: %w", d.getDomainNameInstanceName(), err)
	}

	// wait for domain name instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetDomainNameInstanceByName(apiClient, apiEndpoint, d.Name); err == nil {
			return errors.New("domain name instance not deleted")
		}
		return nil
	})

	return deletedDomainNameInstance, nil
}

// getDomainNameInstanceName returns the name of the domain name instance.
func (d *DomainNameInstanceValues) getDomainNameInstanceName() string {
	return fmt.Sprintf("%s-%s", d.WorkloadInstance.Name, strcase.ToKebab(d.DomainNameDefinition.Name))
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

	if len(multiError.Errors) > 0 {
		return multiError.Error()
	}

	return nil
}

// GetOperations returns a slice of operations used to create or delete a
// domain name.
func (n *DomainNameValues) GetOperations(
	apiClient *http.Client,
	apiEndpoint string,
) (*util.Operations, *v0.DomainNameDefinition, *v0.DomainNameInstance) {

	var err error
	var createdDomainNameInstance v0.DomainNameInstance
	var createdDomainNameDefinition v0.DomainNameDefinition

	operations := util.Operations{}

	// add domain name definition operation
	domainNameDefinitionValues := DomainNameDefinitionValues{
		Name:       n.Name,
		Domain:     n.Name,
		Zone:       n.Zone,
		AdminEmail: n.AdminEmail,
	}
	operations.AppendOperation(util.Operation{
		Name: "domain name definition",
		Create: func() error {
			domainNameDefinition, err := domainNameDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create domain name definition %s: %w", n.Name, err)
			}
			createdDomainNameDefinition = *domainNameDefinition
			return nil
		},
		Delete: func() error {
			_, err = domainNameDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete domain name definition %s: %w", n.Name, err)
			}
			return nil
		},
	})

	// add domain name instance operation
	domainNameInstanceValues := DomainNameInstanceValues{
		Name:                      n.Name,
		KubernetesRuntimeInstance: n.KubernetesRuntimeInstance,
		WorkloadInstance:          n.WorkloadInstance,
		DomainNameDefinition: DomainNameDefinitionValues{
			Name: n.Name,
		},
	}
	operations.AppendOperation(util.Operation{
		Name: "domain name instance",
		Create: func() error {
			domainNameInstance, err := domainNameInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf(
					"failed to create domain name instance %s: %w",
					domainNameInstanceValues.Name,
					err,
				)
			}
			createdDomainNameInstance = *domainNameInstance
			return nil
		},
		Delete: func() error {
			_, err = domainNameInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf(
					"failed to delete domain name instance %s: %w",
					domainNameInstanceValues.Name,
					err,
				)
			}
			return nil
		},
	})

	return &operations, &createdDomainNameDefinition, &createdDomainNameInstance
}
