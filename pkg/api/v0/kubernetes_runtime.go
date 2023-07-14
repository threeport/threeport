//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

type KubernetesRuntimeDefinition struct {
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

type KubernetesRuntimeInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The geographical region for the cluster roughly corresponding to cloud
	// provider regions.  Stored in the instance (as well as definition) since a
	// change to the definition will not move a cluster.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// If true, the Kubernetes cluster is hosting a threeport control plane and
	// any controllers that connect to the kube API will use internal cluster
	// DNS rather than the external APIEndpoint.
	ThreeportControlPlaneHost *bool `json:"ThreeportControlPlaneHost,omitempty" query:"threeportcontrolplanehost" gorm:"default:false" validate:"optional"`

	// The network endpoint at which to reach the kube-api.
	APIEndpoint *string `json:"APIEndpoint,omitempty" gorm:"not null" validate:"required"`

	// The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `json:"CACertificate,omitempty" gorm:"not null" validate:"required"`

	// The client certificate to use for auth to the kube-api.
	Certificate *string `json:"Certificate,omitempty" validate:"optional"`

	// The client key to use for auth to the kube-api.
	Key *string `json:"Key,omitempty" validate:"optional"`

	// ConnectionToken is used to authenticate with a OIDC provider that
	// implements auth for a Kubernetes cluster.  It is an alternative to client
	// certficate and key authenticaion.
	ConnectionToken *string `json:"ConnectionToken,omitempty" validate:"optional"`

	// If true, this Kubernetes cluster will be used for all workloads if not
	// otherwise assigned.
	DefaultRuntime *bool `json:"DefaultRuntime,omitempty" query:"defaultruntime" gorm:"default:false" validate:"optional"`

	// The kubernetes runtime definition for this instance.
	KubernetesRuntimeDefinitionID *uint `json:"KubernetesRuntimeDefinitionID,omitempty" gorm:"not null" validate:"required"`

	// The associated workload instances running on this kubernetes runtime.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstance,omitempty" validate:"optional,association"`

	// The WorkloadInstanceID of the gateway support service
	GatewayControllerInstanceID *uint `json:"GatewayWorkloadInstanceID,omitempty" validate:"optional"`
}
