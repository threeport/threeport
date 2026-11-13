package provider

import (
	"sort"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAProductionEksClusterRunsANodeGroupPerZone covers the program with a
// node group per zone: three zones, chosen in order from those offering the
// instance type, and a node group in each with that zone's private subnet
// alone and the default node group's sizes.
func TestAProductionEksClusterRunsANodeGroupPerZone(t *testing.T) {
	infra := testEksInfra(3)
	infra.NodeGroupPerZone = true
	infra.DefaultNodeGroupInstanceType = "m8i.large"
	infra.DefaultNodeGroupInitialNodes = 1
	infra.DefaultNodeGroupMinNodes = 1
	infra.DefaultNodeGroupMaxNodes = 10
	mocks := runEksPulumiProgram(t, infra)

	zones := privateSubnetZones(mocks)
	assert.Equal(t, []any{"us-east-1a", "us-east-1c", "us-east-1d"}, zones,
		"us-east-1b doesn't offer the instance type")

	nodeGroups := mocks.byType(eksNodeGroupTypeToken)
	sort.Slice(nodeGroups, func(a, b int) bool { return nodeGroups[a].name < nodeGroups[b].name })
	require.Len(t, nodeGroups, 3)
	for zoneIndex, nodeGroup := range nodeGroups {
		name := []string{"eks-test-private-node-group-0", "eks-test-private-node-group-1", "eks-test-private-node-group-2"}[zoneIndex]
		assert.Equal(t, name, nodeGroup.name)
		assert.Equal(t, name, nodeGroup.inputs["nodeGroupName"])
		assert.Equal(t, []any{"m8i.large"}, nodeGroup.inputs["instanceTypes"])
		assert.Len(t, nodeGroup.inputs["subnetIds"], 1, "a node group in one zone")
		assert.Equal(t, map[string]any{"desiredSize": 1.0, "minSize": 1.0, "maxSize": 10.0}, nodeGroup.inputs["scalingConfig"])
	}
	subnetOf := func(i int) any { return nodeGroups[i].inputs["subnetIds"].([]any)[0] }
	assert.NotEqual(t, subnetOf(0), subnetOf(1))
	assert.NotEqual(t, subnetOf(1), subnetOf(2))
}

// TestADevelopmentEksClusterKeepsItsNodeGroup covers the program without a
// node group per zone: the one node group spanning the first zones, as
// clusters already created have, whatever zones offer the instance type.
func TestADevelopmentEksClusterKeepsItsNodeGroup(t *testing.T) {
	mocks := runEksPulumiProgram(t, testEksInfra(2))

	nodeGroups := mocks.byType(eksNodeGroupTypeToken)
	require.Len(t, nodeGroups, 1)
	assert.Equal(t, "eks-test-private-node-group", nodeGroups[0].name)
	assert.Len(t, nodeGroups[0].inputs["subnetIds"], 2)

	zones := privateSubnetZones(mocks)
	assert.Equal(t, []any{"us-east-1a", "us-east-1b"}, zones)
}

// TestAProductionEksClusterNeedsThreeZonesOfferingItsInstanceType covers a
// region with too few zones offering the instance type: the program fails,
// naming the type, before creating anything.
func TestAProductionEksClusterNeedsThreeZonesOfferingItsInstanceType(t *testing.T) {
	saved := testInstanceTypeZones
	testInstanceTypeZones = []string{"us-east-1a", "us-east-1c"}
	defer func() { testInstanceTypeZones = saved }()

	infra := testEksInfra(3)
	infra.NodeGroupPerZone = true
	infra.DefaultNodeGroupInstanceType = "m8i.large"
	mocks := &eksRecordingMocks{}
	err := pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("eks", "test-stack", mocks))
	assert.ErrorContains(t, err, "region us-east-1 has 2 availability zones offering m8i.large, 3 required")
	assert.Empty(t, mocks.byType(eksVpcTypeToken))
}

// privateSubnetZones returns the zones of the private subnets the program
// registered, in the order of the zones they were created for.
func privateSubnetZones(mocks *eksRecordingMocks) []any {
	subnets := mocks.byType(eksSubnetTypeToken)
	sort.Slice(subnets, func(a, b int) bool { return subnets[a].name < subnets[b].name })
	var zones []any
	for _, subnet := range subnets {
		if subnet.inputs["mapPublicIpOnLaunch"] != true {
			zones = append(zones, subnet.inputs["availabilityZone"])
		}
	}
	return zones
}
