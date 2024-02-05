//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

import "gorm.io/datatypes"

// SecretDefinition defines a secret that can be deployed to a runtime.
type SecretDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The provider that the secret is stored in.
	Provider *string `json:"Provider,omitempty" query:"provider" gorm:"not null;default'awssm'" validate:"required"`

	// The secret value to be stored in the provider.
	Data *datatypes.JSON `json:"Data,omitempty" query:"data" gorm:"not null" validate:"required" encrypt:"true"`
}

// SecretInstance is an instance of a secret deployed to a runtime.
type SecretInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The SecretDefinition that the secret instance is derived from.
	SecretDefinitionID *uint `json:"SecretDefinitionID,omitempty"`

	// The workload instance that the secret is associated with.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" validate:"optional"`

	// The helm workload instance that the secret is associated with.
	HelmWorkloadInstanceID *uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" validate:"optional"`
}
