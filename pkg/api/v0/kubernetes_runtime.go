//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate ../../../bin/threeport-codegen controller --filename $GOFILE
package v0

import "time"

// +threeport-codegen:reconciler
// KubernetesRuntimeDefinition is the configuration for a Kubernetes cluster.
// TODO apply BeforeCreate functions that prevent changes to InfraProvider and
// HighAvailability fields - these are immutable.
type KubernetesRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The infrastructure provider running the compute infrastructure for the
	// cluster.
	InfraProvider *string `json:"InfraProvider,omitempty" query:"infraprovider" gorm:"not null" validate:"required"`

	// The infra provider account ID.  Determines which account the infra is
	// deployed on.
	InfraProviderAccountName *string `json:"InfraProviderAccountName,omitempty" query:"infraprovideraccountname" validate:"optional"`

	// If true, will be deployed in a highly available configuration across
	// multiple zones within a region and with multiple replicas of Kubernetes
	// control plane components.
	HighAvailability *bool `json:"HighAvailability,omitempty" query:"highavailability" validate:"optional"`

	// TODO: add fields for location limitations
	// LocationsAllowed
	// LocationsForbidden

	// The associated kubernetes runtime instances that are deployed from this
	// definition.
	KubernetesRuntimeInstances []*KubernetesRuntimeInstance `json:"KubernetesRuntimeInstances,omitempty" validate:"optional,association"`

	// Indicates if object is considered to be reconciled by the kubernetes
	// runtime controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}

// +threeport-codegen:reconciler
// KubernetesRuntimeInstance is a deployed instance of a Kubernetes cluster.
// TODO: Apply BeforeCreate to the Location field - it is immutable.
type KubernetesRuntimeInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The geographical location for the runtime cluster.  This is an
	// abstraction for the cloud provider regions that is mapped into the
	// regions used by providers.
	Location *string `json:"Location,omitempty" query:"location" validate:"optional"`

	// If true, the Kubernetes cluster is hosting a threeport control plane and
	// any controllers that connect to the kube API will use internal cluster
	// DNS rather than the external APIEndpoint.
	ThreeportControlPlaneHost *bool `json:"ThreeportControlPlaneHost,omitempty" query:"threeportcontrolplanehost" gorm:"default:false" validate:"optional"`

	// The network endpoint at which to reach the kube-api.
	APIEndpoint *string `json:"APIEndpoint,omitempty" validate:"optional"`

	// The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `json:"CACertificate,omitempty" validate:"optional"`

	// The client certificate to use for auth to the kube-api.
	Certificate *string `json:"Certificate,omitempty" validate:"optional"`

	// The client key to use for auth to the kube-api.
	Key *string `json:"Key,omitempty" validate:"optional"`

	// ConnectionToken is used to authenticate with a OIDC provider that
	// implements auth for a Kubernetes cluster.  It is an alternative to client
	// certficate and key authenticaion.
	ConnectionToken *string `json:"ConnectionToken,omitempty" validate:"optional"`

	// ConnectionTokenExpiration is the time when a ConnectionToken will expire.
	// Used to ensure a token will not expire before it can be used.
	ConnectionTokenExpiration *time.Time `json:"ConnectionTokenExpiration,omitempty" validate:"optional"`

	// If true, this Kubernetes cluster will be used for all workloads if not
	// otherwise assigned.
	DefaultRuntime *bool `json:"DefaultRuntime,omitempty" query:"defaultruntime" gorm:"default:false" validate:"optional"`

	// The kubernetes runtime definition for this instance.
	KubernetesRuntimeDefinitionID *uint `json:"KubernetesRuntimeDefinitionID,omitempty" query:"kubernetesruntimedefinitionid" gorm:"not null" validate:"required"`

	// The associated workload instances running on this kubernetes runtime.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstance,omitempty" validate:"optional,association"`

	// The WorkloadInstanceID of the gateway support service
	GatewayControllerInstanceID *uint `json:"GatewayWorkloadInstanceID,omitempty" validate:"optional"`

	// The WorkloadInstanceID of the dns support service
	DnsControllerInstanceID *uint `json:"DnsWorkloadInstanceID,omitempty" validate:"optional"`

	// An alternate threeport image to use when deploying threeport agent to
	// managed Kubernetes runtime clusters.  If not supplied, the official image
	// with the correct version will be used.
	ThreeportAgentImage *string `json:"ThreeportAgentImage,omitempty" validate:"optional"`

	// Indicates if object is considered to be reconciled by the kubernetes
	// runtime controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}
