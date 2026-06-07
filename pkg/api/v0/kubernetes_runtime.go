package v0

import "time"

// KubernetesRuntimeDefinition is the configuration for a Kubernetes cluster.
type KubernetesRuntimeDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The infrastructure provider running the compute infrastructure for the
	// cluster.
	InfraProvider *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The infra provider account name.  Determines which account the infra is
	// deployed on.
	InfraProviderAccountName *string `json:",omitempty" validate:"optional"`

	// If true, will be deployed in a highly available configuration across
	// multiple zones within a region and with multiple replicas of Kubernetes
	// control plane components.
	HighAvailability *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// Sets the compute capacity of the machine type for the default node group.
	NodeSize *string `json:",omitempty" validate:"optional" gorm:"default:Medium"`

	// Sets the CPU:memory ration of the machine type for the default node
	// group.
	NodeProfile *string `json:",omitempty" validate:"optional" gorm:"default:Balanced"`

	// Sets the maximum number of nodes for the default node group.
	NodeMaximum *int `json:",omitempty" validate:"optional" gorm:"default:250"`

	// TODO: add fields for location limitations
	// LocationsAllowed
	// LocationsForbidden

	// The associated kubernetes runtime instances that are deployed from this
	// definition.
	KubernetesRuntimeInstances []*KubernetesRuntimeInstance `json:",omitempty" validate:"optional,association"`
}

// KubernetesRuntimeInstance is a deployed instance of a Kubernetes cluster.
type KubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The geographical location for the runtime cluster.  This is an
	// abstraction for the cloud provider regions that is mapped into the
	// regions used by providers.
	Location *string `json:",omitempty" validate:"required" gorm:"not null"`

	// If true, the Kubernetes cluster is hosting a threeport control plane and
	// any controllers that connect to the kube API will use internal cluster
	// DNS rather than the external APIEndpoint.
	ThreeportControlPlaneHost *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// The network endpoint at which to reach the kube-api.
	APIEndpoint *string `json:",omitempty" validate:"optional"`

	// The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `json:",omitempty" validate:"optional"`

	// The client certificate to use for auth to the kube-api.
	Certificate *string `json:",omitempty" validate:"optional"`

	// The client certificate key to use for auth to the kube-api.
	CertificateKey *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// Used to authenticate with a OIDC provider that implements auth for a
	// Kubernetes cluster.  It is an alternative to client cert authenticaion.
	ConnectionToken *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// ConnectionTokenExpiration is the time when a ConnectionToken will expire.
	// Used to ensure a token will not expire before it can be used.
	ConnectionTokenExpiration *time.Time `json:",omitempty" validate:"optional"`

	// If true, this Kubernetes cluster will be used for all workloads if not
	// otherwise assigned.
	DefaultRuntime *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// The kubernetes runtime definition for this instance.
	KubernetesRuntimeDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The associated kubernetes workload instances running on this kubernetes runtime.
	KubernetesWorkloadInstances []*KubernetesWorkloadInstance `json:",omitempty" validate:"optional,association"`

	// The associated control plane instances running on this kubernetes runtime instance.
	ControlPlaneInstances []*ControlPlaneInstance `json:",omitempty" validate:"optional,association"`

	// The KubernetesWorkloadInstanceID of the gateway support service
	GatewayControllerInstanceID *uint `json:",omitempty" validate:"optional"`

	// The KubernetesWorkloadInstanceID of the dns support service
	DnsControllerInstanceID *uint `json:",omitempty" validate:"optional"`

	// The KubernetesWorkloadInstanceID of the secrets support service
	SecretsControllerInstanceID *uint `json:",omitempty" validate:"optional"`

	// An alternate threeport image to use when deploying threeport agent to
	// managed Kubernetes runtime clusters.  If not supplied, the official image
	// with the correct version will be used.
	ThreeportAgentImage *string `json:",omitempty" validate:"optional"`
}
