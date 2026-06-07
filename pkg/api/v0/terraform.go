package v0

// TerraformDefinition is the configuration for terraform-defined resources.
type TerraformDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Path to the directory containing terraform configs with '.tf' file
	// extension.
	ConfigDir *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The associated terraform instances that are deployed from this definition.
	TerraformInstances []*TerraformInstance `json:",omitempty" validate:"optional,association"`
}

// TerraformInstance is the deployed instances of terraform resources defined in
// the associated definition with the variables values.  The output from
// terraform is stored here along with the terraform state document.
type TerraformInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The AWS provider in which the resources will be provisioned.
	AwsProviderID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The .tfvars document that contains runtime parameters for an instance of
	// some terraform resources.
	VarsDocument *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// The terraform state json object that stores the inventory of
	// infrastructure being managed by terraform.  The terraform state is stored
	// in JSON format but is a string type to support encryption.
	StateDocument *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// The outputs defined in the terraform config that are collected from
	// Terraform.  The terraform outputs are stored in JSON format but is a
	// string typt to support encryption.
	Outputs *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// The definition used to configure the terraform resources.
	TerraformDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`
}
