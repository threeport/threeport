package v0

import (
	"gorm.io/datatypes"
)

// OciProvider is a provider account with the Oracle Cloud Infrastructure service provider.
type OciProvider struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of an OCI provider.
	Name *string `validate:"required" gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`

	// The user OCID credentials for the OCI provider.
	UserOCID *string `validate:"required" gorm:"not null"`

	// The tenancy OCID for the OCI provider account.
	TenancyOCID *string `validate:"required" gorm:"not null"`

	// The compartment OCID for the OCI provider.
	CompartmentOCID *string `validate:"required" gorm:"not null"`

	// If true is the OCI provider used if none specified in an instance.
	DefaultProvider *bool `validate:"optional" gorm:"default:false"`

	// The region to use for OCI managed services if not specified.
	DefaultRegion *string `validate:"required" gorm:"not null"`

	// The fingerprint of the API key for the OCI provider.
	KeyFingerprint *string `validate:"required" gorm:"not null"`

	// The private key for the OCI provider.
	PrivateKey *string `validate:"required" gorm:"not null" encrypt:"true"`

	// The cluster instances deployed with this OCI provider.
	OciOkeKubernetesRuntimeInstances []*OciOkeKubernetesRuntimeInstance `validate:"optional,association"`
}

// OciOkeKubernetesRuntimeDefinition provides the configuration for OKE cluster instances.
type OciOkeKubernetesRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The OCI shape for the worker nodes.
	WorkerNodeShape *string `validate:"required" gorm:"not null"`

	// The number of nodes in the worker node pool.
	WorkerNodeInitialCount *int32 `validate:"required" gorm:"not null"`

	// The OCI OKE kubernetes runtime instances derived from this definition.
	OciOkeKubernetesRuntimeInstances []*OciOkeKubernetesRuntimeInstance `validate:"optional,association"`

	// The kubernetes runtime definition for an OKE cluster in OCI.
	KubernetesRuntimeDefinitionID *uint `validate:"required" gorm:"not null" relationship:"marries"`
}

// OciOkeKubernetesRuntimeInstance is a deployed instance of an OKE cluster.
type OciOkeKubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The OCI provider used to provision this instance.
	OciProviderID *uint `validate:"required" gorm:"not null" relationship:"requires"`

	// The OCI Region in which the cluster is provisioned. This field is
	// stored in the instance (as well as definition) since a change to the
	// definition will not move a cluster.
	Region *string `validate:"optional"`

	// The definition that configures this instance.
	OciOkeKubernetesRuntimeDefinitionID *uint `validate:"required" gorm:"not null" relationship:"requires"`

	// An inventory of all OCI resources for the OKE cluster.
	ResourceInventory *datatypes.JSON `validate:"optional"`

	// The kubernetes runtime instance associated with the OCI OKE cluster.
	KubernetesRuntimeInstanceID *uint `validate:"required" gorm:"not null" relationship:"marries"`

	// The OCID for the OKE cluster. Populated by the controller after cluster creation.
	ClusterOCID *string `validate:"optional"`
}
