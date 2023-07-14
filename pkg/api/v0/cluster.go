//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

type ClusterDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The geographical region for the cluster roughly corresponding to cloud
	// provider regions.  Note: changes to this value will not alter the derived
	// instances which is an immutable characteristic on instances.  It will
	// only affect new instances derived from this definition.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The number of zones the cluster should span for availability.
	ZoneCount *int `json:"ZoneCount,omitempty" query:"zonecount" validate:"optional"`

	// TODO: move these values to the AWS EKS cluster definition object.
	DefaultNodeGroupInstanceType *string `json:"DefaultNodeGroupInstanceType,omitempty" query:"defaultnodegroupinstancetype" validate:"optional"`

	DefaultNodeGroupInitialSize *int `json:"DefaultNodeGroupInitialSize,omitempty" query:"defaultnodegroupinitialsize" validate:"optional"`

	DefaultNodeGroupMinimumSize *int `json:"DefaultNodeGroupMinimumSize,omitempty" query:"defaultnodegroupminimumsize" validate:"optional"`

	DefaultNodeGroupMaximumSize *int `json:"DefaultNodeGroupMaximumSize,omitempty" query:"defaultnodegroupmaximumsize" validate:"optional"`
}

type ClusterInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The geographical region for the cluster roughly corresponding to cloud
	// provider regions.  Stored in the instance (as well as definition) since a
	// change to the definition will not move a cluster.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// If true, controllers will connect to the kube API using internal DNS
	// rather than the APIEndpoint.
	ThreeportControlPlaneCluster *bool `json:"ThreeportControlPlaneCluster,omitempty" query:"threeportcontrolplanecluster" gorm:"default:false" validate:"optional"`

	// The network endpoint at which to reach the kube-api.
	APIEndpoint *string `json:"APIEndpoint,omitempty" gorm:"not null" validate:"required"`

	// The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `json:"CACertificate,omitempty" gorm:"not null" validate:"required"`

	// The client certificate to use for auth to the kube-api.
	Certificate *string `json:"Certificate,omitempty" validate:"optional"`

	// The client key to use for auth to the kube-api.
	Key *string `json:"Key,omitempty" validate:"optional"`

	// TODO: pull these EKS and AWS fields into related AWS tables
	// EKSToken is the token used for authentication by AWS EKS clusters.
	EKSToken *string `json:"EKSToken,omitempty" validate:"optional"`

	AWSConfigEnv *bool `json:"AWSConfigEnv,omitempty" validate:"optional"`

	AWSConfigProfile *string `json:"AWSConfigProfile,omitempty" validate:"optional"`

	AWSRegion *string `json:"AWSRegion,omitempty" validate:"optional"`

	// If true the cluster instance to use for deployments if not otherwise
	// specified.  Can only have one per account.
	DefaultCluster *bool `json:"DefaultCluster,omitempty" query:"defaultcluster" gorm:"default:false" validate:"optional"`

	// The cluster definition for this instance.
	ClusterDefinitionID *uint `json:"ClusterDefinitionID,omitempty" gorm:"not null" validate:"required"`

	// The associated workload instances running on this cluster.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstance,omitempty" validate:"optional,association"`

	// The WorkloadInstanceID of the gateway support service
	GatewayControllerInstanceID *uint `json:"GatewayWorkloadInstanceID,omitempty" validate:"optional"`
}
