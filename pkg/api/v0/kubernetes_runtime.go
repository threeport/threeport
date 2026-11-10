package v0

import (
	"time"

	"gorm.io/datatypes"
)

// KubernetesRuntimeDefinition is the configuration for a Kubernetes cluster.
type KubernetesRuntimeDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The infrastructure provider running the compute infrastructure for the
	// cluster.
	InfraProvider *string `validate:"required" gorm:"not null"`

	// The infra provider account name.  Determines which account the infra is
	// deployed on.
	InfraProviderAccountName *string `validate:"optional"`

	// If true, will be deployed in a highly available configuration across
	// multiple zones within a region and with multiple replicas of Kubernetes
	// control plane components.
	HighAvailability *bool `validate:"optional" gorm:"default:false"`

	// Sets the compute capacity of the machine type for the default node group.
	NodeSize *string `validate:"optional" gorm:"default:Medium"`

	// Sets the CPU:memory ration of the machine type for the default node
	// group.
	NodeProfile *string `validate:"optional" gorm:"default:Balanced"`

	// Sets the maximum number of nodes for the default node group.
	NodeMaximum *int `validate:"optional" gorm:"default:250"`

	// If true, the Kubernetes API is not reachable from the public internet.
	// It is reachable only from the cluster's own network, from networks
	// connected to it and, on GKE, through the IAM-authorized DNS endpoint.
	PrivateApi *bool `validate:"optional" gorm:"default:false"`

	// The CIDR blocks allowed to reach the Kubernetes API, as a JSON array of
	// IPv4 CIDR blocks.  With PrivateApi false, this restricts the public
	// endpoint.  With PrivateApi true, it lists the private networks, such as
	// a VPN or a peered network, that may reach the private endpoint.  Empty
	// means no restriction beyond PrivateApi.  EKS admits up to 40 blocks,
	// GKE up to 50 (100 with PrivateApi) and OKE up to 120.
	AllowedCidrs *datatypes.JSON `validate:"optional" swaggertype:"array,string"`

	// The Kubernetes minor version of the clusters, such as "1.36", on EKS,
	// GKE and OKE.  If unset, the default version is used, and the EKS, GKE
	// or OKE definition records it.  It can't be changed after creation.  A
	// kind cluster's version comes with its node image, so it takes none.
	KubernetesVersion *string `validate:"optional"`

	// TODO: add fields for location limitations
	// LocationsAllowed
	// LocationsForbidden

	// The associated kubernetes runtime instances that are deployed from this
	// definition.
	KubernetesRuntimeInstances []*KubernetesRuntimeInstance `validate:"optional,association"`
}

// KubernetesRuntimeInstance is a deployed instance of a Kubernetes cluster.
type KubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The geographical location for the runtime cluster.  This is an
	// abstraction for the cloud provider regions that is mapped into the
	// regions used by providers.
	Location *string `validate:"required" gorm:"not null"`

	// If true, the Kubernetes cluster is hosting a threeport control plane and
	// any controllers that connect to the kube API will use internal cluster
	// DNS rather than the external APIEndpoint.
	ThreeportControlPlaneHost *bool `validate:"optional" gorm:"default:false"`

	// The network endpoint at which to reach the kube-api.
	APIEndpoint *string `validate:"optional"`

	// The public addresses that this cluster's outbound internet traffic
	// comes from, as a JSON array of CIDR blocks, one /32 per address.
	// Recorded for every cluster Threeport provisions.  Workload runtimes
	// that restrict their Kubernetes API with AllowedCidrs also admit the
	// addresses of the runtime that hosts the control plane, so that the
	// control plane can reach them.
	EgressCidrs *datatypes.JSON `validate:"optional" swaggertype:"array,string"`

	// The CA certificate used to generate the cert and key if
	// self-signed.
	CACertificate *string `validate:"optional"`

	// The client certificate to use for auth to the kube-api.
	Certificate *string `validate:"optional"`

	// The client certificate key to use for auth to the kube-api.
	CertificateKey *string `validate:"optional" encrypt:"true"`

	// Used to authenticate with a OIDC provider that implements auth for a
	// Kubernetes cluster.  It is an alternative to client cert authenticaion.
	ConnectionToken *string `validate:"optional" encrypt:"true"`

	// ConnectionTokenExpiration is the time when a ConnectionToken will expire.
	// Used to ensure a token will not expire before it can be used.
	ConnectionTokenExpiration *time.Time `validate:"optional"`

	// If true, this Kubernetes cluster will be used for all workloads if not
	// otherwise assigned.
	DefaultRuntime *bool `validate:"optional" gorm:"default:false"`

	// The kubernetes runtime definition for this instance.
	KubernetesRuntimeDefinitionID *uint `validate:"required" gorm:"not null" relationship:"requires"`

	// The associated kubernetes workload instances running on this kubernetes runtime.
	KubernetesWorkloadInstances []*KubernetesWorkloadInstance `validate:"optional,association"`

	// The associated control plane instances running on this kubernetes runtime instance.
	ControlPlaneInstances []*ControlPlaneInstance `validate:"optional,association"`

	// The workload that runs the ingress component for this runtime.
	IngressComponentInstanceID *uint `validate:"optional"`

	// The workload that runs the certificates component for this runtime.
	CertificatesComponentInstanceID *uint `validate:"optional"`

	// The workload that stores secrets for this runtime.
	SecretsComponentInstanceID *uint `validate:"optional"`

	// The workload that provides a database for this runtime.
	DatabaseComponentInstanceID *uint `validate:"optional"`

	// An alternate threeport image to use when deploying threeport agent to
	// managed Kubernetes runtime clusters.  If not supplied, the official image
	// with the correct version will be used.
	ThreeportAgentImage *string `validate:"optional"`
}
