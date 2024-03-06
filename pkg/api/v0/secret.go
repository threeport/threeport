//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-sdk codegen controller --filename $GOFILE
package v0

import "gorm.io/datatypes"

// +threeport-sdk:reconciler
// +threeport-sdk:add-custom-middleware
// SecretDefinition defines a secret that can be deployed to a runtime.
type SecretDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The AWS account ID, if the provider is AWS.
	AwsAccountID *uint `json:"AwsAccountID,omitempty" query:"awsaccountid" validate:"optional"`

	// The secret value to be stored in the provider.
	Data *datatypes.JSON `json:"Data,omitempty" query:"data" validate:"required" persist:"false"`
}

// +threeport-sdk:reconciler
// SecretInstance is an instance of a secret deployed to a runtime.
type SecretInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the helm workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The SecretDefinition that the secret instance is derived from.
	SecretDefinitionID *uint `json:"SecretDefinitionID,omitempty" query:"secretdefinitionid" gorm:"not null" validate:"required"`

	// The workload instance that the secret is associated with.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" validate:"optional"`

	// The helm workload instance that the secret is associated with.
	HelmWorkloadInstanceID *uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" validate:"optional"`
}
