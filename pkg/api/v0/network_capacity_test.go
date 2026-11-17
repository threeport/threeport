package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAnEksNodesAddressesFollowItsInstanceType covers how many of its
// subnet's addresses a node takes: its pods, capped at 110 on fewer than 30
// vCPUs and 250 on more, and a network interface's addresses kept beyond
// them, up to every address its interfaces hold.
func TestAnEksNodesAddressesFollowItsInstanceType(t *testing.T) {
	for instanceType, want := range map[string]struct{ addresses, perZone int }{
		// 3 interfaces of 6: every address, 17 pods
		"t3.medium": {18, 909},
		// 4 of 30: 118 pods capped at 110, but only 120 addresses
		"m8i.xlarge": {120, 136},
		// 24 of 64, 192 vCPUs: 250 pods and an interface's 64
		"m8i.48xlarge": {314, 52},
		// 32 vCPUs: the cap of 250, not 110
		"m8i.8xlarge": {300, 54},
		// 8 of 30 on 16 vCPUs: 110 pods and 30
		"c8g.4xlarge": {140, 116},
	} {
		addresses, known := eksAddressesPerNode(instanceType)
		require.True(t, known, instanceType)
		assert.Equal(t, want.addresses, addresses, instanceType)
		perZone, _ := eksNodesPerZone(instanceType)
		assert.Equal(t, want.perZone, perZone, instanceType)
	}

	_, known := eksAddressesPerNode("m5.large")
	assert.False(t, known, "an instance type that isn't listed isn't known")
}

// TestEveryMappedAwsInstanceTypeHasItsNetwork covers the instance types node
// sizes and profiles map to: each has its network limits, so a kubernetes
// runtime definition's NodeMaximum is always checked on EKS.
func TestEveryMappedAwsInstanceTypeHasItsNetwork(t *testing.T) {
	for _, machineType := range *mapping.GetMachineTypeMap() {
		if machineType.AwsMachineType == "" {
			continue
		}
		_, known := eksInstanceNetworks[machineType.AwsMachineType]
		assert.True(t, known, "%s (%s %s)", machineType.AwsMachineType, machineType.NodeProfile, machineType.NodeSize)
	}
}

// TestADefinitionsMaximumMustFitItsNetwork covers an EKS or GKE definition
// whose maximum its cluster's network can't hold: refused on create and on
// an update that changes the node group, with what the network holds.
func TestADefinitionsMaximumMustFitItsNetwork(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	session := func() *gorm.DB { return db.Session(&gorm.Session{}) }
	// each definition is attached to a kubernetes runtime definition of its own
	runtimeDefinition := uint(0)
	nextRuntimeDefinition := func() *uint {
		runtimeDefinition++
		return util.Ptr(runtimeDefinition)
	}
	eks := func(name, instanceType string, zones, maximum int, perZone bool) *AwsEksKubernetesRuntimeDefinition {
		return &AwsEksKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr(name)}, ZoneCount: util.Ptr(zones), DefaultNodeGroupInstanceType: util.Ptr(instanceType),
			DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupInitialSize: util.Ptr(1),
			DefaultNodeGroupMaximumSize: util.Ptr(maximum), NodeGroupPerZone: util.Ptr(perZone), KubernetesRuntimeDefinitionID: nextRuntimeDefinition(),
		}
	}

	// one node group across two zones holds 52 m8i.48xlarge nodes in each
	assert.NoError(t, session().Create(eks("fits", "m8i.48xlarge", 2, 104, false)).Error)
	assert.ErrorContains(t, session().Create(eks("over", "m8i.48xlarge", 2, 105, false)).Error,
		"DefaultNodeGroupMaximumSize 105 is more than the 104 nodes the cluster's network holds: each zone's /18 private subnet has 16379 addresses for nodes and pods, and a m8i.48xlarge node takes up to 314 of them, so it holds 52 nodes, 104 across the 2 zones")
	// a node group per zone holds its maximum in its own zone
	assert.NoError(t, session().Create(eks("per-zone", "m8i.48xlarge", 3, 52, true)).Error)
	assert.ErrorContains(t, session().Create(eks("per-zone-over", "m8i.48xlarge", 3, 53, true)).Error,
		"DefaultNodeGroupMaximumSize 53 is more than the 52 nodes the cluster's network holds")
	// an instance type that isn't listed isn't checked
	assert.NoError(t, session().Create(eks("unknown", "m5.24xlarge", 2, 1000, false)).Error)

	// an update to a larger instance type is checked with the zones it keeps
	small := eks("small", "t3.medium", 2, 200, false)
	require.NoError(t, session().Create(small).Error)
	assert.ErrorContains(t, session().Model(small).Updates(&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupInstanceType: util.Ptr("m8i.48xlarge")}).Error,
		"DefaultNodeGroupMaximumSize 200 is more than the 104 nodes")
	assert.NoError(t, session().Model(small).Updates(&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupInstanceType: util.Ptr("m8i.xlarge")}).Error,
		"136 in each of 2 zones holds 200")
	perZone := eks("to-per-zone", "m8i.48xlarge", 2, 100, false)
	require.NoError(t, session().Create(perZone).Error)
	assert.ErrorContains(t, session().Model(perZone).Updates(&AwsEksKubernetesRuntimeDefinition{NodeGroupPerZone: util.Ptr(true)}).Error,
		"DefaultNodeGroupMaximumSize 100 is more than the 52 nodes", "each zone's node group would hold the maximum")

	// a GKE cluster's pod range holds 256 nodes, whatever its zones
	gke := func(name string, zones, maximum int) *GcpGkeKubernetesRuntimeDefinition {
		return &GcpGkeKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr(name)}, ZoneCount: util.Ptr(zones), DefaultNodeGroupInstanceType: util.Ptr("e2-standard-4"),
			DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupInitialSize: util.Ptr(3),
			DefaultNodeGroupMaximumSize: util.Ptr(maximum), KubernetesRuntimeDefinitionID: nextRuntimeDefinition(),
		}
	}
	assert.NoError(t, session().Create(gke("gke-fits", 3, 256)).Error)
	assert.ErrorContains(t, session().Create(gke("gke-over", 1, 257)).Error,
		"DefaultNodeGroupMaximumSize 257 is more than the 256 nodes the cluster's network holds: the cluster's pod range holds 256 nodes")
	// one from before zone counts limits each zone, not the cluster
	counts, ok := gkeNodeCounts(gke("gke-old", 0, 300))
	require.True(t, ok)
	counts.initial = nil
	assert.NoError(t, validateNodeCounts(counts))
}

// TestAKubernetesRuntimeDefinitionsNodeMaximumMustFitItsNetwork covers
// NodeMaximum against the network of the cluster the definition reconciler
// makes of a kubernetes runtime definition: on EKS, the instance type its
// NodeSize and NodeProfile map to, in one node group across three zones when
// highly available and two otherwise; on GKE the pod range; on OKE the node
// subnet.
func TestAKubernetesRuntimeDefinitionsNodeMaximumMustFitItsNetwork(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	create := func(name, provider, size string, highAvailability bool, maximum int) error {
		definition := newKRD(name, provider, nil, nil)
		definition.HighAvailability = util.Ptr(highAvailability)
		definition.NodeSize = util.Ptr(size)
		definition.NodeProfile = util.Ptr("Balanced")
		definition.NodeMaximum = util.Ptr(maximum)
		return db.Session(&gorm.Session{}).Create(definition).Error
	}

	// 9XLarge Balanced is m8i.48xlarge, 52 nodes in each zone
	assert.NoError(t, create("eks", KubernetesRuntimeInfraProviderEKS, "9XLarge", false, 104))
	assert.ErrorContains(t, create("eks-over", KubernetesRuntimeInfraProviderEKS, "9XLarge", false, 105),
		"NodeMaximum 105 is more than the 104 nodes the cluster's network holds")
	assert.NoError(t, create("eks-ha", KubernetesRuntimeInfraProviderEKS, "9XLarge", true, 156), "three zones hold 156")
	assert.ErrorContains(t, create("eks-ha-over", KubernetesRuntimeInfraProviderEKS, "9XLarge", true, 157), "the 156 nodes")

	assert.NoError(t, create("gke", KubernetesRuntimeInfraProviderGKE, "Large", true, 256))
	assert.ErrorContains(t, create("gke-over", KubernetesRuntimeInfraProviderGKE, "Large", false, 257),
		"NodeMaximum 257 is more than the 256 nodes the cluster's network holds: the cluster's pod range holds 256 nodes")

	assert.NoError(t, create("oke", KubernetesRuntimeInfraProviderOKE, "Large", false, 253))
	assert.ErrorContains(t, create("oke-over", KubernetesRuntimeInfraProviderOKE, "Large", false, 254),
		"NodeMaximum 254 is more than the 253 nodes the cluster's network holds: the cluster's /24 node subnet holds 253 nodes")

	// kind runs on the machine it's on, with no network to size
	assert.NoError(t, create("kind", KubernetesRuntimeInfraProviderKind, "Large", false, 1000))
}
