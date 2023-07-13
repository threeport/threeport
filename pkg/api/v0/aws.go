//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate ../../../bin/threeport-codegen controller --filename $GOFILE
package v0

import "gorm.io/datatypes"

// AwsAccount is a user account with the AWS service provider.
type AwsAccount struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of an AWS account.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The region to use for AWS managed services if not specified.
	DefaultRegion *string `json:"DefaultRegion,omitempty" query:"defaultregion" gorm:"not null" validate:"required"`

	// The account ID for the AWS account.
	AccountID *string `json:"AccountID,omitempty" query:"accountid" gorm:"not null" validate:"required"`

	// The key ID credentials for the AWS account.
	AccessKeyID *string `json:"AccessKeyID,omitempty" query:"accesskeyid" gorm:"not null" validate:"required"`

	// The secret key credentials for the AWS account.
	SecretAccessKey *string `json:"SecretAccessKey,omitempty" query:"secretaccesskey" gorm:"not null" validate:"required"`

	// The cluster instances deployed in this AWS account.
	AwsEksClusterInstances []*AwsEksClusterInstance `json:"AwsEksClusterInstances,omitempty" validate:"optional,association"`
}

// AwsEksKubernetesRuntimeDefinition provides the configuration for EKS cluster instances.
type AwsEksKubernetesRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The AWS region in which EKS clusters will be provisioned.  Note: changes to
	// this value will not alter the derived instances which is an immutable
	// characteristic on instances.  It will only affect new instances derived
	// from this definition.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The number of zones the cluster should span for availability.
	ZoneCount *int `json:"ZoneCount,omitempty" query:"zonecount" validate:"optional"`

	// The AWS instance type for the default initial node group.
	DefaultNodeGroupInstanceType *string `json:"DefaultNodeGroupInstanceType,omitempty" query:"defaultnodegroupinstancetype" validate:"optional"`

	// The number of nodes in the default initial node group.
	DefaultNodeGroupInitialSize *int `json:"DefaultNodeGroupInitialSize,omitempty" query:"defaultnodegroupinitialsize" validate:"optional"`

	// The minimum number of nodes the default initial node group should have.
	DefaultNodeGroupMinimumSize *int `json:"DefaultNodeGroupMinimumSize,omitempty" query:"defaultnodegroupminimumsize" validate:"optional"`

	// The maximum number of nodes the default initial node group should have.
	DefaultNodeGroupMaximumSize *int `json:"DefaultNodeGroupMaximumSize,omitempty" query:"defaultnodegroupmaximumsize" validate:"optional"`

	// The AWS EKS cluster instances derived from this definition.
	AwsEksClusterInstances []*AwsEksClusterInstance `json:"AwsEksClusterInstances,omitempty" validate:"optional,association"`

	// The cluster definition for an EKS cluster in AWS.
	ClusterDefinitionID *uint `json:"ClusterDefinitionID,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// AwsEksClusterInstance is a deployed instance of an EKS cluster.
type AwsEksClusterInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The AWS Region in which the cluster is provisioned.  This field is
	// stored in the instance (as well as definition) since a change to the
	// definition will not move a cluster.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The cluster instance associated with the AWS EKS cluster.
	ClusterInstanceID *uint `json:"ClusterInstanceID,omitempty" validate:"optional,association"`

	// The AWS account in which the EKS cluster is provisioned.
	AwsAccountID *uint `json:"AWSAccountID,omitempty" query:"awsaccountid" gorm:"not null" validate:"required"`

	// The definition that configures this instance.
	AwsEksClusterDefinitionID *uint `json:"AwsEksClusterDefinitionID,omitempty" query:"awseksclusterdefinitionid" gorm:"not null" validate:"required"`

	// Indicates if object is considered to be reconciled by workload controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}

// AwsRelationalDatabaseDefinition is the configuration for an RDS instance
// provided by AWS that is used by a workload.
type AwsRelationalDatabaseDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The database engine for the instance.  One of:
	// * mysql
	// * postgres
	Engine *string `json:"Engine,omitempty" query:"engine" gorm:"not null" validate:"required"`

	// The amount of storage to allocate for the database.
	Storage *int `json:"Storage,omitempty" query:"storage" gorm:"not null" validate:"required"`

	// The AWS account in which the RDS instance will be provisioned.
	AWSAccountID *uint `json:"AWSAccountID,omitempty" query:"awsaccountid" gorm:"not null" validate:"required"`
}

// AwsRelationalDatabaseInstance is a deployed instance of an RDS instance.
type AwsRelationalDatabaseInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The definition that configures this instance.
	AwsRelationalDatabaseDefinitionID *uint `json:"AwsRelationalDatabaseDefinitionID,omitempty" query:"awsrelationaldatabasedefinitionid" gorm:"not null" validate:"required"`

	// The status of the running instance
	Status *string `json:"Status,omitempty" query:"status" gorm:"not null" validate:"required"`
}
