package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gkeNodePoolDisk returns the boot disk the program gives the node pool.
func gkeNodePoolDisk(t *testing.T, i *KubernetesRuntimeInfraGKE) (any, any) {
	t.Helper()
	nodePools := runGkeProgram(t, i).ofType(gkeNodePoolTypeToken)
	require.Len(t, nodePools, 1)
	nodeConfig := nodePools[0].inputs["nodeConfig"].(map[string]any)
	return nodeConfig["diskType"], nodeConfig["diskSizeGb"]
}

// TestANewGkeNodePoolGetsABalancedDisk covers the boot disk of a new node
// pool's nodes: a 100 GB balanced persistent disk.
func TestANewGkeNodePoolGetsABalancedDisk(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"}}
	i.keepStackNodePool(nil)

	diskType, diskSize := gkeNodePoolDisk(t, i)
	assert.Equal(t, "pd-balanced", diskType)
	assert.Equal(t, float64(100), diskSize)
}

// TestAGkeNodePoolKeepsItsDisk covers a stack that already holds the node
// pool: its nodes keep the disk they were created with, since a new disk type
// or size would recreate every one of them.
func TestAGkeNodePoolKeepsItsDisk(t *testing.T) {
	for name, tc := range map[string]struct {
		deployment string
		diskType   string
		diskSize   float64
	}{
		"a node pool on a standard disk": {
			deployment: `{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"diskType":"pd-standard","diskSizeGb":50}}}]}`,
			diskType:   "pd-standard", diskSize: 50,
		},
		"a node pool on a disk of its own": {
			deployment: `{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{}},{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"diskType":"pd-ssd","diskSizeGb":200}}}]}`,
			diskType:   "pd-ssd", diskSize: 200,
		},
		"a node pool that doesn't record its disk": {
			deployment: `{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"machineType":"e2-medium"}}}]}`,
			diskType:   "pd-standard", diskSize: 50,
		},
		"a deployment that can't be read": {deployment: `{"resources":`, diskType: "pd-standard", diskSize: 50},
		"no node pool yet": {
			deployment: `{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{"nodeConfig":{"diskType":"pd-standard","diskSizeGb":50}}}]}`,
			diskType:   "pd-balanced", diskSize: 100,
		},
		"an empty stack": {deployment: `{"manifest":{}}`, diskType: "pd-balanced", diskSize: 100},
	} {
		t.Run(name, func(t *testing.T) {
			i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"}}
			i.keepStackNodePool([]byte(tc.deployment))

			diskType, diskSize := gkeNodePoolDisk(t, i)
			assert.Equal(t, tc.diskType, diskType)
			assert.Equal(t, tc.diskSize, diskSize)
		})
	}
}

// TestKeepStackNodePoolKeepsTheServiceAccount covers keepStackNodePool
// keeping a node pool created on the default account on it, which every path
// that runs the program on an existing stack relies on.
func TestKeepStackNodePoolKeepsTheServiceAccount(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{}
	i.keepStackNodePool([]byte(`{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"diskType":"pd-standard"}}}]}`))
	assert.True(t, i.nodesKeepDefaultServiceAccount)

	i = &KubernetesRuntimeInfraGKE{}
	i.keepStackNodePool([]byte(`{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"serviceAccount":"n@p.iam.gserviceaccount.com"}}}]}`))
	assert.False(t, i.nodesKeepDefaultServiceAccount)
}
