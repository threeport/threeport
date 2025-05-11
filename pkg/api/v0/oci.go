package v0

import (
	"gorm.io/datatypes"
)

// OciAccount is a user account with the Oracle Cloud Infrastructure service provider.
type OciAccount struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of an OCI account.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The tenancy OCID for the OCI account.
	TenancyID *string `json:"TenancyID,omitempty" query:"tenancyid" gorm:"not null" validate:"required"`

	// If true is the OCI Account used if none specified in a definition.
	DefaultAccount *bool `json:"DefaultAccount,omitempty" query:"defaultaccount" gorm:"default:false" validate:"optional"`

	// The region to use for OCI managed services if not specified.
	DefaultRegion *string `json:"DefaultRegion,omitempty" query:"defaultregion" gorm:"not null" validate:"required"`

	// The user OCID credentials for the OCI account.
	UserOCID *string `json:"UserOCID,omitempty" validate:"optional" encrypt:"true"`

	// The fingerprint of the API key for the OCI account.
	KeyFingerprint *string `json:"KeyFingerprint,omitempty" validate:"optional" encrypt:"true"`

	// The private key for the OCI account.
	PrivateKey *string `json:"PrivateKey,omitempty" validate:"optional" encrypt:"true"`

	// The cluster instances deployed in this OCI account.
	OciOkeKubernetesRuntimeDefinitions []*OciOkeKubernetesRuntimeDefinition `json:"OciOkeKubernetesRuntimeDefinitions,omitempty" validate:"optional,association"`
}

// OciOkeKubernetesRuntimeDefinition provides the configuration for OKE cluster instances.
type OciOkeKubernetesRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The OCI account in which the OKE cluster is provisioned.
	OciAccountID *uint `json:"OciAccountID,omitempty" query:"ociaccountid" gorm:"not null" validate:"required"`

	// The number of availability domains the cluster should span.
	AvailabilityDomainCount *int32 `json:"AvailabilityDomainCount,omitempty" query:"availabilitydomaincount" gorm:"not null" validate:"required"`

	// The OCI shape for the worker nodes.
	WorkerNodeShape *string `json:"WorkerNodeShape,omitempty" query:"workernodeshape" gorm:"not null" validate:"required"`

	// The number of nodes in the worker node pool.
	WorkerNodeInitialCount *int32 `json:"WorkerNodeInitialCount,omitempty" query:"workernodeinitialcount" gorm:"not null" validate:"required"`

	// The minimum number of nodes the worker node pool should have.
	WorkerNodeMinCount *int32 `json:"WorkerNodeMinCount,omitempty" query:"workernodemincount" gorm:"not null" validate:"required"`

	// The maximum number of nodes the worker node pool should have.
	WorkerNodeMaxCount *int32 `json:"WorkerNodeMaxCount,omitempty" query:"workernodemaxcount" gorm:"not null" validate:"required"`

	// The OCI OKE kubernetes runtime instances derived from this definition.
	OciOkeKubernetesRuntimeInstances []*OciOkeKubernetesRuntimeInstance `json:"OciOkeKubernetesRuntimeInstances,omitempty" validate:"optional,association"`

	// The kubernetes runtime definition for an OKE cluster in OCI.
	KubernetesRuntimeDefinitionID *uint `json:"KubernetesRuntimeDefinitionID,omitempty" query:"kubernetesruntimedefinitionid" gorm:"not null" validate:"required"`
}

// OciOkeKubernetesRuntimeInstance is a deployed instance of an OKE cluster.
type OciOkeKubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The OCI Region in which the cluster is provisioned. This field is
	// stored in the instance (as well as definition) since a change to the
	// definition will not move a cluster.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The definition that configures this instance.
	OciOkeKubernetesRuntimeDefinitionID *uint `json:"OciOkeKubernetesRuntimeDefinitionID,omitempty" query:"ociokekubernetesruntimedefinitionid" gorm:"not null" validate:"required"`

	// An inventory of all OCI resources for the OKE cluster.
	ResourceInventory *datatypes.JSON `json:"ResourceInventory,omitempty" validate:"optional"`

	// The kubernetes runtime instance associated with the OCI OKE cluster.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`
}
