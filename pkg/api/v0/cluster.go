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
	ZoneCount *int32 `json:"ZoneCount,omitempty" query:"zonecount" validate:"optional"`

	DefaultNodeGroupInstanceType *string `json:"DefaultNodeGroupInstanceType,omitempty" query:"defaultnodegroupinstancetype" validate:"optional"`

	DefaultNodeGroupInitialSize *int32 `json:"DefaultNodeGroupInitialSize,omitempty" query:"defaultnodegroupinitialsize" validate:"optional"`

	DefaultNodeGroupMinimumSize *int32 `json:"DefaultNodeGroupMinimumSize,omitempty" query:"defaultnodegroupminimumsize" validate:"optional"`

	DefaultNodeGroupMaximumSize *int32 `json:"DefaultNodeGroupMaximumSize,omitempty" query:"defaultnodegroupmaximumsize" validate:"optional"`
}

type ClusterInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	//// The provider or technology used to provision the cluster.
	//Provider *string `json:"Provider,omitempty" query:"provider" gorm:"not null" validate:"required"`

	// The geographical region for the cluster roughly corresponding to cloud
	// provider regions.  Stored in the instance (as well as definition) since a
	// change to the definition will not move a cluster.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// If true, controllers will connect to the kube API using internal DSN
	// rather than the APIEndpoint.
	ThreeportControlPlaneCluster *bool `json:"ThreeportControlPlaneCluster,omitempty" query:"threeportcontrolplanecluster" gorm:"default:false" validate:"optional"`

	// The network endpoint at which to reach the kube-api.
	APIEndpoint *string `json:"APIEndpoint,omitempty" gorm:"not null" validate:"required"`

	// The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `json:"CACertificate,omitempty" validate:"optional"`

	// The client certificate to use for auth to the kube-api.
	Certificate *string `json:"Certificate,omitempty" gorm:"not null" validate:"required"`

	// The client key to use for auth to the kube-api.
	Key *string `json:"Key,omitempty" gorm:"not null" validate:"required"`

	// If true the cluster instance to use for deployments if not otherwise
	// specified.  Can only have one per account.
	DefaultCluster *bool `json:"DefaultCluster,omitempty" query:"defaultcluster" gorm:"default:false" validate:"optional"`

	ClusterDefinitionID *uint `json:"ClusterDefinitionID,omitempty" gorm:"not null" validate:"required"`

	// The associated workload instances running on this cluster.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstance,omitempty" validate:"optional,association"`
}
