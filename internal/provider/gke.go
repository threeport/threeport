package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	gkecontainer "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/projects"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/iam/v1"
	gcpiam "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	gcpoption "google.golang.org/api/option"
	"google.golang.org/api/storage/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// KubernetesRuntimeInfraGKE represents the infrastructure for a threeport-managed
// GKE (Google Kubernetes Engine) cluster.
type KubernetesRuntimeInfraGKE struct {
	// PulumiWorkspace provides workspace, stack, state, and automation API helpers
	// (aligned with KubernetesRuntimeInfraOKE).
	PulumiWorkspace

	// Kubernetes minor version of the GKE cluster, such as "1.36".  If empty,
	// the default version is used.  GKE creates the control plane on the
	// release its channel offers for that minor version, and the nodes on the
	// control plane's.
	Version string

	// The GKE release channel the cluster is enrolled in, such as "STABLE".
	// If empty, the default channel is used.
	ReleaseChannel string

	// The days the cluster's maintenance window recurs on, in UTC, such as
	// "SA,SU".  If empty, the default days are used.
	MaintenanceWindowDays string

	// The time of day the maintenance window starts, in UTC, such as "04:00".
	// If empty, the default start is used.
	MaintenanceWindowStart string

	// How many hours the maintenance window lasts.  If zero, the default
	// duration is used.
	MaintenanceWindowHours int

	// The Google Cloud project ID where the cluster infra is provisioned.
	ProjectID string

	// The Google Cloud region where the cluster infra is provisioned.
	Region string

	// The number of zones the cluster's nodes run in.  One makes a zonal
	// cluster: a single control plane replica, and its nodes in one zone of
	// the region.  More make a regional cluster: its control plane replicated
	// across the region's zones, and its nodes in that many of them.  With it
	// set, MinNodeCount and MaxNodeCount limit the whole node pool, and
	// WorkerNodeInitialCount is spread across the zones, rounded up to a
	// multiple of them.  Zero keeps the shape of the clusters created before
	// it was set: regional, with nodes in every zone of the region and the
	// node counts applied to each zone.
	ZoneCount int32

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// The GCP machine type for the worker node pool (e.g. "e2-standard-2").
	MachineType string

	// The minimum number of nodes the worker node pool's autoscaler will
	// scale down to.
	MinNodeCount int32

	// The maximum number of nodes the worker node pool's autoscaler will
	// scale up to.
	MaxNodeCount int32

	// How the worker node pool's autoscaler places nodes across zones:
	// "BALANCED" keeps the zones even, and "ANY", used when this is empty,
	// places nodes wherever capacity is.
	LocationPolicy string

	// The email address of the GCP service account created for Threeport.
	// This service account is used with Workload Identity to allow
	// Threeport controllers to manage GCP resources.
	ServiceAccountEmail string

	// ServiceAccountCredentials contains the JSON key for a GCP service account.
	// Used when running outside GCP (e.g., in AWS or on-prem) where Workload
	// Identity is not available. This is the contents of a service account key
	// file exported from GCP Console.
	ServiceAccountCredentials string

	// credentialsFilePath stores the path to a temporary file containing
	// the service account credentials. This is set internally when
	// ServiceAccountCredentials is used.
	credentialsFilePath string

	// ProvisionBackups makes Create provision a bucket in another region the
	// control plane's database is backed up to.
	ProvisionBackups bool

	// BackupRegion is the region of the backup bucket.  Empty uses the
	// default for the cluster's region.
	BackupRegion string

	// BackupBucket is the name of the backup bucket: set by Create when it
	// provisions one, and by a caller tearing down a control plane whose
	// bucket was recorded, so Delete removes the database's access to it.
	BackupBucket string

	// DeleteBackups makes Delete remove the backup bucket in BackupBucket and
	// every backup in it.  The bucket is kept by default, since its backups
	// may still be needed after the control plane is gone.
	DeleteBackups bool

	// stackMinMasterVersion is set by CreateInfra when the stack already holds
	// the cluster, to the minimum version the cluster was created with: ""
	// for one created before the version was set.  The program keeps it.
	stackMinMasterVersion *string

	// stackReleaseChannel is set by CreateInfra when the stack already holds
	// the cluster, to the release channel the cluster was created with: ""
	// for one created before the channel was set.  The program keeps it.
	stackReleaseChannel *string

	// stackLocation is set by CreateInfra when the stack already holds the
	// cluster, to the location it was created in: its region, or its zone.
	// The program keeps it, since changing a cluster's location replaces it.
	stackLocation *string

	// stackNodePool is set by CreateInfra when the stack already holds the
	// worker node pool, whose zones are then kept.  A cluster whose create
	// failed before its node pool was made has none, and the node pool still
	// gets the zones a new cluster's would.
	stackNodePool bool

	// stackPerZoneNodeLimits is set by CreateInfra when the stack already
	// holds a node pool whose autoscaler limits each zone, as the node pools
	// created before ZoneCount was set do.  The program keeps it so.
	stackPerZoneNodeLimits bool

	// nodesKeepDefaultServiceAccount is set by CreateInfra when the stack
	// already holds a node pool whose nodes run as the project's default
	// Compute Engine service account.  The program then leaves them on it,
	// since changing a node pool's service account replaces the node pool.
	nodesKeepDefaultServiceAccount bool

	// gcpProjectResolvedFromGcloudINI is true when loadGCPConfig set ProjectID from the active gcloud INI.
	gcpProjectResolvedFromGcloudINI bool
	// gcpRegionResolvedFromGcloudINI is true when loadGCPConfig set Region from the active gcloud INI.
	gcpRegionResolvedFromGcloudINI bool

	// PrivateApi turns off the Kubernetes API's public IP endpoint.  The
	// nodes and the networks connected to the cluster's network reach its
	// internal endpoint, and Threeport and tptctl reach its DNS endpoint, which
	// Google authorizes by IAM rather than by address.
	PrivateApi bool

	// The CIDR blocks allowed to reach the Kubernetes API: on its public
	// endpoint, or with PrivateApi, on its internal endpoint.  Empty leaves
	// the public endpoint open to every address, and the internal endpoint
	// to the cluster's own network.
	AllowedCidrs []string

	// The public addresses of the control plane that manages the cluster,
	// admitted along with AllowedCidrs on the public endpoint when those are
	// set.  A private cluster does not need them, since the control plane
	// reaches it through the DNS endpoint.
	ControlPlaneEgressCidrs []string

	// ReserveNatAddress gives the cluster's Cloud NAT a reserved address, so
	// that its outbound traffic comes from one address that does not change
	// and is recorded as the EgressCidrsOutput.  It is set for clusters
	// created with one and left off for clusters created before, whose NAT
	// keeps the addresses Google allocates.
	ReserveNatAddress bool
}

const (
	gkePulumiProjectName        = "gke"
	gkePulumiProjectDescription = "Google Kubernetes Engine (GKE) cluster for Threeport"

	// gkeNodeServiceAccountRole is the only role granted to the service
	// account a cluster's nodes run as.  Google provides it for that account:
	// it lets the nodes write logs and metrics.
	gkeNodeServiceAccountRole = "roles/container.defaultNodeServiceAccount"
)

// InitPulumiProject fills in the Pulumi project identity and stack config, and
// returns the receiver so a caller can set it at construction.
//
// It has to be called before the infra is handed to the lifecycle, not from
// CreateInfra. executeInfraCreate starts the fsnotify watcher that streams
// partial state to the API before it calls DeployInfra, and the watched path is
// built from ProjectName: setting it later means the watcher reads an empty
// name, watches one directory above where Pulumi writes, and never fires - so a
// controller that crashes mid-provision has persisted nothing and a retry
// starts over, leaving the first attempt's resources behind. Setting it here
// also keeps the two goroutines from racing on the same field.
func (i *KubernetesRuntimeInfraGKE) InitPulumiProject() *KubernetesRuntimeInfraGKE {
	if i.ProjectName == "" {
		i.ProjectName = gkePulumiProjectName
	}
	if i.ProjectDescription == "" {
		i.ProjectDescription = gkePulumiProjectDescription
	}
	i.StackConfigs = map[string]string{
		"gcp:project": i.ProjectID,
		"gcp:region":  i.Region,
	}

	return i
}

// Create installs a Kubernetes cluster using Google Cloud GKE for threeport workloads.
func (i *KubernetesRuntimeInfraGKE) Create() (*kube.KubeConnectionInfo, error) {
	if _, err := i.kubeApiAllowlist(); err != nil {
		return nil, err
	}

	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	// load GCP configuration to ensure ProjectID is set
	if err := i.loadGCPConfig(); err != nil {
		return nil, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	// Only compare ADC identity to gcloud core.account when project or region came from the
	// gcloud profile; explicit flags/env avoid implying that profile's user context.
	if i.gcpProjectResolvedFromGcloudINI || i.gcpRegionResolvedFromGcloudINI {
		if err := validateADCUserMatchesGcloudAccount(context.Background()); err != nil {
			return nil, err
		}
	}

	// refuse a backup region that would not survive the loss of the
	// cluster's region before anything is created
	if i.ProvisionBackups {
		if _, err := GkeBackupRegion(i.Region, i.BackupRegion); err != nil {
			return nil, err
		}
	}

	// create GCP service account for Threeport to manage GCP resources
	// This service account will be used with Workload Identity
	if err := i.createGCPServiceAccountAndCredentials(); err != nil {
		return nil, fmt.Errorf("failed to create GCP service account: %w", err)
	}

	connection, err := i.CreateInfra()
	if err != nil {
		return nil, err
	}

	// provision the database backups once the cluster, and with it the
	// workload identity pool the backup binding names, exists
	if i.ProvisionBackups {
		if err := i.provisionBackups(); err != nil {
			return nil, fmt.Errorf("failed to provision database backups: %w", err)
		}
	}

	return connection, nil
}

// CreateInfra provisions GKE cluster infrastructure via Pulumi (network, cluster, node pool)
// and configures Workload Identity. It does not create the Threeport GCP service account —
// call createGCPServiceAccountAndCredentials first for the bootstrap path, or ensure
// credentials and Workload Identity are already configured for the controller path.
func (i *KubernetesRuntimeInfraGKE) CreateInfra() (*kube.KubeConnectionInfo, error) {
	if _, err := i.kubeApiAllowlist(); err != nil {
		return nil, err
	}

	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return nil, fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	// a create resumed on a stack that holds the cluster keeps its version,
	// release channel, location and node limits, and one whose node pool was
	// created before the nodes had their own service account leaves that node
	// pool as it is
	ctx := context.Background()
	exported, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the stack's GKE cluster: %w", err)
	}
	i.keepStackMinMasterVersion(exported.Deployment)
	i.keepStackReleaseChannel(exported.Deployment)
	i.keepStackShape(exported.Deployment)
	i.nodesKeepDefaultServiceAccount = nodePoolRunsAsDefaultServiceAccount(exported.Deployment)

	if _, err = i.RunUp(ctx, stack); err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	// configure Workload Identity binding after cluster is created
	// This allows Kubernetes service accounts to impersonate the GCP service account
	if err := i.configureWorkloadIdentityBindingPostCreate(); err != nil {
		return nil, fmt.Errorf("failed to configure Workload Identity binding: %w", err)
	}

	// let the control plane's controllers reach a private cluster through its
	// DNS endpoint, once the cluster, and with it the workload identity pool
	// the grant names, exists
	if err := i.grantControlPlaneClusterConnect(); err != nil {
		return nil, err
	}

	return i.GetConnection()
}

// DeployInfra runs CreateInfra and discards connection info. It satisfies InfraProvider
// for the shared infrastructure lifecycle (same pattern as KubernetesRuntimeInfraOKE).
func (i *KubernetesRuntimeInfraGKE) DeployInfra() error {
	_, err := i.CreateInfra()
	return err
}

// DestroyInfra tears down the GKE infrastructure. It satisfies InfraProvider.
//
// It is the controller's delete, and the controller path never creates the GCP
// service account (CreateInfra skips it), so there is none of this runtime's to
// clean up. An account found under the runtime's name belongs to something else -
// a GcpProvider of the same name, for one - and removing it would cut that
// provider off. Delete is the bootstrap path that also cleans up the account.
func (i *KubernetesRuntimeInfraGKE) DestroyInfra() error {
	if err := i.destroyInfraStack(); err != nil {
		return err
	}

	// the control plane's connect role, unless another private cluster
	// Threeport provisioned is still in the project.  A failure is returned
	// so the delete is retried; destroying a stack already gone does nothing.
	return i.revokeControlPlaneClusterConnect()
}

// keepStackMinMasterVersion records the minimum version of the cluster the
// stack's deployment holds, if it holds one, for the program to keep.
func (i *KubernetesRuntimeInfraGKE) keepStackMinMasterVersion(deployment json.RawMessage) {
	if version, held := clusterInputInStack(deployment, "gcp:container/cluster:Cluster", "minMasterVersion"); held {
		i.stackMinMasterVersion = &version
	}
}

// minMasterVersion returns the minimum version the program asks for the
// control plane: the cluster's, when the stack already holds it, and the
// definition's or the default otherwise.  A cluster created before the version
// was set keeps having none.
func (i *KubernetesRuntimeInfraGKE) minMasterVersion() pulumi.StringPtrInput {
	if i.stackMinMasterVersion != nil {
		if *i.stackMinMasterVersion == "" {
			return nil
		}
		return pulumi.String(*i.stackMinMasterVersion)
	}

	return pulumi.String(v0.KubernetesVersionOrDefault(&i.Version))
}

// CreatedKubernetesVersion returns the Kubernetes minor version Create asked
// for the cluster it created or found in the stack, such as "1.36": "" for one
// created before the version was set, which GKE chose.
func (i *KubernetesRuntimeInfraGKE) CreatedKubernetesVersion() string {
	if i.stackMinMasterVersion != nil {
		return *i.stackMinMasterVersion
	}

	return v0.KubernetesVersionOrDefault(&i.Version)
}

// CreatedReleaseChannel returns the release channel of the cluster Create
// created or found in the stack: "" for one created before the channel was
// set, which GKE chose.
func (i *KubernetesRuntimeInfraGKE) CreatedReleaseChannel() string {
	if i.stackReleaseChannel != nil {
		return *i.stackReleaseChannel
	}

	return v0.GkeReleaseChannelOrDefault(&i.ReleaseChannel)
}

// keepStackReleaseChannel records the release channel of the cluster the
// stack's deployment holds, if it holds one, for the program to keep.
func (i *KubernetesRuntimeInfraGKE) keepStackReleaseChannel(deployment json.RawMessage) {
	if channel, held := clusterInputInStack(deployment, "gcp:container/cluster:Cluster", "releaseChannel.channel"); held {
		i.stackReleaseChannel = &channel
	}
}

// releaseChannel returns the release channel the program enrolls the cluster
// in: the cluster's, when the stack already holds it, and the definition's or
// the default otherwise.  GKE refuses to move a cluster to a slower channel
// than one offering its version, so a cluster created before the channel was
// set keeps the one GKE chose for it.
func (i *KubernetesRuntimeInfraGKE) releaseChannel() gkecontainer.ClusterReleaseChannelPtrInput {
	channel := i.CreatedReleaseChannel()
	if channel == "" {
		return nil
	}

	return &gkecontainer.ClusterReleaseChannelArgs{Channel: pulumi.String(channel)}
}

// maintenanceWindowAnchor is the date the maintenance window's recurrence
// starts from.  A fixed date in the past keeps the window the program asks for
// the same on every run.
var maintenanceWindowAnchor = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// maintenancePolicy returns the cluster's maintenance window: each of its
// days from its start, for its hours, in UTC.  GKE upgrades the control plane
// and nodes only in that window, apart from an upgrade it can't defer.
func (i *KubernetesRuntimeInfraGKE) maintenancePolicy() (*gkecontainer.ClusterMaintenancePolicyArgs, error) {
	start, err := time.Parse("15:04", v0.GkeMaintenanceWindowStartOrDefault(&i.MaintenanceWindowStart))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the maintenance window's start: %w", err)
	}
	startTime := maintenanceWindowAnchor.Add(time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute)
	endTime := startTime.Add(time.Duration(v0.GkeMaintenanceWindowHoursOrDefault(&i.MaintenanceWindowHours)) * time.Hour)

	return &gkecontainer.ClusterMaintenancePolicyArgs{
		RecurringWindow: &gkecontainer.ClusterMaintenancePolicyRecurringWindowArgs{
			StartTime:  pulumi.String(startTime.Format(time.RFC3339)),
			EndTime:    pulumi.String(endTime.Format(time.RFC3339)),
			Recurrence: pulumi.String("FREQ=WEEKLY;BYDAY=" + v0.GkeMaintenanceWindowDaysOrDefault(&i.MaintenanceWindowDays)),
		},
	}, nil
}

// keepStackShape records where the cluster the stack's deployment holds was
// created, and how its node pool's autoscaler limits the nodes, for the
// program to keep.
func (i *KubernetesRuntimeInfraGKE) keepStackShape(deployment json.RawMessage) {
	if location, held := clusterInputInStack(deployment, "gcp:container/cluster:Cluster", "location"); held && location != "" {
		i.stackLocation = &location
	}
	i.stackNodePool = stackHoldsResource(deployment, "gcp:container/nodePool:NodePool")
	i.stackPerZoneNodeLimits = stackInputSet(deployment, "gcp:container/nodePool:NodePool", "autoscaling.maxNodeCount")
}

// stackHoldsResource reports whether a stack's deployment holds a resource of
// a type.
func stackHoldsResource(deployment json.RawMessage, resourceType string) bool {
	var contents struct {
		Resources []struct {
			Type string `json:"type"`
		} `json:"resources"`
	}
	if len(deployment) == 0 || json.Unmarshal(deployment, &contents) != nil {
		return false
	}

	return slices.ContainsFunc(contents.Resources, func(resource struct {
		Type string `json:"type"`
	}) bool {
		return resource.Type == resourceType
	})
}

// stackInputSet reports whether the first resource of a type in a stack's
// deployment has an input set at a dotted path.
func stackInputSet(deployment json.RawMessage, resourceType, input string) bool {
	var contents struct {
		Resources []struct {
			Type   string         `json:"type"`
			Inputs map[string]any `json:"inputs"`
		} `json:"resources"`
	}
	if len(deployment) == 0 || json.Unmarshal(deployment, &contents) != nil {
		return false
	}
	for _, resource := range contents.Resources {
		if resource.Type != resourceType {
			continue
		}
		var value any = resource.Inputs
		for _, key := range strings.Split(input, ".") {
			object, _ := value.(map[string]any)
			value = object[key]
		}
		return value != nil
	}

	return false
}

// clusterLocation returns where the program creates the cluster and its node
// pool, and the zones the node pool's nodes run in when the cluster is
// regional and they don't run in every zone of the region.  A cluster the
// stack holds stays where it is, and so does a node pool the stack holds; a
// regional cluster whose node pool wasn't made yet, as when its create failed
// part way, gives its node pool the zones a new cluster's would.  Without a
// ZoneCount the cluster is regional with nodes in every zone, as clusters
// were before it was set.  Otherwise the zones are the region's first, by
// name, that are up.
func (i *KubernetesRuntimeInfraGKE) clusterLocation(
	ctx *pulumi.Context,
	gcpProvider *gcp.Provider,
) (string, []string, error) {
	if i.ZoneCount <= 0 {
		if i.stackLocation != nil {
			return *i.stackLocation, nil, nil
		}
		return i.Region, nil, nil
	}
	// a cluster in the stack stays where it is, and a node pool in the
	// stack keeps its zones; a zonal cluster's node pool is in its zone
	if i.stackLocation != nil && (i.stackNodePool || *i.stackLocation != i.Region) {
		return *i.stackLocation, nil, nil
	}

	available, err := compute.GetZones(ctx, &compute.GetZonesArgs{
		Project: &i.ProjectID,
		Region:  &i.Region,
		Status:  util.Ptr("UP"),
	}, pulumi.Provider(gcpProvider))
	if err != nil {
		return "", nil, fmt.Errorf("failed to list the zones of region %s: %w", i.Region, err)
	}
	names := slices.Clone(available.Names)
	slices.Sort(names)
	if len(names) == 0 {
		return "", nil, fmt.Errorf("region %s has no zone that is up", i.Region)
	}
	zones := names[:min(int(i.ZoneCount), len(names))]
	// a regional cluster in the stack whose node pool wasn't made yet stays
	// regional, and its node pool takes the zones
	if i.stackLocation != nil {
		return *i.stackLocation, zones, nil
	}
	if i.ZoneCount == 1 {
		return names[0], nil, nil
	}

	return i.Region, zones, nil
}

// nodeLocationsInput returns a node pool's zones as the program sets them:
// none when its nodes run in the cluster's own location.
func nodeLocationsInput(zones []string) pulumi.StringArrayInput {
	if len(zones) == 0 {
		return nil
	}

	return pulumi.ToStringArray(zones)
}

// perZoneNodeLimits reports whether the node counts apply to each zone, as on
// the clusters created before ZoneCount was set, rather than to the whole
// node pool.
func (i *KubernetesRuntimeInfraGKE) perZoneNodeLimits() bool {
	return i.ZoneCount <= 0 || i.stackPerZoneNodeLimits
}

// initialNodeCountPerZone returns the node pool's initial node count, which
// GKE applies to each of its zones: WorkerNodeInitialCount divided across the
// zones, rounded up, or WorkerNodeInitialCount itself where the node counts
// are per zone.  nodeZones is the number of zones the node pool's nodes were
// placed in, none for a zonal cluster's.
func (i *KubernetesRuntimeInfraGKE) initialNodeCountPerZone(nodeZones int) int {
	initial := int(i.WorkerNodeInitialCount)
	if i.perZoneNodeLimits() || nodeZones <= 1 {
		return initial
	}

	return (initial + nodeZones - 1) / nodeZones
}

// autoscaling returns the node pool autoscaler's limits: on the whole node
// pool, or on each zone where the node counts are per zone.  GKE takes one
// or the other, not both.
func (i *KubernetesRuntimeInfraGKE) autoscaling() *gkecontainer.NodePoolAutoscalingArgs {
	autoscaling := &gkecontainer.NodePoolAutoscalingArgs{
		LocationPolicy: pulumi.String(i.locationPolicy()),
	}
	if i.perZoneNodeLimits() {
		autoscaling.MinNodeCount = pulumi.Int(int(i.MinNodeCount))
		autoscaling.MaxNodeCount = pulumi.Int(int(i.MaxNodeCount))
	} else {
		autoscaling.TotalMinNodeCount = pulumi.Int(int(i.MinNodeCount))
		autoscaling.TotalMaxNodeCount = pulumi.Int(int(i.MaxNodeCount))
	}

	return autoscaling
}

// locationPolicy returns the node pool autoscaler's location policy.
func (i *KubernetesRuntimeInfraGKE) locationPolicy() string {
	if i.LocationPolicy == "" {
		return "ANY"
	}

	return i.LocationPolicy
}

// gkeNodePoolIgnoredChanges are the node pool properties Pulumi sets only when
// it creates the node pool. GKE's autoscaler changes the node count with the
// load, and an up on an existing stack runs after a refresh from GCP (see
// setupStack), so setting the initial count again would scale a busy cluster
// back down. Moving to InitialNodeCount instead would replace the node pool of
// every existing stack, since the provider replaces a node pool whose initial
// node count changes.
var gkeNodePoolIgnoredChanges = []string{"nodeCount"}

// pulumiProgram defines the Pulumi resources for the GKE stack.
func (i *KubernetesRuntimeInfraGKE) pulumiProgram() pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		// Pass ServiceAccountCredentials directly to the GCP provider when
		// available so this provider's calls always use the credentials tied
		// to this specific GcpProvider object, rather than falling back to
		// whatever ambient credentials the process happens to have valid
		// (e.g. a genesis control plane's own Workload Identity, which is
		// scoped to a different GCP project). When no explicit credentials
		// are supplied, Pulumi falls back to its own default ADC resolution,
		// matching prior behavior for that case.
		providerArgs := &gcp.ProviderArgs{
			Project: pulumi.String(i.ProjectID),
			Region:  pulumi.String(i.Region),
		}
		if i.ServiceAccountCredentials != "" {
			providerArgs.Credentials = pulumi.String(i.ServiceAccountCredentials)
		}
		gcpProvider, err := gcp.NewProvider(ctx, "gcp-provider", providerArgs)
		if err != nil {
			return fmt.Errorf("failed to create GCP provider: %w", err)
		}

		network, err := compute.NewNetwork(ctx, fmt.Sprintf("%s-vpc", i.RuntimeInstanceName), &compute.NetworkArgs{
			Name:                  pulumi.String(fmt.Sprintf("%s-vpc", i.RuntimeInstanceName)),
			AutoCreateSubnetworks: pulumi.Bool(false),
			Description:           pulumi.String(fmt.Sprintf("VPC network for Threeport GKE cluster %s", i.RuntimeInstanceName)),
		}, pulumi.Provider(gcpProvider))
		if err != nil {
			return fmt.Errorf("failed to create VPC network: %w", err)
		}

		subnet, err := compute.NewSubnetwork(ctx, fmt.Sprintf("%s-subnet", i.RuntimeInstanceName), &compute.SubnetworkArgs{
			Name:        pulumi.String(fmt.Sprintf("%s-subnet", i.RuntimeInstanceName)),
			IpCidrRange: pulumi.String("10.0.0.0/16"),
			Region:      pulumi.String(i.Region),
			Network:     network.ID(),
			SecondaryIpRanges: compute.SubnetworkSecondaryIpRangeArray{
				&compute.SubnetworkSecondaryIpRangeArgs{
					RangeName:   pulumi.String("pods"),
					IpCidrRange: pulumi.String("10.1.0.0/16"),
				},
				&compute.SubnetworkSecondaryIpRangeArgs{
					RangeName:   pulumi.String("services"),
					IpCidrRange: pulumi.String("10.2.0.0/20"),
				},
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return fmt.Errorf("failed to create subnet: %w", err)
		}

		router, err := compute.NewRouter(ctx, fmt.Sprintf("%s-router", i.RuntimeInstanceName), &compute.RouterArgs{
			Name:    pulumi.String(fmt.Sprintf("%s-router", i.RuntimeInstanceName)),
			Network: network.ID(),
			Region:  pulumi.String(i.Region),
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return fmt.Errorf("failed to create Cloud Router: %w", err)
		}

		// a cluster with a reserved NAT address sends its outbound traffic
		// from that one address, which is recorded so that clusters this one
		// manages can admit it; one at the default of 64 ports per VM serves
		// about 1,000 nodes.  Other clusters keep the addresses Google
		// allocates, which can change.
		natIpAllocateOption := pulumi.String("AUTO_ONLY")
		var natIps pulumi.StringArray
		var egressAddresses []pulumi.StringOutput
		if i.ReserveNatAddress {
			natAddressName := fmt.Sprintf("%s-nat-ip", i.RuntimeInstanceName)
			natAddress, err := compute.NewAddress(ctx, natAddressName, &compute.AddressArgs{
				Name:        pulumi.String(natAddressName),
				Region:      pulumi.String(i.Region),
				AddressType: pulumi.String("EXTERNAL"),
				Labels:      GcpLabelsInput(i.RuntimeInstanceName),
			}, pulumi.Provider(gcpProvider))
			if err != nil {
				return fmt.Errorf("failed to reserve the Cloud NAT address: %w", err)
			}
			natIpAllocateOption = pulumi.String("MANUAL_ONLY")
			natIps = pulumi.StringArray{natAddress.SelfLink}
			egressAddresses = append(egressAddresses, natAddress.Address)
		}

		_, err = compute.NewRouterNat(ctx, fmt.Sprintf("%s-nat", i.RuntimeInstanceName), &compute.RouterNatArgs{
			Name:                          pulumi.String(fmt.Sprintf("%s-nat", i.RuntimeInstanceName)),
			Router:                        router.Name,
			Region:                        pulumi.String(i.Region),
			NatIpAllocateOption:           natIpAllocateOption,
			NatIps:                        natIps,
			SourceSubnetworkIpRangesToNat: pulumi.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
			LogConfig: &compute.RouterNatLogConfigArgs{
				Enable: pulumi.Bool(true),
				Filter: pulumi.String("ERRORS_ONLY"),
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{router}))
		if err != nil {
			return fmt.Errorf("failed to create Cloud NAT: %w", err)
		}

		maintenancePolicy, err := i.maintenancePolicy()
		if err != nil {
			return err
		}
		masterAuthorizedNetworksConfig, err := i.masterAuthorizedNetworksConfig()
		if err != nil {
			return err
		}

		// with PrivateApi the public IP endpoint is off, and the DNS
		// endpoint is how Threeport and tptctl reach the cluster
		var controlPlaneEndpointsConfig gkecontainer.ClusterControlPlaneEndpointsConfigPtrInput
		if i.PrivateApi {
			controlPlaneEndpointsConfig = &gkecontainer.ClusterControlPlaneEndpointsConfigArgs{
				DnsEndpointConfig: &gkecontainer.ClusterControlPlaneEndpointsConfigDnsEndpointConfigArgs{
					AllowExternalTraffic: pulumi.Bool(true),
				},
			}
		}

		location, nodeLocations, err := i.clusterLocation(ctx, gcpProvider)
		if err != nil {
			return err
		}

		cluster, err := gkecontainer.NewCluster(ctx, i.RuntimeInstanceName, &gkecontainer.ClusterArgs{
			Name:           pulumi.String(i.RuntimeInstanceName),
			Location:       pulumi.String(location),
			Network:        network.Name,
			Subnetwork:     subnet.Name,
			ResourceLabels: GcpLabelsInput(i.RuntimeInstanceName),

			IpAllocationPolicy: &gkecontainer.ClusterIpAllocationPolicyArgs{
				ClusterSecondaryRangeName:  pulumi.String("pods"),
				ServicesSecondaryRangeName: pulumi.String("services"),
			},

			// a minimum: GKE upgrades the control plane past it on its own,
			// to its channel's releases, within the maintenance window
			MinMasterVersion:  i.minMasterVersion(),
			ReleaseChannel:    i.releaseChannel(),
			MaintenancePolicy: maintenancePolicy,

			InitialNodeCount:      pulumi.Int(1),
			RemoveDefaultNodePool: pulumi.Bool(true),

			PrivateClusterConfig: &gkecontainer.ClusterPrivateClusterConfigArgs{
				EnablePrivateNodes:    pulumi.Bool(true),
				EnablePrivateEndpoint: pulumi.Bool(i.PrivateApi),
				MasterIpv4CidrBlock:   pulumi.String("172.16.0.0/28"),
			},

			MasterAuthorizedNetworksConfig: masterAuthorizedNetworksConfig,
			ControlPlaneEndpointsConfig:    controlPlaneEndpointsConfig,

			NetworkPolicy: &gkecontainer.ClusterNetworkPolicyArgs{
				Enabled:  pulumi.Bool(true),
				Provider: pulumi.String("CALICO"),
			},

			WorkloadIdentityConfig: &gkecontainer.ClusterWorkloadIdentityConfigArgs{
				WorkloadPool: pulumi.String(fmt.Sprintf("%s.svc.id.goog", i.ProjectID)),
			},

			DeletionProtection: pulumi.Bool(false),
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{subnet}))
		if err != nil {
			return fmt.Errorf("failed to create GKE cluster: %w", err)
		}

		// the cloud-platform scope is kept: the node service account's role
		// is what limits the nodes
		nodeConfig := &gkecontainer.NodePoolNodeConfigArgs{
			MachineType: pulumi.String(i.MachineType),
			DiskSizeGb:  pulumi.Int(50),
			DiskType:    pulumi.String("pd-standard"),
			OauthScopes: pulumi.StringArray{
				pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
			},
			Labels: pulumi.StringMap{
				kube.ThreeportManagedByLabelKey: pulumi.String(kube.ThreeportManagedByLabelValue),
			},
			ResourceLabels: GcpLabelsInput(i.RuntimeInstanceName),
			WorkloadMetadataConfig: &gkecontainer.NodePoolNodeConfigWorkloadMetadataConfigArgs{
				Mode: pulumi.String("GKE_METADATA"),
			},
		}
		nodePoolDependencies := []pulumi.Resource{cluster}
		if !i.nodesKeepDefaultServiceAccount {
			nodeServiceAccount, nodeRole, err := i.nodeServiceAccount(ctx, gcpProvider)
			if err != nil {
				return err
			}
			nodeConfig.ServiceAccount = nodeServiceAccount.Email
			// the nodes can write logs and metrics from the start
			nodePoolDependencies = append(nodePoolDependencies, nodeRole)
		}

		_, err = gkecontainer.NewNodePool(ctx, i.workerNodePoolName(), &gkecontainer.NodePoolArgs{
			Name:          pulumi.String(i.workerNodePoolName()),
			Cluster:       cluster.Name,
			Location:      pulumi.String(location),
			NodeLocations: nodeLocationsInput(nodeLocations),

			// per zone, which a node pool's initial count always is
			NodeCount: pulumi.Int(i.initialNodeCountPerZone(len(nodeLocations))),

			NodeConfig: nodeConfig,

			Autoscaling: i.autoscaling(),

			Management: &gkecontainer.NodePoolManagementArgs{
				AutoRepair:  pulumi.Bool(true),
				AutoUpgrade: pulumi.Bool(true),
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn(nodePoolDependencies),
			pulumi.IgnoreChanges(gkeNodePoolIgnoredChanges))
		if err != nil {
			return fmt.Errorf("failed to create node pool: %w", err)
		}

		exportEgressCidrs(ctx, egressAddresses)

		return nil
	}
}

// nodeServiceAccountID returns the ID of the service account the cluster's
// nodes run as.  Its prefix is its own, so it never matches the ID of a
// Threeport account, which is derived from a runtime or provider name.
func (i *KubernetesRuntimeInfraGKE) nodeServiceAccountID() string {
	return formatPrefixedServiceAccountID(nodeServiceAccountIDPrefix, nodeServiceAccountIDNameLength, i.RuntimeInstanceName)
}

// nodeServiceAccount adds the service account the cluster's nodes run as to
// the stack, with gkeNodeServiceAccountRole on the project and nothing else.
// The account is the cluster's own, rather than the project's default Compute
// Engine account, which every VM in the project shares and which has the
// Editor role in older projects and no role at all where the organization
// policy iam.automaticIamGrantsForDefaultServiceAccounts is enforced.  Being
// in the stack, the account and its role are deleted with the cluster.
func (i *KubernetesRuntimeInfraGKE) nodeServiceAccount(
	ctx *pulumi.Context,
	gcpProvider *gcp.Provider,
) (*serviceaccount.Account, *projects.IAMMember, error) {
	account, err := serviceaccount.NewAccount(ctx, fmt.Sprintf("%s-node-sa", i.RuntimeInstanceName), &serviceaccount.AccountArgs{
		AccountId:   pulumi.String(i.nodeServiceAccountID()),
		Project:     pulumi.String(i.ProjectID),
		DisplayName: pulumi.String(fmt.Sprintf("Threeport GKE nodes of %s", i.RuntimeInstanceName)),
		Description: pulumi.String(fmt.Sprintf("Nodes of the Threeport GKE cluster %s run as this account", i.RuntimeInstanceName)),
	}, pulumi.Provider(gcpProvider))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create node service account: %w", err)
	}

	role, err := projects.NewIAMMember(ctx, fmt.Sprintf("%s-node-sa-role", i.RuntimeInstanceName), &projects.IAMMemberArgs{
		Project: pulumi.String(i.ProjectID),
		Role:    pulumi.String(gkeNodeServiceAccountRole),
		Member:  account.Member,
	}, pulumi.Provider(gcpProvider))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to grant the node service account its role: %w", err)
	}

	return account, role, nil
}

// nodePoolRunsAsDefaultServiceAccount reports whether an exported deployment
// holds a node pool created without a service account, whose nodes run as the
// project's default Compute Engine account.  A deployment that can't be read
// is taken to hold one, since a node pool that is there would otherwise be
// replaced.
func nodePoolRunsAsDefaultServiceAccount(deployment json.RawMessage) bool {
	var contents struct {
		Resources []struct {
			Type   string `json:"type"`
			Inputs struct {
				NodeConfig struct {
					ServiceAccount string `json:"serviceAccount"`
				} `json:"nodeConfig"`
			} `json:"inputs"`
		} `json:"resources"`
	}
	if len(deployment) == 0 {
		return false
	}
	if err := json.Unmarshal(deployment, &contents); err != nil {
		return true
	}

	for _, resource := range contents.Resources {
		if resource.Type == "gcp:container/nodePool:NodePool" && resource.Inputs.NodeConfig.ServiceAccount == "" {
			return true
		}
	}

	return false
}

// Delete deletes a GKE cluster and the threeport control plane with it, along
// with the GCP service account Create made for it. It is for the bootstrap path:
// DestroyInfra leaves the account alone because the controller never created one.
func (i *KubernetesRuntimeInfraGKE) Delete() error {
	if err := i.destroyInfraStack(); err != nil {
		return err
	}

	// A failed cleanup is returned, not logged: the caller removes the only
	// record of this instance once Delete succeeds, and what is left here is a
	// service account and project roles that nothing else will find to retry.
	// The stack is already gone by now, so the error says so - a retry has no
	// stack to load and must go straight to CleanUpGCPResources.
	if err := i.DeleteGCPResources(); err != nil {
		return &GcpCleanupError{
			ServiceAccountEmail: i.getServiceAccountEmail(),
			ProjectID:           i.ProjectID,
			Err:                 err,
		}
	}

	return nil
}

// destroyInfraStack destroys the Pulumi-managed GKE infrastructure and nothing
// else.
func (i *KubernetesRuntimeInfraGKE) destroyInfraStack() error {
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	if err := i.loadGCPConfig(); err != nil {
		return fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	if err := i.DestroyStack(); err != nil {
		return fmt.Errorf("failed to destroy Pulumi stack: %w", err)
	}

	return nil
}

// GcpCleanupError reports that a GKE cluster's infrastructure was destroyed but
// the GCP service account and project roles created for it could not be removed.
type GcpCleanupError struct {
	ServiceAccountEmail string
	ProjectID           string
	Err                 error
}

func (e *GcpCleanupError) Error() string {
	return fmt.Sprintf(
		"infrastructure destroyed, but failed to clean up GCP service account %s in project %s: %v",
		e.ServiceAccountEmail, e.ProjectID, e.Err,
	)
}

func (e *GcpCleanupError) Unwrap() error { return e.Err }

// CleanUpGCPResources removes the GCP service account and project roles for a
// runtime whose infrastructure is already destroyed. It authenticates as Delete
// does, and is safe to repeat: an account that is gone is not an error.
func (i *KubernetesRuntimeInfraGKE) CleanUpGCPResources() error {
	if i.ProjectID == "" {
		return errors.New("GCP project ID is required to clean up GCP resources")
	}
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	return i.DeleteGCPResources()
}

// DeleteGCPResources deletes the GCP service account Create made for this
// Threeport instance, and its project roles. It does so only if the account
// carries this runtime's own description - an account that is there but belongs
// to something else, such as a GcpProvider of the same name, is refused.
func (i *KubernetesRuntimeInfraGKE) DeleteGCPResources() error {
	ctx := context.Background()

	// Use ServiceAccountCredentials directly when available, for the same
	// reason as the Pulumi GCP provider and the Workload Identity IAM
	// client: cleanup must act as the service account tied to this
	// GcpProvider, not whatever ambient credentials the process happens to
	// have - otherwise, in a cross-project setup, this silently runs (and
	// fails, only as a logged warning) against the wrong project's identity.
	iamClientOpts := []option.ClientOption{option.WithScopes(iam.CloudPlatformScope)}
	crmClientOpts := []option.ClientOption{option.WithScopes(cloudresourcemanager.CloudPlatformScope)}
	if i.ServiceAccountCredentials != "" {
		iamClientOpts = append(iamClientOpts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
		crmClientOpts = append(crmClientOpts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}

	// Create IAM service client
	iamService, err := iam.NewService(ctx, iamClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	// Create Cloud Resource Manager service client for IAM bindings
	crmService, err := cloudresourcemanager.NewService(ctx, crmClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	if err := deleteOwnedServiceAccount(
		iamService,
		crmService,
		i.ProjectID,
		i.getServiceAccountEmail(),
		i.RuntimeInstanceName,
		runtimeServiceAccountOwnedBy,
	); err != nil {
		return err
	}

	// the control plane's connect role, unless another private cluster
	// Threeport provisioned is still in the project
	if err := i.revokeControlPlaneClusterConnect(); err != nil {
		return err
	}

	// Remove the database's access to its backup bucket, and the backups
	// when asked to
	if i.BackupBucket != "" {
		storageClientOpts := []option.ClientOption{option.WithScopes(storage.DevstorageFullControlScope)}
		if i.ServiceAccountCredentials != "" {
			storageClientOpts = append(storageClientOpts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
		}
		storageService, err := storage.NewService(ctx, storageClientOpts...)
		if err != nil {
			return fmt.Errorf("failed to create storage service client: %w", err)
		}
		if err := i.deleteBackupResources(storageService); err != nil {
			return fmt.Errorf("failed to clean up database backup resources: %w", err)
		}
	}

	return nil
}

// kubeApiAllowlist returns the blocks the cluster's authorized networks
// admit.  On the public endpoint these are AllowedCidrs and the managing
// control plane's addresses.  With PrivateApi they are AllowedCidrs alone,
// on the internal endpoint, which the control plane's public addresses could
// not reach anyway.  It fails when the list is longer than GKE admits.
func (i *KubernetesRuntimeInfraGKE) kubeApiAllowlist() ([]string, error) {
	if !i.PrivateApi {
		return kubeApiAllowlist(v0.KubernetesRuntimeInfraProviderGKE, i.AllowedCidrs, i.ControlPlaneEgressCidrs)
	}

	limit, _ := v0.AllowedCidrsLimit(v0.KubernetesRuntimeInfraProviderGKE, true)
	if len(i.AllowedCidrs) > limit {
		return nil, fmt.Errorf(
			"AllowedCidrs has %d entries, but gke admits at most %d on a private Kubernetes API",
			len(i.AllowedCidrs), limit,
		)
	}

	return i.AllowedCidrs, nil
}

// masterAuthorizedNetworksConfig returns the authorized networks that limit
// the Kubernetes API to kubeApiAllowlist.  With none to list, a public
// cluster gets none, which leaves its endpoint open to every address, and a
// private cluster, which GKE refuses without them, gets them enabled with
// none listed, so its internal endpoint admits only the cluster's own node,
// Pod and Service ranges.
// On a public cluster the nodes reach the API through the internal endpoint,
// which these do not limit.  On a private cluster they are enforced on the
// internal endpoint, which GKE does only when asked to; GKE still admits the
// cluster's own node, Pod and Service ranges there without them being listed.
// Access from Google Cloud's own public addresses is turned off, since it
// would let any VM in any project past the list.
func (i *KubernetesRuntimeInfraGKE) masterAuthorizedNetworksConfig() (gkecontainer.ClusterMasterAuthorizedNetworksConfigPtrInput, error) {
	allowlist, err := i.kubeApiAllowlist()
	if err != nil {
		return nil, err
	}
	if len(allowlist) == 0 {
		if !i.PrivateApi {
			return nil, nil
		}
		// GKE refuses a private endpoint without authorized networks, and
		// enforces them on it, as it is told here so a later up plans no
		// change.  With none listed, the internal endpoint admits the
		// cluster's own ranges, and the DNS endpoint everyone IAM lets in
		return &gkecontainer.ClusterMasterAuthorizedNetworksConfigArgs{
			GcpPublicCidrsAccessEnabled:       pulumi.Bool(false),
			PrivateEndpointEnforcementEnabled: pulumi.Bool(true),
		}, nil
	}

	cidrBlocks := gkecontainer.ClusterMasterAuthorizedNetworksConfigCidrBlockArray{}
	for _, cidr := range allowlist {
		cidrBlocks = append(cidrBlocks, &gkecontainer.ClusterMasterAuthorizedNetworksConfigCidrBlockArgs{
			CidrBlock: pulumi.String(cidr),
		})
	}

	config := &gkecontainer.ClusterMasterAuthorizedNetworksConfigArgs{
		CidrBlocks:                  cidrBlocks,
		GcpPublicCidrsAccessEnabled: pulumi.Bool(false),
	}
	if i.PrivateApi {
		config.PrivateEndpointEnforcementEnabled = pulumi.Bool(true)
	}

	return config, nil
}

// useGCPAuth puts GCP authentication and configuration in place for a call that
// reaches GCP directly. The returned function releases what was set up and must
// be called when the call is done.
func (i *KubernetesRuntimeInfraGKE) useGCPAuth() (func(), error) {
	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	cleanup := func() {}
	if i.ServiceAccountCredentials != "" {
		cleanup = gcpauth.CleanupGCPCredentials
	}

	// load GCP configuration from gcloud CLI config or environment variables
	if err := i.loadGCPConfig(); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	return cleanup, nil
}

// getCluster fetches the GKE cluster as GCP reports it right now.  The caller
// has put GCP authentication in place.
func (i *KubernetesRuntimeInfraGKE) getCluster(ctx context.Context) (*containerpb.Cluster, error) {
	// Use ServiceAccountCredentials directly when available, for the same
	// reason as the other GCP clients constructed in this file: this call
	// must act as the service account tied to this GcpProvider, not
	// whatever ambient credentials the process happens to have.
	clusterManagerClientOpts := []option.ClientOption{}
	if i.ServiceAccountCredentials != "" {
		clusterManagerClientOpts = append(clusterManagerClientOpts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}

	// create GKE cluster manager client
	clusterManagerClient, err := container.NewClusterManagerClient(ctx, clusterManagerClientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster manager client: %w", err)
	}
	defer clusterManagerClient.Close()

	return i.findCluster(ctx, clusterManagerClient)
}

// gkeClusterLister is the part of the GKE cluster manager client that finds
// a cluster.
type gkeClusterLister interface {
	ListClusters(ctx context.Context, req *containerpb.ListClustersRequest, opts ...gax.CallOption) (*containerpb.ListClustersResponse, error)
}

// findCluster returns the cluster named for this runtime in its region,
// wherever in the region it is: the region itself for a regional cluster, or
// one of its zones for a zonal one.  A cluster that isn't there is a NotFound
// error, and a listing GKE couldn't complete is an error of its own, so it
// isn't taken for a cluster that is gone.  Where more than one cluster in the
// region has the name, the one in the location the stack records is this
// runtime's, and without one it is an error rather than a guess.
func (i *KubernetesRuntimeInfraGKE) findCluster(ctx context.Context, client gkeClusterLister) (*containerpb.Cluster, error) {
	response, err := client.ListClusters(ctx, &containerpb.ListClustersRequest{
		Parent: fmt.Sprintf("projects/%s/locations/-", i.ProjectID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the project's GKE clusters: %w", err)
	}
	var matches []*containerpb.Cluster
	for _, cluster := range response.GetClusters() {
		if cluster.GetName() == i.RuntimeInstanceName && gkeLocationInRegion(cluster.GetLocation(), i.Region) {
			matches = append(matches, cluster)
		}
	}
	if _, err := completeClusterList(response); err != nil {
		return nil, err
	}

	switch len(matches) {
	case 0:
		return nil, status.Errorf(codes.NotFound, "GKE cluster %s not found in region %s", i.RuntimeInstanceName, i.Region)
	case 1:
		return matches[0], nil
	}
	// clusters in different zones of a region may share a name: the one the
	// stack created is the one in its location, and without the stack none
	// is taken for it
	locations := make([]string, 0, len(matches))
	for _, cluster := range matches {
		if i.stackLocation != nil && cluster.GetLocation() == *i.stackLocation {
			return cluster, nil
		}
		locations = append(locations, cluster.GetLocation())
	}

	return nil, fmt.Errorf(
		"found %d GKE clusters named %s in region %s, in %v, and can't tell which is this runtime's",
		len(matches), i.RuntimeInstanceName, i.Region, locations,
	)
}

// gkeLocationInRegion reports whether a GKE location is a region or one of
// its zones, whose names are the region's followed by a letter.
func gkeLocationInRegion(location, region string) bool {
	return location == region || strings.HasPrefix(location, region+"-")
}

// gkeClusterReady returns an error naming what keeps the cluster from being
// connected to yet.  GCP returns a cluster that is still being created with no
// endpoint, and an endpoint on its own does not make it usable.
func gkeClusterReady(cluster *containerpb.Cluster) error {
	if cluster.GetStatus() != containerpb.Cluster_RUNNING {
		return fmt.Errorf("cluster is %s, not RUNNING", cluster.GetStatus())
	}
	if cluster.GetEndpoint() == "" {
		return errors.New("cluster has no API endpoint yet")
	}
	if cluster.GetMasterAuth().GetClusterCaCertificate() == "" {
		return errors.New("cluster has no CA certificate yet")
	}

	return nil
}

// workerNodePoolName is the name of the node pool the stack creates for
// workloads. The create check looks for it by this name, so it is named once.
func (i *KubernetesRuntimeInfraGKE) workerNodePoolName() string {
	return fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)
}

// gkeWorkerPoolReady returns an error unless the named worker node pool exists
// and is RUNNING. Any other pool does not count: the cluster is created with a
// default pool that the stack removes before it creates the worker pool, so a
// running default pool says the create has not reached the workers yet.
func gkeWorkerPoolReady(cluster *containerpb.Cluster, workerPool string) error {
	for _, nodePool := range cluster.GetNodePools() {
		if nodePool.GetName() != workerPool {
			continue
		}
		if nodePool.GetStatus() != containerpb.NodePool_RUNNING {
			return fmt.Errorf("node pool %s is %s, not RUNNING", workerPool, nodePool.GetStatus())
		}
		return nil
	}

	return fmt.Errorf("worker node pool %s does not exist yet", workerPool)
}

// CreateComplete reports whether the whole create finished, as GCP sees it
// rather than as the streamed Pulumi state does.  The stack state is saved
// while the stack is still being built, so state alone only says that
// provisioning started.  The cluster and its worker node pool are running and the
// Workload Identity binding is in place only when CreateInfra has run to its
// last step.  A cluster GCP does not know yet is not complete, not an error.
func (i *KubernetesRuntimeInfraGKE) CreateComplete() (bool, error) {
	release, err := i.useGCPAuth()
	if err != nil {
		return false, err
	}
	defer release()

	ctx := context.Background()
	cluster, err := i.getCluster(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, err
	}
	if gkeClusterReady(cluster) != nil || gkeWorkerPoolReady(cluster, i.workerNodePoolName()) != nil {
		return false, nil
	}

	// The binding and the connect grant are made after the stack is up, in
	// that order, so they are checked last.  A create interrupted before
	// either is not complete, and is run again.
	clientOpts := []gcpoption.ClientOption{gcpoption.WithScopes(gcpiam.CloudPlatformScope)}
	if i.ServiceAccountCredentials != "" {
		clientOpts = append(clientOpts, gcpoption.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}
	iamService, err := gcpiam.NewService(ctx, clientOpts...)
	if err != nil {
		return false, fmt.Errorf("failed to create IAM service client: %w", err)
	}
	if bound, err := i.workloadIdentityBound(iamService); err != nil || !bound {
		return false, err
	}

	return i.controlPlaneClusterConnectGranted()
}

// GetConnection returns the connection information for the GKE cluster.  It
// fails while the cluster is not ready to be connected to, so that a caller
// never records the endpoint of a cluster that does not have one yet.
func (i *KubernetesRuntimeInfraGKE) GetConnection() (*kube.KubeConnectionInfo, error) {
	release, err := i.useGCPAuth()
	if err != nil {
		return nil, err
	}
	defer release()

	ctx := context.Background()
	cluster, err := i.getCluster(ctx)
	if err != nil {
		return nil, err
	}
	if err := gkeClusterReady(cluster); err != nil {
		return nil, fmt.Errorf("GKE cluster is not ready for connection: %w", err)
	}

	apiEndpoint, caCert, err := gkeKubeApiEndpoint(cluster)
	if err != nil {
		return nil, err
	}

	// get an access token for authentication; it lasts about an hour, so a
	// caller whose work can outlast it mints fresh ones with TokenGenerator
	tokenSource, err := i.tokenSource(ctx)
	if err != nil {
		return nil, err
	}

	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// create connection info
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:     apiEndpoint,
		CACertificate:   caCert,
		Token:           token.AccessToken,
		TokenExpiration: token.Expiry,
	}

	return kubeConnInfo, nil
}

// gkeHasPublicIpEndpoint reports whether a cluster's Kubernetes API has a
// public IP endpoint.  One created before the endpoints config, or without
// its public endpoint set, has one unless it is a private cluster.
func gkeHasPublicIpEndpoint(cluster *containerpb.Cluster) bool {
	ipEndpoints := cluster.GetControlPlaneEndpointsConfig().GetIpEndpointsConfig()

	return !cluster.GetPrivateClusterConfig().GetEnablePrivateEndpoint() &&
		(ipEndpoints == nil || ipEndpoints.EnablePublicEndpoint == nil || *ipEndpoints.EnablePublicEndpoint)
}

// gkeKubeApiEndpoint returns the URL the cluster's Kubernetes API is reached
// at and the CA certificate to verify it with.  That is the IP endpoint and
// the cluster's own CA, unless the cluster has no public IP endpoint: then
// it is the DNS endpoint, which Google serves with a publicly trusted
// certificate, so no CA is returned and the system's roots verify it.
func gkeKubeApiEndpoint(cluster *containerpb.Cluster) (string, string, error) {
	if gkeHasPublicIpEndpoint(cluster) {
		// GKE provides the endpoint without the https:// prefix
		caCert, err := base64.StdEncoding.DecodeString(cluster.GetMasterAuth().GetClusterCaCertificate())
		if err != nil {
			return "", "", fmt.Errorf("failed to decode CA certificate: %w", err)
		}
		return fmt.Sprintf("https://%s", cluster.GetEndpoint()), string(caCert), nil
	}

	dnsEndpoint := cluster.GetControlPlaneEndpointsConfig().GetDnsEndpointConfig()
	if dnsEndpoint.GetEndpoint() == "" || !dnsEndpoint.GetAllowExternalTraffic() {
		return "", "", fmt.Errorf(
			"GKE cluster %s has no public IP endpoint and no DNS endpoint that admits traffic from outside its network, so it cannot be reached",
			cluster.GetName(),
		)
	}

	return fmt.Sprintf("https://%s", dnsEndpoint.GetEndpoint()), "", nil
}

// ResourcesExist reports whether the GKE cluster for this instance exists in
// GCP. Only a NotFound response means it does not; any other failure, such as
// revoked credentials, is returned as an error so a delete is not confirmed on
// the strength of a lookup that could not be made.
func (i *KubernetesRuntimeInfraGKE) ResourcesExist() (bool, error) {
	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return false, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	if err := i.loadGCPConfig(); err != nil {
		return false, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	ctx := context.Background()

	// act as the service account tied to this GcpProvider, as GetConnection does
	clusterManagerClientOpts := []option.ClientOption{}
	if i.ServiceAccountCredentials != "" {
		clusterManagerClientOpts = append(clusterManagerClientOpts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}

	clusterManagerClient, err := container.NewClusterManagerClient(ctx, clusterManagerClientOpts...)
	if err != nil {
		return false, fmt.Errorf("failed to create cluster manager client: %w", err)
	}
	defer clusterManagerClient.Close()

	_, err = i.findCluster(ctx, clusterManagerClient)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to look up GKE cluster %s: %w", i.RuntimeInstanceName, err)
	}

	return true, nil
}

// tokenSource returns the source of the access tokens the cluster is
// reached with: the provider's service account credentials when it has them,
// and the caller's own application default credentials otherwise.  It
// refreshes each token as it nears its expiry.
func (i *KubernetesRuntimeInfraGKE) tokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
	if i.ServiceAccountCredentials != "" {
		// google.DefaultTokenSource has no credentials parameter, so explicit
		// credentials are threaded through google.CredentialsFromJSON
		// instead - same "explicit creds override, ambient is the fallback"
		// shape used for the other GCP clients in this file.
		creds, err := google.CredentialsFromJSON(ctx, []byte(i.ServiceAccountCredentials), cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("failed to load service account credentials: %w", err)
		}
		return creds.TokenSource, nil
	}

	tokenSource, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to get token source: %w", err)
	}

	return tokenSource, nil
}

// TokenGenerator returns a function that returns a current access token for
// the cluster on every call, for work that can outlast the one GetConnection
// returns, such as a restore.
func (i *KubernetesRuntimeInfraGKE) TokenGenerator() (func() (string, error), error) {
	tokenSource, err := i.tokenSource(context.Background())
	if err != nil {
		return nil, err
	}

	return func() (string, error) {
		token, err := tokenSource.Token()
		if err != nil {
			return "", fmt.Errorf("failed to get an access token for the GKE cluster: %w", err)
		}
		return token.AccessToken, nil
	}, nil
}

// LoadConfigFromStack loads the GCP project ID and region from the existing
// Pulumi stack configuration. This is useful when deleting a cluster where
// we only have the runtime instance name but need the project and region
// to connect to GCP.
func (i *KubernetesRuntimeInfraGKE) LoadConfigFromStack() error {
	workspace, err := i.initWorkspace(auto.Program(func(ctx *pulumi.Context) error { return nil }))
	if err != nil {
		return fmt.Errorf("failed to initialize Pulumi workspace: %w", err)
	}

	ctx := context.Background()
	stack, err := auto.SelectStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return fmt.Errorf("failed to select stack: %w", err)
	}

	projectConfig, err := stack.GetConfig(ctx, "gcp:project")
	if err != nil {
		return fmt.Errorf("failed to get gcp:project config: %w", err)
	}
	i.ProjectID = projectConfig.Value

	regionConfig, err := stack.GetConfig(ctx, "gcp:region")
	if err != nil {
		return fmt.Errorf("failed to get gcp:region config: %w", err)
	}
	i.Region = regionConfig.Value

	// the stack configs are derived from the two values just read, so they are
	// rebuilt rather than left holding the empty ones construction produced -
	// writing those back would clear the stack's real project and region
	i.InitPulumiProject()

	return nil
}

// GetStackState returns the state of the GKE stack as a JSON object.
func (i *KubernetesRuntimeInfraGKE) GetStackState() (*datatypes.JSON, error) {
	return i.PulumiWorkspace.GetStackState()
}

// SetStackState restores Pulumi state from a JSON object (export or checkpoint format).
func (i *KubernetesRuntimeInfraGKE) SetStackState(state *datatypes.JSON) error {
	return i.PulumiWorkspace.SetStackState(state)
}

// configureWorkloadIdentityBindingPostCreate sets up the Workload Identity binding
// after the GKE cluster has been created.
func (i *KubernetesRuntimeInfraGKE) configureWorkloadIdentityBindingPostCreate() error {
	ctx := context.Background()

	// Use ServiceAccountCredentials directly when available, for the same
	// reason as the Pulumi GCP provider above: this call must act as the
	// service account tied to this GcpProvider, not whatever ambient
	// credentials the process happens to have.
	clientOpts := []gcpoption.ClientOption{gcpoption.WithScopes(gcpiam.CloudPlatformScope)}
	if i.ServiceAccountCredentials != "" {
		clientOpts = append(clientOpts, gcpoption.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}

	// Create IAM service client
	iamService, err := gcpiam.NewService(ctx, clientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	return i.configureWorkloadIdentityBinding(iamService)
}

// loadGCPConfig populates ProjectID and Region when they are not already set.
// Priority for each field independently:
//  1. Caller-provided values (e.g. CLI flags) already on the receiver — never overwritten
//  2. Environment variables (only if that field is still empty)
//  3. Active gcloud configuration INI ([core] project, [compute] region)
func (i *KubernetesRuntimeInfraGKE) loadGCPConfig() error {
	i.gcpProjectResolvedFromGcloudINI = false
	i.gcpRegionResolvedFromGcloudINI = false

	if i.ProjectID == "" {
		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			projectID = os.Getenv("CLOUDSDK_CORE_PROJECT")
		}
		if projectID == "" {
			projectID = os.Getenv("GCLOUD_PROJECT")
		}
		if projectID != "" {
			i.ProjectID = projectID
		}
	}

	if i.Region == "" {
		region := os.Getenv("CLOUDSDK_COMPUTE_REGION")
		if region == "" {
			region = os.Getenv("GOOGLE_REGION")
		}
		if region != "" {
			i.Region = region
		}
	}

	if i.ProjectID != "" && i.Region != "" {
		i.InitPulumiProject()
		return nil
	}

	if err := i.loadGCPConfigFromFile(); err != nil {
		if i.ProjectID == "" {
			return fmt.Errorf("GCP project ID not found in environment variables or gcloud config: %w", err)
		}
		if i.Region == "" {
			return fmt.Errorf("GCP region not found in environment variables or gcloud config: %w", err)
		}
	}

	// the stack config is derived from the two values this resolves, so it is
	// rebuilt here rather than left holding whatever construction saw. A
	// caller that omits both GCP flags constructs with neither, and the stack
	// would record an empty project and region while the cluster is built in
	// the resolved ones - leaving a later delete to read nothing back and fall
	// through to whatever the environment says at that point, or to nothing.
	i.InitPulumiProject()

	return nil
}

// loadGCPConfigFromFile reads the gcloud CLI configuration files
// to get project ID and region settings.
func (i *KubernetesRuntimeInfraGKE) loadGCPConfigFromFile() error {
	cfg, err := loadActiveGcloudConfigINI()
	if err != nil {
		return err
	}

	// get project ID from [core] section
	if i.ProjectID == "" {
		projectID := cfg.Section("core").Key("project").String()
		if projectID != "" {
			i.ProjectID = projectID
			i.gcpProjectResolvedFromGcloudINI = true
		}
	}

	// get region from [compute] section
	if i.Region == "" {
		region := cfg.Section("compute").Key("region").String()
		if region != "" {
			i.Region = region
			i.gcpRegionResolvedFromGcloudINI = true
		}
	}

	return nil
}
