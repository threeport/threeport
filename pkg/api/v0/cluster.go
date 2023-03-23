//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

type ClusterDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	//// Required.  An arbitrary name for the cluster definition.
	//Name *string `json:"Name,omitempty" query:"name" gorm:"not null"  validate:"required"`

	// The geographical region for the cluster roughly corresponding to cloud
	// provider regions.
	// TODO: determine whether to make this attribute immutable b/c cluster
	// instances will not be moved once deployed.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The number of zones the cluster should span for availability.
	ZoneCount *int32 `json:"ZoneCount,omitempty" query:"zonecount" gorm:"not null" validate:"required"`

	DefaultNodeGroupInstanceType *string `json:"DefaultNodeGroupInstanceType,omitempty" query:"defaultnodegroupinstancetype" gorm:"not null" validate:"required"`

	DefaultNodeGroupInitialSize *int32 `json:"DefaultNodeGroupInitialSize,omitempty" query:"defaultnodegroupinitialsize" gorm:"not null" validate:"required"`

	DefaultNodeGroupMinimumSize *int32 `json:"DefaultNodeGroupMinimumSize,omitempty" query:"defaultnodegroupminimumsize" gorm:"not null" validate:"required"`

	DefaultNodeGroupMaximumSize *int32 `json:"DefaultNodeGroupMaximumSize,omitempty" query:"defaultnodegroupmaximumsize" gorm:"not null" validate:"required"`
}

type ClusterInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	//// Required.  The provider or technology used to provision the cluster.
	//Provider *string `json:"Provider,omitempty" query:"provider" gorm:"not null" validate:"required"`

	// The geographical region for the cluster roughly corresponding to cloud
	// provider regions.  Stored in the instance (as well as definition) since a
	// change to the definition will not move a cluster.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// Required.  The network endpoint at which to reach the kube-api.
	APIEndpoint *string `json:"APIEndpoint,omitempty" gorm:"not null" validate:"required"`

	// Optional.  The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `json:"CACertificate,omitempty" validate:"optional"`

	// Required.  The client certificate to use for auth to the kube-api.
	Certificate *string `json:"Certificate,omitempty" gorm:"not null" validate:"required"`

	// Required.  The client key to use for auth to the kube-api.
	Key *string `json:"Key,omitempty" gorm:"not null" validate:"required"`

	ClusterDefinitionID *uint

	// The associated workload instances running on this cluster.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstance,omitempty" validate:"optional,association"`
}
