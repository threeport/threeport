package v0

import "gorm.io/datatypes"

// GcpAccount is a user account with the Google Cloud Platform service provider.
type GcpAccount struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of an Google account.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The account ID for the Google account.
	AccountID *string `json:"AccountID,omitempty" query:"accountid" gorm:"not null" validate:"required"`

	// If true is the Google Account used if none specified in a definition.
	DefaultAccount *bool `json:"DefaultAccount,omitempty" query:"defaultaccount" gorm:"default:false" validate:"optional"`

	// The region to use for GCP managed services if not specified.
	DefaultRegion *string `json:"DefaultRegion,omitempty" query:"defaultregion" gorm:"not null" validate:"required"`

	// The cluster instances deployed in this GCP account.
	GcpGkeKubernetesRuntimeDefinitions []*GcpGkeKubernetesRuntimeDefinition `json:"GcpGkeKubernetesRuntimeDefinitions,omitempty" validate:"optional,association"`
}

// GcpGkeKubernetesRuntimeDefinition provides the configuration for GKE cluster instances.
type GcpGkeKubernetesRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The GCP account in which the GKE cluster is provisioned.
	GcpAccountID *uint `json:"GcpAccountID,omitempty" query:"gcpaccountid" gorm:"not null" validate:"required"`

	// TODO: add fields for region limitations
	// RegionsAllowed
	// RegionsForbidden

	// The number of zones the cluster should span for availability.
	ZoneCount *int `json:"ZoneCount,omitempty" query:"zonecount" gorm:"not null" validate:"required"`

	// The GCP instance type for the default initial node group.
	DefaultNodeGroupInstanceType *string `json:"DefaultNodeGroupInstanceType,omitempty" query:"defaultnodegroupinstancetype" gorm:"not null" validate:"required"`

	// The number of nodes in the default initial node group.
	DefaultNodeGroupInitialSize *int `json:"DefaultNodeGroupInitialSize,omitempty" query:"defaultnodegroupinitialsize" gorm:"not null" validate:"required"`

	// The minimum number of nodes the default initial node group should have.
	DefaultNodeGroupMinimumSize *int `json:"DefaultNodeGroupMinimumSize,omitempty" query:"defaultnodegroupminimumsize" gorm:"not null" validate:"required"`

	// The maximum number of nodes the default initial node group should have.
	DefaultNodeGroupMaximumSize *int `json:"DefaultNodeGroupMaximumSize,omitempty" query:"defaultnodegroupmaximumsize" gorm:"not null" validate:"required"`

	// The GCP GKE kubernetes runtime instances derived from this definition.
	GcpGkeKubernetesRuntimeInstances []*GcpGkeKubernetesRuntimeInstance `json:"GcpGkeKubernetesRuntimeInstances,omitempty" validate:"optional,association"`

	// The kubernetes runtime definition for a GKE cluster in GCP.
	KubernetesRuntimeDefinitionID *uint `json:"KubernetesRuntimeDefinitionID,omitempty" query:"kubernetesruntimedefinitionid" gorm:"not null" validate:"required"`
}

// GcpGkeKubernetesRuntimeInstance is a deployed instance of a GKE cluster.
type GcpGkeKubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The GCP region in which the cluster is provisioned.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The definition that configures this instance.
	GcpGkeKubernetesRuntimeDefinitionID *uint `json:"GcpGkeKubernetesRuntimeDefinitionID,omitempty" query:"gcpgkekubernetesruntimedefinitionid" gorm:"not null" validate:"required"`

	// The kubernetes runtime instance associated with the GKE cluster.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// An inventory of all GCP resources for the GKE cluster.
	ResourceInventory *datatypes.JSON `json:"ResourceInventory,omitempty" validate:"optional"`
}
