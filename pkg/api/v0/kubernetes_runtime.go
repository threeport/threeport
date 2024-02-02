//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-sdk codegen controller --filename $GOFILE
package v0

import "time"

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl
// KubernetesRuntimeDefinition is the configuration for a Kubernetes cluster.
// TODO apply BeforeCreate functions that prevent changes to InfraProvider and
// HighAvailability fields - these are immutable.
type KubernetesRuntimeDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The infrastructure provider running the compute infrastructure for the
	// cluster.
	InfraProvider *string `json:"InfraProvider,omitempty" query:"infraprovider" gorm:"not null" validate:"required"`

	// The infra provider account name.  Determines which account the infra is
	// deployed on.
	InfraProviderAccountName *string `json:"InfraProviderAccountName,omitempty" query:"infraprovideraccountname" validate:"optional"`

	// If true, will be deployed in a highly available configuration across
	// multiple zones within a region and with multiple replicas of Kubernetes
	// control plane components.
	HighAvailability *bool `json:"HighAvailability,omitempty" query:"highavailability" gorm:"default:false" validate:"optional"`

	// Sets the compute capacity of the machine type for the default node group.
	NodeSize *string `json:"NodeSize,omitempty" query:"nodesize" gorm:"default:Medium" validate:"optional"`

	// Sets the CPU:memory ration of the machine type for the default node
	// group.
	NodeProfile *string `json:"NodeProfile,omitempty" query:"nodeprofile" gorm:"default:Balanced" validate:"optional"`

	// Sets the maximum number of nodes for the default node group.
	NodeMaximum *int `json:"NodeMaximum,omitempty" query:"nodemaximum" gorm:"default:250" validate:"optional"`

	// TODO: add fields for location limitations
	// LocationsAllowed
	// LocationsForbidden

	// The associated kubernetes runtime instances that are deployed from this
	// definition.
	KubernetesRuntimeInstances []*KubernetesRuntimeInstance `json:"KubernetesRuntimeInstances,omitempty" validate:"optional,association"`
}

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl
// KubernetesRuntimeInstance is a deployed instance of a Kubernetes cluster.
type KubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The geographical location for the runtime cluster.  This is an
	// abstraction for the cloud provider regions that is mapped into the
	// regions used by providers.
	Location *string `json:"Location,omitempty" query:"location" gorm:"not null" validate:"required"`

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

	// The client certificate key to use for auth to the kube-api.
	CertificateKey *string `json:"CertificateKey,omitempty" validate:"optional" encrypt:"true"`

	// Used to authenticate with a OIDC provider that implements auth for a
	// Kubernetes cluster.  It is an alternative to client cert authenticaion.
	ConnectionToken *string `json:"ConnectionToken,omitempty" validate:"optional" encrypt:"true"`

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

	// The associated control plane instances running on this kubernetes runtime instance.
	ControlPlaneInstances []*ControlPlaneInstance `json:"ControlPlaneInstance,omitempty" validate:"optional,association"`

	// If true, delete the runtime even if there are workloads present.
	ForceDelete *bool `json:"ForceDelete,omitempty" query:"forcedelete" gorm:"default:false" validate:"optional"`

	// The WorkloadInstanceID of the gateway support service
	GatewayControllerInstanceID *uint `json:"GatewayWorkloadInstanceID,omitempty" validate:"optional"`

	// The WorkloadInstanceID of the gateway support service
	DnsControllerInstanceID *uint `json:"DnsControllerInstanceId,omitempty" validate:"optional"`

	// An alternate threeport image to use when deploying threeport agent to
	// managed Kubernetes runtime clusters.  If not supplied, the official image
	// with the correct version will be used.
	ThreeportAgentImage *string `json:"ThreeportAgentImage,omitempty" validate:"optional"`
}
