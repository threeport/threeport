//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-sdk codegen controller --filename $GOFILE
package v0

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl:config-path
// TerraformDefinition is the configuration for terraform-defined resources.
type TerraformDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Path to the directory containing terraform configs with '.tf' file
	// extension.
	ConfigDir *string `json:"ConfigDir,omitempty" gorm:"not null" validate:"required"`

	// The associated terraform instances that are deployed from this definition.
	TerraformInstances []*TerraformInstance `json:"TerraformInstances,omitempty" validate:"optional,association"`
}

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl:config-path
// TerraformInstance is the deployed instances of terraform resources defined in
// the associated definition with the variables values.  The output from
// terraform is stored here along with the terraform state document.
type TerraformInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The AWS account in which the resources will be provisioned.
	AwsAccountID *uint `json:"AwsAccountID,omitempty" query:"awsaccountid" gorm:"not null" validate:"required"`

	// The .tfvars document that contains runtime parameters for an instance of
	// some terraform resources.
	VarsDocument *string `json:"VarsDocument,omitempty" validate:"optional" encrypt:"true"`

	// The terraform state json object that stores the inventory of
	// infrastructure being managed by terraform.  The terraform state is stored
	// in JSON format but is a string type to support encryption.
	StateDocument *string `json:"StateDocument,omitempty" validate:"optional" encrypt:"true"`

	// The outputs defined in the terraform config that are collected from
	// Terraform.  The terraform outputs are stored in JSON format but is a
	// string typt to support encryption.
	Outputs *string `json:"Outputs,omitempty" validate:"optional" encrypt:"true"`

	// The definition used to configure the terraform resources.
	TerraformDefinitionID *uint `json:"TerraformDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"not null" validate:"required"`
}
