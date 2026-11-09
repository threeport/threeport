package v0

import "gorm.io/datatypes"

// GcpProvider represents a Google Cloud Platform (GCP) project in an account with the GCP service provider.
type GcpProvider struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of a GCP provider.
	Name *string `validate:"required" gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`

	// The GCP project ID for the Google Cloud account.
	ProjectID *string `validate:"required" gorm:"not null"`

	// If true, is the GCP provider used when none specified for an instance.
	DefaultProvider *bool `validate:"optional" gorm:"default:false"`

	// The region to use for GCP managed services if not specified.
	DefaultRegion *string `validate:"required" gorm:"not null"`

	// The service account key JSON for authenticating to GCP from outside GCP.
	// This is the contents of a service account key file exported from GCP Console.
	// Used when the gcp-controller runs outside GCP (e.g., in AWS or on-prem).
	ServiceAccountCredentials *string `validate:"optional" encrypt:"true"`

	// The cluster instances deployed with this GCP provider.
	GcpGkeKubernetesRuntimeInstances []*GcpGkeKubernetesRuntimeInstance `validate:"optional,association"`
}

// GcpGkeKubernetesRuntimeDefinition provides the configuration for GKE cluster instances.
type GcpGkeKubernetesRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// TODO: add fields for region limitations
	// RegionsAllowed
	// RegionsForbidden

	// The number of zones the cluster should span for availability.
	ZoneCount *int `validate:"required" gorm:"not null"`

	// The GCP instance type for the default initial node group.
	DefaultNodeGroupInstanceType *string `validate:"required" gorm:"not null"`

	// The number of nodes in the default initial node group.
	DefaultNodeGroupInitialSize *int `validate:"required" gorm:"not null"`

	// The minimum number of nodes the default initial node group should have.
	DefaultNodeGroupMinimumSize *int `validate:"required" gorm:"not null"`

	// The maximum number of nodes the default initial node group should have.
	DefaultNodeGroupMaximumSize *int `validate:"required" gorm:"not null"`

	// If true, the Kubernetes API is not reachable from the public internet.
	// Set from the KubernetesRuntimeDefinition's PrivateApi.
	PrivateApi *bool `validate:"optional" gorm:"default:false"`

	// The CIDR blocks allowed to reach the Kubernetes API, as a JSON array of
	// IPv4 CIDR blocks.  Set from the KubernetesRuntimeDefinition's
	// AllowedCidrs.
	AllowedCidrs *datatypes.JSON `validate:"optional" swaggertype:"array,string"`

	// The GCP GKE kubernetes runtime instances derived from this definition.
	GcpGkeKubernetesRuntimeInstances []*GcpGkeKubernetesRuntimeInstance `validate:"optional,association"`

	// The kubernetes runtime definition for a GKE cluster in GCP.
	KubernetesRuntimeDefinitionID *uint `validate:"required" gorm:"not null" relationship:"marries"`
}

// GcpGkeKubernetesRuntimeInstance is a deployed instance of a GKE cluster.
type GcpGkeKubernetesRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The GCP provider in which the GKE cluster is provisioned.
	GcpProviderID *uint `validate:"required" gorm:"not null" relationship:"requires"`

	// The GCP region in which the cluster is provisioned.
	Region *string `validate:"optional"`

	// The definition that configures this instance.
	GcpGkeKubernetesRuntimeDefinitionID *uint `validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes runtime instance associated with the GKE cluster.
	KubernetesRuntimeInstanceID *uint `validate:"required" gorm:"not null" relationship:"marries"`

	// An inventory of all GCP resources for the GKE cluster.
	ResourceInventory *datatypes.JSON `validate:"optional"`

	// Set by an operator to allow this instance to be deleted even though its
	// creation was confirmed and no resource inventory is recorded, or the
	// recorded inventory cannot be restored. The controller refuses that delete
	// otherwise, because cloud resources may still exist. Setting this orphans
	// any resources the controller cannot find: they keep running and are no
	// longer tracked by Threeport. Resources it has recorded and can restore are
	// still destroyed. It is rejected on create, and once set it stays set until
	// changed, so tptctl sets it back to false if the delete fails.
	AbandonInfrastructure *bool `validate:"optional" gorm:"default:false"`

	// If true, the cluster's Cloud NAT uses an address reserved for it, so
	// that its outbound traffic comes from one address that does not change
	// and is recorded as the kubernetes runtime instance's EgressCidrs.  Set
	// on clusters created since Threeport reserves one; clusters created
	// before keep the addresses Google allocates.  Cannot be changed after
	// creation.
	ReservedNatAddress *bool `validate:"optional" gorm:"default:false"`
}
