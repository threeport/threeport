package v0

import (
	"time"

	"gorm.io/datatypes"
)

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

	// The Kubernetes minor version of the clusters, such as "1.36": the
	// KubernetesRuntimeDefinition's KubernetesVersion, or the default version
	// when that is unset.  If unset here, the default version is used.
	// Raising it to another supported version upgrades the clusters, one minor
	// version at a time.  It can't be lowered, since a cluster can't be
	// downgraded, or cleared.
	KubernetesVersion *string `validate:"optional"`

	// The GKE release channel of the clusters, REGULAR or STABLE.  GKE
	// upgrades their control planes and nodes to its releases on its own,
	// within the maintenance window.  If unset, REGULAR is used.  It can't be
	// changed after creation.
	ReleaseChannel *string `validate:"optional"`

	// The days the clusters' maintenance window recurs on, in UTC, as an
	// RRULE writes them: such as "SA,SU".  GKE upgrades a cluster only in that
	// window, apart from an upgrade it can't defer.  If unset, "SA,SU" is used.
	// It can't be changed after creation.
	MaintenanceWindowDays *string `validate:"optional"`

	// The time of day the maintenance window starts on each of its days, in
	// UTC, such as "04:00".  If unset, "04:00" is used.  It can't be changed
	// after creation.
	MaintenanceWindowStart *string `validate:"optional"`

	// How many hours the maintenance window lasts, from 4 to 24.  If unset, 8
	// is used.  It can't be changed after creation.
	MaintenanceWindowHours *int `validate:"optional"`

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

	// The Kubernetes minor version the cluster runs, such as "1.36", as the
	// controller last read it from the provider.  That is its control
	// plane's: GKE upgrades its nodes to it in the maintenance window.
	KubernetesVersion *string `validate:"optional"`

	// The definition's KubernetesVersion the cluster was last asked to be
	// upgraded to.  The API server sets it when an update of the GKE
	// definition asks the cluster to upgrade, once the cluster's controller
	// has been notified, so a request it couldn't finish is made again by the
	// definition's next update.
	UpgradeRequestedVersion *string `validate:"optional"`

	// Set by the controller when it starts upgrading the cluster to the
	// definition's KubernetesVersion, and refreshed while the upgrade runs.
	UpgradeAcknowledged *time.Time `validate:"optional"`

	// True while the controller upgrades the cluster.
	Upgrading *bool `validate:"optional" gorm:"default:false"`

	// True when the last upgrade failed.  The controller tries it again after
	// a wait.
	UpgradeFailed *bool `validate:"optional" gorm:"default:false"`

	// True when the controller refused to upgrade the cluster: because the
	// upgrade would replace or delete any of its resources, or because
	// Threeport didn't create the cluster.  It doesn't try again until the
	// definition's KubernetesVersion changes, or this is set back to false.
	UpgradeRefused *bool `validate:"optional" gorm:"default:false"`

	// What the controller last did to upgrade the cluster, or why it
	// couldn't.
	UpgradeStatus *string `validate:"optional"`
}
