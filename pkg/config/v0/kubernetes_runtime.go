package v0

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

//// KubernetesRuntimeInstanceValues contains the attributes needed to manage a cluster
//// instance.
//type KubernetesRuntimeInstanceValues struct {
//	Name string `yaml:"Name"`
//}

///////////////////////////////////////////////////////////////////////////////

//// KubernetesRuntimeDefinitionConfig contains the config for a workload definition.
//type KubernetesRuntimeDefinitionConfig struct {
//	KubernetesRuntimeDefinition KubernetesRuntimeDefinitionValues `yaml:"KubernetesRuntimeDefinition"`
//}
//
//// KubernetesRuntimeDefinitionValues contains the attributes needed to manage a workload
//// definition.
//type KubernetesRuntimeDefinitionValues struct {
//	Name         string `yaml:"Name"`
//	YAMLDocument string `yaml:"YAMLDocument"`
//}

// KubernetesRuntimeInstanceConfig contains the config for a workload instance.
type KubernetesRuntimeInstanceConfig struct {
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
}

// KubernetesRuntimeInstanceValues contains the attributes needed to manage a workload
// instance.
type KubernetesRuntimeInstanceValues struct {
	Name                          string `yaml:"Name"`
	KubernetesRuntimeDefinitionID uint   `yaml:"KubernetesRuntimeDefinitionID"`
	DefaultRuntime                bool   `yaml:"DefaultRuntime"`
}

func (kri *KubernetesRuntimeInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.KubernetesRuntimeInstance, error) {
	// construct workload instance object
	kubernetesRuntimeInstance := v0.KubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &kri.Name,
		},
		KubernetesRuntimeDefinitionID: &kri.KubernetesRuntimeDefinitionID,
		DefaultRuntime:                &kri.DefaultRuntime,
	}

	// create workload instance
	createdKubernetesRuntimeInstance, err := client.CreateKubernetesRuntimeInstance(apiClient, apiEndpoint, &kubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload instance in threeport API: %w", err)
	}

	return createdKubernetesRuntimeInstance, nil
}
