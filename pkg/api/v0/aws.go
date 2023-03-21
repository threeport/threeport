//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE
package v0

// AwsAccount is a user account with the AWS service provider.
type AwsAccount struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The region to use for AWS managed services if not specified.
	DefaultRegion *string `json:"DefaultRegion,omitempty" query:"defaultregion" gorm:"not null" validate:"required"`

	// The key ID credentials for the AWS account.
	AccessKeyID *string `json:"AccessKeyID,omitempty" query:"accesskeyid" gorm:"not null" validate:"required"`

	// The secret key credentials for the AWS account.
	SecretAccessKey *string `json:"SecretAccessKey,omitempty" query:"secretaccesskey" gorm:"not null" validate:"required"`
}

type AwsEksClusterDefinition struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	ClusterDefinitionID *uint `json:"ClusterDefinitionID,omitempty" validate:"optional,association"`

	// The AWS account in which the RDS instance will be provisioned.
	AWSAccountID *uint `json:"AWSAccountID,omitempty" query:"awsaccountid" gorm:"not null" validate:"required"`
}

type AwsEksClusterInstance struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	ClusterInstanceID *uint `json:"ClusterInstanceID,omitempty" validate:"optional,association"`
}

// AWSRelationalDatabase is an RDS instance provided by AWS that is used by a
// workload.
type AwsRelationalDatabaseDefinition struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// Unique name for DB definition.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The database engine for the instance.  One of:
	// * mysql
	// * postgres
	Engine *string `json:"Engine,omitempty" query:"engine" gorm:"not null" validate:"required"`

	// The amount of storage to allocate for the database.
	Storage *int32 `json:"Storage,omitempty" query:"storage" gorm:"not null" validate:"required"`

	// The AWS account in which the RDS instance will be provisioned.
	AWSAccountID *uint `json:"AWSAccountID,omitempty" query:"awsaccountid" gorm:"not null" validate:"required"`
}

type AwsRelationalDatabaseInstance struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// Unique name for DB instance.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	Status *string `json:"Status,omitempty" query:"status" gorm:"not null" validate:"required"`
}
