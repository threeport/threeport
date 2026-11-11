package provider

import (
	"net/netip"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestAClusterWithNoRangeKeepsItsNetwork covers a cluster created before
// ranges were allocated: its layout is the one every cluster used to get, so
// its stack plans no change to its network.
func TestAClusterWithNoRangeKeepsItsNetwork(t *testing.T) {
	eks, err := eksLayout("")
	require.NoError(t, err)
	assert.Equal(t, eksNetworkLayout{
		Vpc:            "10.0.0.0/16",
		PublicSubnets:  []string{"10.0.0.0/22", "10.0.8.0/22", "10.0.16.0/22"},
		PrivateSubnets: []string{"10.0.4.0/22", "10.0.12.0/22", "10.0.20.0/22"},
	}, eks, "no service range, which EKS chose")

	gke, err := gkeLayout("")
	require.NoError(t, err)
	assert.Equal(t, gkeNetworkLayout{Nodes: "10.0.0.0/16", Pods: "10.1.0.0/16", Services: "10.2.0.0/20"}, gke)

	oke, err := okeLayout("")
	require.NoError(t, err)
	assert.Equal(t, okeNetworkLayout{Vcn: "10.0.0.0/16", Public: "10.0.0.0/28", Private: "10.0.10.0/24", LoadBalancer: "10.0.20.0/24"}, oke)
}

// TestAClustersNetworkIsLaidOutInItsRange covers the layouts of a cluster
// with a range: every range is inside the block's first /16, and none
// overlaps another.
func TestAClustersNetworkIsLaidOutInItsRange(t *testing.T) {
	eks, err := eksLayout("10.16.0.0/16")
	require.NoError(t, err)
	assert.Equal(t, eksNetworkLayout{
		Vpc:            "10.16.0.0/16",
		PrivateSubnets: []string{"10.16.0.0/18", "10.16.64.0/18", "10.16.128.0/18"},
		PublicSubnets:  []string{"10.16.192.0/22", "10.16.196.0/22", "10.16.200.0/22"},
		ServiceCidr:    "172.20.0.0/16",
	}, eks)
	assert.Contains(t, v0.ClusterInternalCidrs, eks.ServiceCidr, "the pool leaves the service range out")
	assertDisjointInside(t, "10.16.0.0/16", append(append([]string{}, eks.PrivateSubnets...), eks.PublicSubnets...))

	gke, err := gkeLayout("10.17.0.0/16")
	require.NoError(t, err)
	assert.Equal(t, gkeNetworkLayout{Pods: "10.17.0.0/17", Nodes: "10.17.128.0/20", Services: "10.17.144.0/20"}, gke)
	assertDisjointInside(t, "10.17.0.0/16", []string{gke.Pods, gke.Nodes, gke.Services})
	assert.Equal(t, 128, 1<<(24-17), "a /24 of pods for each of 128 nodes")

	oke, err := okeLayout("10.18.0.0/16")
	require.NoError(t, err)
	assert.Equal(t, okeNetworkLayout{Vcn: "10.18.0.0/16", Public: "10.18.0.0/28", Private: "10.18.10.0/24", LoadBalancer: "10.18.20.0/24"}, oke)
	assertDisjointInside(t, "10.18.0.0/16", []string{oke.Public, oke.Private, oke.LoadBalancer})

	// a block larger than a /16 is laid out in its first /16
	wide, err := eksLayout("10.20.0.0/15")
	require.NoError(t, err)
	assert.Equal(t, "10.20.0.0/15", wide.Vpc)
	assert.Equal(t, "10.20.0.0/18", wide.PrivateSubnets[0])

	for _, bad := range []string{"10.16.1.0/16", "10.16.0.0/17", "10.96.0.0/16", "not-a-range"} {
		_, err := eksLayout(bad)
		assert.Error(t, err, bad)
		_, err = gkeLayout(bad)
		assert.Error(t, err, bad)
		_, err = okeLayout(bad)
		assert.Error(t, err, bad)
	}
}

// assertDisjointInside checks ranges are inside a block and don't overlap.
func assertDisjointInside(t *testing.T, block string, ranges []string) {
	t.Helper()
	outer := netip.MustParsePrefix(block)
	prefixes := make([]netip.Prefix, 0, len(ranges))
	for _, r := range ranges {
		prefix := netip.MustParsePrefix(r)
		assert.True(t, outer.Contains(prefix.Addr()) && prefix.Bits() >= outer.Bits(), "%s inside %s", r, block)
		for _, other := range prefixes {
			assert.False(t, other.Overlaps(prefix), "%s overlaps %s", r, other)
		}
		prefixes = append(prefixes, prefix)
	}
}

// TestTheEksProgramLaysOutItsRange covers the EKS program with and without a
// range: the VPC, subnets and service range of each.
func TestTheEksProgramLaysOutItsRange(t *testing.T) {
	infra := testEksInfra(3)
	infra.NetworkCidr = "10.16.0.0/16"
	mocks := runEksPulumiProgram(t, infra)

	vpcs := mocks.byType(eksVpcTypeToken)
	require.Len(t, vpcs, 1)
	assert.Equal(t, "10.16.0.0/16", vpcs[0].inputs["cidrBlock"])
	cidrs := map[string]any{}
	for _, subnet := range mocks.byType(eksSubnetTypeToken) {
		cidrs[subnet.name] = subnet.inputs["cidrBlock"]
	}
	assert.Equal(t, map[string]any{
		"eks-test-private-subnet-0": "10.16.0.0/18", "eks-test-private-subnet-1": "10.16.64.0/18", "eks-test-private-subnet-2": "10.16.128.0/18",
		"eks-test-public-subnet-0": "10.16.192.0/22", "eks-test-public-subnet-1": "10.16.196.0/22", "eks-test-public-subnet-2": "10.16.200.0/22",
	}, cidrs)
	clusters := mocks.byType(eksClusterTypeToken)
	require.Len(t, clusters, 1)
	assert.Equal(t, map[string]any{"serviceIpv4Cidr": "172.20.0.0/16"}, clusters[0].inputs["kubernetesNetworkConfig"])

	legacy := runEksPulumiProgram(t, testEksInfra(2))
	assert.Equal(t, "10.0.0.0/16", legacy.byType(eksVpcTypeToken)[0].inputs["cidrBlock"])
	assert.NotContains(t, legacy.byType(eksClusterTypeToken)[0].inputs, "kubernetesNetworkConfig",
		"setting it on a cluster created without it would replace the cluster")

	infra = testEksInfra(2)
	infra.NetworkCidr = "10.96.0.0/16"
	err := pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("eks", "test-stack", &eksRecordingMocks{}))
	assert.ErrorContains(t, err, "failed to lay out the cluster's network")
}

// TestTheGkeProgramLaysOutItsRange covers the GKE program with and without a
// range: its subnet's nodes, pods and services ranges.
func TestTheGkeProgramLaysOutItsRange(t *testing.T) {
	subnetRanges := func(block string) (any, any) {
		infra := &KubernetesRuntimeInfraGKE{
			PulumiWorkspace:        PulumiWorkspace{RuntimeInstanceName: "gke-network-test", ProjectName: "gke"},
			NetworkCidr:            block,
			ProjectID:              "test-project",
			Region:                 "us-central1",
			WorkerNodeInitialCount: 1,
		}
		mocks := &gkeRecordingMocks{}
		require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))
		for _, r := range mocks.resources {
			if r.typeToken == "gcp:compute/subnetwork:Subnetwork" {
				return r.inputs["ipCidrRange"], r.inputs["secondaryIpRanges"]
			}
		}
		t.Fatal("the program registered no subnet")
		return nil, nil
	}

	nodes, secondary := subnetRanges("10.17.0.0/16")
	assert.Equal(t, "10.17.128.0/20", nodes)
	assert.Equal(t, []any{
		map[string]any{"rangeName": "pods", "ipCidrRange": "10.17.0.0/17"},
		map[string]any{"rangeName": "services", "ipCidrRange": "10.17.144.0/20"},
	}, secondary)

	nodes, secondary = subnetRanges("")
	assert.Equal(t, "10.0.0.0/16", nodes)
	assert.Equal(t, []any{
		map[string]any{"rangeName": "pods", "ipCidrRange": "10.1.0.0/16"},
		map[string]any{"rangeName": "services", "ipCidrRange": "10.2.0.0/20"},
	}, secondary)
}

// TestAResumedCreateKeepsItsStacksNetwork covers a create resumed on a stack
// that already holds the cluster's network: it keeps the range the network
// was laid out in, none for one laid out before ranges were allocated, rather
// than the one asked for, which would replace it.
func TestAResumedCreateKeepsItsStacksNetwork(t *testing.T) {
	stack := func(resourceType, input, value string) []byte {
		return []byte(`{"resources":[{"type":"` + resourceType + `","inputs":{"` + input + `":"` + value + `"}}]}`)
	}

	for held, want := range map[string]string{"10.0.0.0/16": "", "10.30.0.0/16": "10.30.0.0/16"} {
		eks := &KubernetesRuntimeInfraEKS{NetworkCidr: "10.16.0.0/16"}
		eks.keepStackNetwork(stack("aws:ec2/vpc:Vpc", "cidrBlock", held))
		assert.Equal(t, want, eks.NetworkCidr, "EKS VPC %s", held)
	}
	eks := &KubernetesRuntimeInfraEKS{NetworkCidr: "10.16.0.0/16"}
	eks.keepStackNetwork([]byte(`{"resources":[]}`))
	assert.Equal(t, "10.16.0.0/16", eks.NetworkCidr, "a new stack lays out the range asked for")

	for held, want := range map[string]string{"10.0.0.0/16": "", "10.30.128.0/20": "10.30.0.0/16"} {
		gke := &KubernetesRuntimeInfraGKE{NetworkCidr: "10.16.0.0/16"}
		gke.keepStackNetwork(stack("gcp:compute/subnetwork:Subnetwork", "ipCidrRange", held))
		assert.Equal(t, want, gke.NetworkCidr, "GKE nodes %s", held)
	}
	gke := &KubernetesRuntimeInfraGKE{NetworkCidr: "10.16.0.0/16"}
	gke.keepStackNetwork([]byte(`{"resources":[]}`))
	assert.Equal(t, "10.16.0.0/16", gke.NetworkCidr)

	block, held := okeStackNetwork(stack("oci:Core/vcn:Vcn", "cidrBlock", "10.0.0.0/16"))
	assert.True(t, held)
	assert.Empty(t, block)
	block, held = okeStackNetwork(stack("oci:Core/vcn:Vcn", "cidrBlock", "10.30.0.0/16"))
	assert.True(t, held)
	assert.Equal(t, "10.30.0.0/16", block)
	_, held = okeStackNetwork([]byte(`{"resources":[]}`))
	assert.False(t, held)
	_, held = okeStackNetwork([]byte(`{"resources":`))
	assert.False(t, held, "an unreadable record")
}
