package config

import (
	"encoding/json"

	"github.com/threeport/threeport/internal/tptctl/install"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// ForwardProxyDefinitionConfig contains the attributes needed to manage a
// forward proxy definition.
type ForwardProxyDefinitionConfig struct {
	Name         string `yaml:"Name"`
	UpstreamHost string `yaml:"UpstreamHost"`
	UpstreamPath string `yaml:"UpstreamPath"`
	//WorkloadInstanceName string `yaml:"WorkloadInstanceName"`
}

// Create creates a forward proxy definition in the Threeport API.
func (wsdc *ForwardProxyDefinitionConfig) Create() (*v0.ForwardProxyDefinition, error) {
	//// get workload instance by name
	//workloadInstance, err := client.GetWorkloadInstanceByName(
	//	wsdc.WorkloadInstanceName,
	//	install.GetThreeportAPIEndpoint(), "",
	//)
	//if err != nil {
	//	return nil, err
	//}

	// construct forward proxy definition object
	forwardProxyDefinition := &v0.ForwardProxyDefinition{
		Definition: v0.Definition{
			Name: &wsdc.Name,
		},
		UpstreamHost: &wsdc.UpstreamHost,
		UpstreamPath: &wsdc.UpstreamPath,
		//WorkloadInstanceID: workloadInstance.ID,
	}

	// create workload instance in API
	wsdJSON, err := json.Marshal(&forwardProxyDefinition)
	if err != nil {
		return nil, err
	}
	wsd, err := client.CreateForwardProxyDefinition(wsdJSON, install.GetThreeportAPIEndpoint(), "")
	if err != nil {
		return nil, err
	}

	return wsd, nil
}

// Update updates a forward proxy definition in the Threeport API.
func (wsdc *ForwardProxyDefinitionConfig) Update() (*v0.ForwardProxyDefinition, error) {
	//// get workload instance by name
	//workloadInstance, err := client.GetWorkloadInstanceByName(
	//	wsdc.WorkloadInstanceName,
	//	install.GetThreeportAPIEndpoint(), "",
	//)
	//if err != nil {
	//	return nil, err
	//}

	// construct forward proxy definition object
	forwardProxyDefinition := &v0.ForwardProxyDefinition{
		Definition: v0.Definition{
			Name: &wsdc.Name,
		},
		UpstreamHost: &wsdc.UpstreamHost,
		UpstreamPath: &wsdc.UpstreamPath,
		//WorkloadInstanceID: workloadInstance.ID,
	}

	// get existing forward proxy definition by name to retrieve its ID
	existingWSD, err := client.GetForwardProxyDefinitionByName(
		wsdc.Name,
		install.GetThreeportAPIEndpoint(), "",
	)
	if err != nil {
		return nil, err
	}

	// update workload instance in API
	wsdJSON, err := json.Marshal(&forwardProxyDefinition)
	if err != nil {
		return nil, err
	}
	wsd, err := client.UpdateForwardProxyDefinition(
		*existingWSD.ID,
		wsdJSON,
		install.GetThreeportAPIEndpoint(), "",
	)
	if err != nil {
		return nil, err
	}

	return wsd, nil
}
