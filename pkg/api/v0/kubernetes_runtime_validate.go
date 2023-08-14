// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

import (
	"fmt"

	"gorm.io/gorm"
)

// KubernetesRuntimeInfraProvider indicates which infrastructure provider is being
// used to run the kubernetes cluster for the threeport control plane.
type KubernetesRuntimeInfraProvider string

const (
	KubernetesRuntimeInfraProviderKind = "kind"
	KubernetesRuntimeInfraProviderEKS  = "eks"
)

// SupportedInfraProviders returns all supported infra providers.
func SupportedInfraProviders() []KubernetesRuntimeInfraProvider {
	return []KubernetesRuntimeInfraProvider{
		KubernetesRuntimeInfraProviderKind,
		KubernetesRuntimeInfraProviderEKS,
	}
}

// KubernetesRuntimeDefinitionValidationErr is an error that accepts a custom
// message when validation errors occur for the KubernetesRuntimeDefinition
// object.
type KubernetesRuntimeDefinitionValidationErr struct {
	Message string
}

// Error returns the custom message generated during validation.
func (e *KubernetesRuntimeDefinitionValidationErr) Error() string {
	return e.Message
}

// BeforeCreate validates a KubernetesRuntimeDefinition object before creating
// in the database.
func (k *KubernetesRuntimeDefinition) BeforeCreate(tx *gorm.DB) error {
	// validate infra provider is one of the support types
	infraProviders := SupportedInfraProviders()
	providerValid := false
	for _, provider := range infraProviders {
		if *k.InfraProvider == string(provider) {
			providerValid = true
			break
		}
	}
	if !providerValid {
		msg := fmt.Sprintf(
			"%s provider is not valid, valid providers: %s",
			*k.InfraProvider,
			infraProviders,
		)
		return &KubernetesRuntimeDefinitionValidationErr{msg}
	}

	return nil
}

// BeforeDelete validates a delete request on a kubernetes runtime instance
// deletion to ensure deletion is possible.
func (k *KubernetesRuntimeInstance) BeforeDelete(tx *gorm.DB) error {
	// validate that no workloads exist or that ForceDelete is true
	var workloadInstances []WorkloadInstance
	if result := tx.Where(&WorkloadInstance{KubernetesRuntimeInstanceID: k.ID}).Find(&workloadInstances); result.Error != nil {
		msg := fmt.Sprintf("failed to query workload instances for kubernetes runtime instance %s", *k.Name)
		return &KubernetesRuntimeDefinitionValidationErr{msg}
	}

	if len(workloadInstances) > 0 {
		msg := fmt.Sprintf(
			"kubernetes runtime instance %s cannot be deleted until workloads are removed",
			*k.Name,
		)
		return &KubernetesRuntimeDefinitionValidationErr{msg}
	}

	return nil
}
