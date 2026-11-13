package provider

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"cloud.google.com/go/container/apiv1/containerpb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// gkeZoneMocks records the GKE program's cluster and node pool, and answers
// its zone lookup with the region's zones, out of order.
type gkeZoneMocks struct {
	mu        sync.Mutex
	resources map[string]map[string]any
	zoneCalls int
}

func (m *gkeZoneMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resources == nil {
		m.resources = map[string]map[string]any{}
	}
	m.resources[args.TypeToken] = args.Inputs.Mappable()

	return args.Name + "-id", args.Inputs, nil
}

func (m *gkeZoneMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	if args.Token != "gcp:compute/getZones:getZones" {
		return resource.PropertyMap{}, nil
	}
	m.mu.Lock()
	m.zoneCalls++
	m.mu.Unlock()

	return resource.NewPropertyMapFromMap(map[string]any{
		"names": []any{"us-central1-f", "us-central1-b", "us-central1-c", "us-central1-a"},
	}), nil
}

// runGkeZoneProgram runs the GKE program for infra and returns its cluster
// and node pool inputs.
func runGkeZoneProgram(t *testing.T, infra *KubernetesRuntimeInfraGKE) (*gkeZoneMocks, map[string]any, map[string]any) {
	t.Helper()
	mocks := &gkeZoneMocks{}
	require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))

	return mocks, mocks.resources[gkeClusterTypeToken], mocks.resources[gkeNodePoolTypeToken]
}

// gkeZoneInfra returns a GKE infra whose node pool starts with initial nodes
// and is limited to minimum and maximum, in zones zones.
func gkeZoneInfra(zones, initial, minimum, maximum int32) *KubernetesRuntimeInfraGKE {
	return &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:        PulumiWorkspace{RuntimeInstanceName: "gke-zones", ProjectName: "gke"},
		ProjectID:              "test-project",
		Region:                 "us-central1",
		ZoneCount:              zones,
		WorkerNodeInitialCount: initial,
		MinNodeCount:           minimum,
		MaxNodeCount:           maximum,
		MachineType:            "e2-standard-4",
	}
}

// TestAGkeClustersShapeFollowsItsZoneCount covers where the GKE program puts
// a new cluster and how it limits its nodes:
//   - one zone: a zonal cluster in the region's first zone that is up;
//   - three zones: a regional cluster, its nodes in the region's first three;
//   - no zone count: regional with nodes in every zone, as clusters were
//     before it was set, with the node counts applied to each zone.
//
// With a zone count the limits apply to the whole node pool, and the initial
// count, which GKE applies to each zone, is divided across the zones and
// rounded up.
func TestAGkeClustersShapeFollowsItsZoneCount(t *testing.T) {
	t.Run("zonal", func(t *testing.T) {
		_, cluster, nodePool := runGkeZoneProgram(t, gkeZoneInfra(1, 2, 0, 60))
		assert.Equal(t, "us-central1-a", cluster["location"])
		assert.Equal(t, "us-central1-a", nodePool["location"])
		assert.Nil(t, nodePool["nodeLocations"])
		assert.Equal(t, float64(2), nodePool["nodeCount"])
		assert.Equal(t, map[string]any{"totalMinNodeCount": float64(0), "totalMaxNodeCount": float64(60), "locationPolicy": "ANY"}, nodePool["autoscaling"])
	})
	t.Run("regional", func(t *testing.T) {
		_, cluster, nodePool := runGkeZoneProgram(t, gkeZoneInfra(3, 4, 1, 60))
		assert.Equal(t, "us-central1", cluster["location"])
		assert.Equal(t, "us-central1", nodePool["location"])
		assert.Equal(t, []any{"us-central1-a", "us-central1-b", "us-central1-c"}, nodePool["nodeLocations"])
		assert.Equal(t, float64(2), nodePool["nodeCount"], "4 nodes across 3 zones is 2 in each, rounded up")
		assert.Equal(t, map[string]any{"totalMinNodeCount": float64(1), "totalMaxNodeCount": float64(60), "locationPolicy": "ANY"}, nodePool["autoscaling"])
	})
	t.Run("more zones than the region has", func(t *testing.T) {
		_, _, nodePool := runGkeZoneProgram(t, gkeZoneInfra(6, 4, 1, 60))
		assert.Equal(t, []any{"us-central1-a", "us-central1-b", "us-central1-c", "us-central1-f"}, nodePool["nodeLocations"])
		assert.Equal(t, float64(1), nodePool["nodeCount"])
	})
	t.Run("from before zone counts", func(t *testing.T) {
		mocks, cluster, nodePool := runGkeZoneProgram(t, gkeZoneInfra(0, 2, 1, 10))
		assert.Equal(t, "us-central1", cluster["location"])
		assert.Nil(t, nodePool["nodeLocations"])
		assert.Equal(t, float64(2), nodePool["nodeCount"])
		assert.Equal(t, map[string]any{"minNodeCount": float64(1), "maxNodeCount": float64(10), "locationPolicy": "ANY"}, nodePool["autoscaling"])
		assert.Zero(t, mocks.zoneCalls, "the zones aren't looked up")
	})
}

// TestAGkeClusterInTheStackKeepsItsShape covers a create resumed on, or an
// up of, a stack that already holds the cluster: the cluster stays where it
// was created, since moving it replaces it, and a node pool whose limits
// apply to each zone keeps them so.
func TestAGkeClusterInTheStackKeepsItsShape(t *testing.T) {
	deployment := json.RawMessage(`{"resources": [
		{"type": "gcp:container/cluster:Cluster", "inputs": {"location": "us-central1"}},
		{"type": "gcp:container/nodePool:NodePool", "inputs": {"autoscaling": {"minNodeCount": 1, "maxNodeCount": 10}}}
	]}`)
	infra := gkeZoneInfra(1, 2, 1, 10)
	infra.keepStackShape(deployment)
	require.NotNil(t, infra.stackLocation)
	assert.Equal(t, "us-central1", *infra.stackLocation)
	assert.True(t, infra.stackPerZoneNodeLimits)

	mocks, cluster, nodePool := runGkeZoneProgram(t, infra)
	assert.Equal(t, "us-central1", cluster["location"], "a zone count of 1 doesn't move a regional cluster")
	assert.Nil(t, nodePool["nodeLocations"])
	assert.Equal(t, map[string]any{"minNodeCount": float64(1), "maxNodeCount": float64(10), "locationPolicy": "ANY"}, nodePool["autoscaling"])
	assert.Zero(t, mocks.zoneCalls)

	// a stack with whole-pool limits, or none, keeps nothing per zone
	infra = gkeZoneInfra(3, 2, 1, 10)
	infra.keepStackShape(json.RawMessage(`{"resources": [
		{"type": "gcp:container/cluster:Cluster", "inputs": {"location": "us-central1-b"}},
		{"type": "gcp:container/nodePool:NodePool", "inputs": {"autoscaling": {"totalMinNodeCount": 1, "totalMaxNodeCount": 10}}}
	]}`))
	assert.Equal(t, "us-central1-b", *infra.stackLocation)
	assert.False(t, infra.stackPerZoneNodeLimits)
	infra = gkeZoneInfra(3, 2, 1, 10)
	infra.keepStackShape(json.RawMessage(`{"resources": []}`))
	assert.Nil(t, infra.stackLocation)
	assert.False(t, infra.stackPerZoneNodeLimits)
}

// fakeGkeClusterLister answers a cluster listing with clusters, and the
// zones it couldn't reach.
type fakeGkeClusterLister struct {
	clusters []*containerpb.Cluster
	missing  []string
	parent   string
}

func (f *fakeGkeClusterLister) ListClusters(_ context.Context, req *containerpb.ListClustersRequest, _ ...gax.CallOption) (*containerpb.ListClustersResponse, error) {
	f.parent = req.GetParent()

	return &containerpb.ListClustersResponse{Clusters: f.clusters, MissingZones: f.missing}, nil
}

// TestAGkeClusterIsFoundWhereverItIsInItsRegion covers how a runtime's GKE
// cluster is found: by its name in its region, at the location its stack or
// resource inventory records when one does.  Without one, a cluster in the
// region itself, as every cluster was before zonal ones, or one in a zone
// carrying this runtime's Threeport labels, is it.  A cluster of the name in
// another region, or unlabelled in a zone, is someone else's, so it isn't
// taken for this runtime's after that one is gone.  One that isn't there is
// NotFound, a listing GKE couldn't complete is an error rather than a cluster
// that is gone, and more than one candidate is an error rather than a guess.
func TestAGkeClusterIsFoundWhereverItIsInItsRegion(t *testing.T) {
	ctx := context.Background()
	labelled := func(location string) *containerpb.Cluster {
		return &containerpb.Cluster{Name: "gke-zones", Location: location, ResourceLabels: GcpResourceLabels("gke-zones")}
	}
	elsewhere := labelled("us-east1-b")
	someoneElses := &containerpb.Cluster{Name: "gke-zones", Location: "us-central1-f"}
	anotherRuntimes := &containerpb.Cluster{Name: "gke-zones", Location: "us-central1-a", ResourceLabels: GcpResourceLabels("another-runtime")}

	infra := gkeZoneInfra(1, 2, 0, 60)
	for _, cluster := range []*containerpb.Cluster{{Name: "gke-zones", Location: "us-central1"}, labelled("us-central1-c")} {
		lister := &fakeGkeClusterLister{clusters: []*containerpb.Cluster{elsewhere, someoneElses, anotherRuntimes, cluster}}
		found, err := infra.findCluster(ctx, lister)
		require.NoError(t, err)
		assert.Equal(t, cluster.GetLocation(), found.GetLocation())
		assert.Equal(t, "projects/test-project/locations/-", lister.parent)
	}

	// this runtime's cluster is gone: someone else's of the name isn't it
	_, err := infra.findCluster(ctx, &fakeGkeClusterLister{clusters: []*containerpb.Cluster{elsewhere, someoneElses, anotherRuntimes}})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = infra.findCluster(ctx, &fakeGkeClusterLister{missing: []string{"us-central1-a"}})
	require.Error(t, err)
	assert.NotEqual(t, codes.NotFound, status.Code(err), "a partial listing isn't a cluster that is gone")

	// two candidates of the name in the region, and no recorded location
	twins := &fakeGkeClusterLister{clusters: []*containerpb.Cluster{labelled("us-central1-b"), labelled("us-central1-c")}}
	_, err = infra.findCluster(ctx, twins)
	assert.ErrorContains(t, err, "found 2 GKE clusters named gke-zones in region us-central1")
	assert.NotEqual(t, codes.NotFound, status.Code(err))

	// with a recorded location, only the cluster there is this runtime's,
	// labelled or not, even where GKE couldn't list other zones
	infra.stackLocation = util.Ptr("us-central1-c")
	found, err := infra.findCluster(ctx, twins)
	require.NoError(t, err)
	assert.Equal(t, "us-central1-c", found.GetLocation())
	found, err = infra.findCluster(ctx, &fakeGkeClusterLister{
		clusters: []*containerpb.Cluster{{Name: "gke-zones", Location: "us-central1-c"}}, missing: []string{"us-central1-a"},
	})
	require.NoError(t, err)
	assert.Equal(t, "us-central1-c", found.GetLocation())
	_, err = infra.findCluster(ctx, &fakeGkeClusterLister{clusters: []*containerpb.Cluster{labelled("us-central1-b")}})
	assert.Equal(t, codes.NotFound, status.Code(err), "a cluster of the name elsewhere in the region, after this one is gone")
	infra.stackLocation = nil

	assert.True(t, gkeLocationInRegion("us-central1", "us-central1"))
	assert.True(t, gkeLocationInRegion("us-central1-f", "us-central1"))
	assert.False(t, gkeLocationInRegion("us-central2-a", "us-central1"))
	assert.False(t, gkeLocationInRegion("us-central10", "us-central1"))
}

// TestAGkeCreateResumedWithoutItsNodePoolGivesItItsZones covers a create
// that made the cluster and failed before its node pool: the cluster stays
// where it is, and the node pool still gets the zones, the whole-pool limits
// and the divided initial count a new cluster's would.  A zonal cluster's
// node pool is in its zone.
func TestAGkeCreateResumedWithoutItsNodePoolGivesItItsZones(t *testing.T) {
	clusterOnly := func(location string) json.RawMessage {
		return json.RawMessage(`{"resources": [{"type": "gcp:container/cluster:Cluster", "inputs": {"location": "` + location + `"}}]}`)
	}

	regional := gkeZoneInfra(3, 3, 1, 60)
	regional.keepStackShape(clusterOnly("us-central1"))
	assert.False(t, regional.stackNodePool)
	_, cluster, nodePool := runGkeZoneProgram(t, regional)
	assert.Equal(t, "us-central1", cluster["location"])
	assert.Equal(t, []any{"us-central1-a", "us-central1-b", "us-central1-c"}, nodePool["nodeLocations"])
	assert.Equal(t, float64(1), nodePool["nodeCount"], "3 nodes across 3 zones, not 3 in each")
	assert.Equal(t, map[string]any{"totalMinNodeCount": float64(1), "totalMaxNodeCount": float64(60), "locationPolicy": "ANY"}, nodePool["autoscaling"])

	// a zone count of 1 doesn't move a regional cluster to a zone
	oneZone := gkeZoneInfra(1, 2, 1, 60)
	oneZone.keepStackShape(clusterOnly("us-central1"))
	_, cluster, nodePool = runGkeZoneProgram(t, oneZone)
	assert.Equal(t, "us-central1", cluster["location"])
	assert.Equal(t, []any{"us-central1-a"}, nodePool["nodeLocations"])

	zonal := gkeZoneInfra(1, 2, 1, 60)
	zonal.keepStackShape(clusterOnly("us-central1-c"))
	mocks, cluster, nodePool := runGkeZoneProgram(t, zonal)
	assert.Equal(t, "us-central1-c", cluster["location"])
	assert.Equal(t, "us-central1-c", nodePool["location"])
	assert.Nil(t, nodePool["nodeLocations"])
	assert.Zero(t, mocks.zoneCalls)
}

// TestAGkeRuntimesInventoryRecordsItsClustersLocation covers the location a
// runtime's resource inventory records for its cluster, in the export and
// checkpoint shapes a stack's state is kept in: the cluster is then found
// there.  An inventory without a cluster records nothing.
func TestAGkeRuntimesInventoryRecordsItsClustersLocation(t *testing.T) {
	resources := `[{"type": "gcp:container/cluster:Cluster", "inputs": {"location": "us-central1-c"}}]`
	for name, inventory := range map[string]string{
		"export":     `{"version": 3, "deployment": {"resources": ` + resources + `}}`,
		"checkpoint": `{"version": 3, "checkpoint": {"latest": {"resources": ` + resources + `}}}`,
		"deployment": `{"resources": ` + resources + `}`,
	} {
		infra := gkeZoneInfra(1, 2, 0, 60)
		state := datatypes.JSON(inventory)
		infra.KeepInventoryLocation(&state)
		require.NotNil(t, infra.stackLocation, name)
		assert.Equal(t, "us-central1-c", *infra.stackLocation, name)
	}

	for _, inventory := range []*datatypes.JSON{nil, util.Ptr(datatypes.JSON(`{}`)), util.Ptr(datatypes.JSON(`{"resources": []}`))} {
		infra := gkeZoneInfra(1, 2, 0, 60)
		infra.KeepInventoryLocation(inventory)
		assert.Nil(t, infra.stackLocation)
	}
}
