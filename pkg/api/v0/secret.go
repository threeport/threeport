package v0

import "gorm.io/datatypes"

// SecretDefinition defines a secret that can be deployed to a runtime.
type SecretDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The AWS account ID, if the provider is AWS.
	AwsProviderID *uint `json:"AwsProviderID,omitempty" query:"awsproviderid" validate:"optional" relationship:"requires"`

	// The secret value to be stored in the provider.
	Data *datatypes.JSON `json:"Data,omitempty" query:"data" validate:"required" persist:"false"`

	// The associated secret instances that are deployed from this definition.
	SecretInstances []*SecretInstance `json:"SecretInstances,omitempty" validate:"optional,association"`
}

// SecretInstance is an instance of a secret deployed to a runtime.
type SecretInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the secret is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required" relationship:"requires"`

	// The SecretDefinition that the secret instance is derived from.
	SecretDefinitionID *uint `json:"SecretDefinitionID,omitempty" query:"secretdefinitionid" gorm:"not null" validate:"required" relationship:"requires"`

	// The kubernetes workload instance that the secret is associated with.
	KubernetesWorkloadInstanceID *uint `json:"KubernetesWorkloadInstanceID,omitempty" query:"kubernetesworkloadinstanceid" validate:"optional" relationship:"requires"`

	// The helm workload instance that the secret is associated with.
	HelmWorkloadInstanceID *uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" validate:"optional" relationship:"requires"`
}
