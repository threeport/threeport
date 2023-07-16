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
