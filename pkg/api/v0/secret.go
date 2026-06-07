package v0

import "gorm.io/datatypes"

// SecretDefinition defines a secret that can be deployed to a runtime.
type SecretDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The AWS account ID, if the provider is AWS.
	AwsProviderID *uint `json:",omitempty" validate:"optional" relationship:"requires"`

	// The secret value to be stored in the provider.
	Data *datatypes.JSON `json:",omitempty" validate:"required" persist:"false"`

	// The associated secret instances that are deployed from this definition.
	SecretInstances []*SecretInstance `json:",omitempty" validate:"optional,association"`
}

// SecretInstance is an instance of a secret deployed to a runtime.
type SecretInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the secret is deployed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The SecretDefinition that the secret instance is derived from.
	SecretDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes workload instance that the secret is associated with.
	KubernetesWorkloadInstanceID *uint `json:",omitempty" validate:"optional" relationship:"requires"`

	// The helm workload instance that the secret is associated with.
	HelmWorkloadInstanceID *uint `json:",omitempty" validate:"optional" relationship:"requires"`
}
