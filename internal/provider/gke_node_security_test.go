package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	testSecretsKey  = "projects/p/locations/us-central1/keyRings/r/cryptoKeys/secrets"
	testBootDiskKey = "projects/p/locations/us-central1/keyRings/r/cryptoKeys/disks"
)

// gkeProgramClusterAndNodePool runs the program and returns the inputs of
// the cluster and the node pool it makes, with the node pool's name.
func gkeProgramClusterAndNodePool(t *testing.T, i *KubernetesRuntimeInfraGKE) (map[string]any, map[string]any, string) {
	t.Helper()
	mocks := runGkeProgram(t, i)
	clusters := mocks.ofType(gkeClusterTypeToken)
	require.Len(t, clusters, 1)
	nodePools := mocks.ofType(gkeNodePoolTypeToken)
	require.Len(t, nodePools, 1)
	assert.Equal(t, nodePools[0].name, nodePools[0].inputs["name"], "the node pool is named in Pulumi as in GKE")

	return clusters[0].inputs, nodePools[0].inputs, nodePools[0].name
}

// TestAGkeRuntimeWithoutNodeSecurityIsTodays covers a cluster whose
// definition sets none of the encryption and boot settings: it is the
// cluster Threeport made before them, with no secrets key, intranode
// visibility or shielded instance config set, a node pool of the name every
// one had, and its nodes on a 100 GB balanced disk with no key of the caller's.
func TestAGkeRuntimeWithoutNodeSecurityIsTodays(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"}}
	cluster, nodePool, name := gkeProgramClusterAndNodePool(t, i)

	assert.NotContains(t, cluster, "databaseEncryption")
	assert.NotContains(t, cluster, "enableIntranodeVisibility")
	nodeConfig := nodePool["nodeConfig"].(map[string]any)
	assert.NotContains(t, nodeConfig, "bootDiskKmsKey")
	assert.NotContains(t, nodeConfig, "shieldedInstanceConfig")
	assert.Equal(t, "pd-balanced", nodeConfig["diskType"])
	assert.Equal(t, float64(100), nodeConfig["diskSizeGb"])
	assert.Equal(t, "threeport-test-nodepool", name)

	assert.Equal(t, GkeAppliedSettings{BootDiskType: "pd-balanced", BootDiskSizeGb: 100}, i.AppliedSettings())
}

// TestAGkeRuntimeGetsItsNodeSecurity covers a definition that sets each of
// the encryption and boot settings: the cluster encrypts its Secrets with the
// key and has intranode visibility, and the node pool's nodes boot with
// Secure Boot from a disk of the type and size asked for, encrypted with the
// key, in a node pool named for the key and Secure Boot.
func TestAGkeRuntimeGetsItsNodeSecurity(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:      PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		SecretsEncryptionKey: util.Ptr(testSecretsKey),
		BootDiskType:         util.Ptr("pd-ssd"),
		BootDiskSizeGb:       util.Ptr(200),
		BootDiskKmsKey:       util.Ptr(testBootDiskKey),
		SecureBoot:           util.Ptr(true),
		IntranodeVisibility:  util.Ptr(true),
	}
	cluster, nodePool, name := gkeProgramClusterAndNodePool(t, i)

	assert.Equal(t, map[string]any{"state": "ENCRYPTED", "keyName": testSecretsKey}, cluster["databaseEncryption"])
	assert.Equal(t, true, cluster["enableIntranodeVisibility"])
	nodeConfig := nodePool["nodeConfig"].(map[string]any)
	assert.Equal(t, testBootDiskKey, nodeConfig["bootDiskKmsKey"])
	assert.Equal(t, map[string]any{"enableSecureBoot": true, "enableIntegrityMonitoring": true}, nodeConfig["shieldedInstanceConfig"])
	assert.Equal(t, "pd-ssd", nodeConfig["diskType"])
	assert.Equal(t, float64(200), nodeConfig["diskSizeGb"])
	assert.Equal(t, i.workerNodePoolName(), name)
	assert.True(t, strings.HasPrefix(name, "threeport-test-nodepool-"), name)

	assert.Equal(t, GkeAppliedSettings{
		SecretsEncryptionKey: testSecretsKey,
		BootDiskType:         "pd-ssd",
		BootDiskSizeGb:       200,
		BootDiskKmsKey:       testBootDiskKey,
		SecureBoot:           true,
		IntranodeVisibility:  true,
	}, i.AppliedSettings())
}

// TestSettingsTurnedOffAreSetOff covers a definition that sets Secure Boot
// and intranode visibility off: both are set off, as GKE has them, so a
// cluster already there has nothing to change, and its node pool keeps the
// name it has.
func TestSettingsTurnedOffAreSetOff(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:     PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		SecureBoot:          util.Ptr(false),
		IntranodeVisibility: util.Ptr(false),
		BootDiskKmsKey:      util.Ptr(""),
	}
	cluster, nodePool, name := gkeProgramClusterAndNodePool(t, i)

	assert.Equal(t, false, cluster["enableIntranodeVisibility"])
	nodeConfig := nodePool["nodeConfig"].(map[string]any)
	assert.Equal(t, map[string]any{"enableSecureBoot": false, "enableIntegrityMonitoring": true}, nodeConfig["shieldedInstanceConfig"])
	assert.NotContains(t, nodeConfig, "bootDiskKmsKey")
	assert.Equal(t, "threeport-test-nodepool", name)
}

// TestAGkeNodePoolMovesToTheDiskItsDefinitionAsksFor covers a node pool the
// stack holds: its nodes keep their disk until the definition asks for
// another type or size, and then move onto it, keeping what it doesn't ask
// to change.
func TestAGkeNodePoolMovesToTheDiskItsDefinitionAsksFor(t *testing.T) {
	standard := []byte(`{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"diskType":"pd-standard","diskSizeGb":50}}}]}`)
	for name, tc := range map[string]struct {
		diskType *string
		diskSize *int
		want     gkeNodeDisk
	}{
		"nothing asked":   {want: gkeNodeDisk{Type: "pd-standard", SizeGb: 50}},
		"another type":    {diskType: util.Ptr("pd-ssd"), want: gkeNodeDisk{Type: "pd-ssd", SizeGb: 50}},
		"another size":    {diskSize: util.Ptr(200), want: gkeNodeDisk{Type: "pd-standard", SizeGb: 200}},
		"another of each": {diskType: util.Ptr("pd-balanced"), diskSize: util.Ptr(120), want: gkeNodeDisk{Type: "pd-balanced", SizeGb: 120}},
		"the one it has":  {diskType: util.Ptr("pd-standard"), diskSize: util.Ptr(50), want: gkeNodeDisk{Type: "pd-standard", SizeGb: 50}},
	} {
		t.Run(name, func(t *testing.T) {
			i := &KubernetesRuntimeInfraGKE{
				PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
				BootDiskType:    tc.diskType,
				BootDiskSizeGb:  tc.diskSize,
			}
			i.keepStackNodePool(standard)

			diskType, diskSize := gkeNodePoolDisk(t, i)
			assert.Equal(t, tc.want.Type, diskType)
			assert.Equal(t, float64(tc.want.SizeGb), diskSize)
			assert.Equal(t, tc.want.Type, i.AppliedSettings().BootDiskType)
			assert.Equal(t, tc.want.SizeGb, i.AppliedSettings().BootDiskSizeGb)
		})
	}
}

// TestAGkeNodePoolIsNamedForWhatItCantChange covers the node pool's name: the
// one every node pool had without a boot disk key or Secure Boot, and one of
// its own for each key and Secure Boot otherwise, the same each time, within
// the length GKE allows.
func TestAGkeNodePoolIsNamedForWhatItCantChange(t *testing.T) {
	name := func(runtime string, key *string, secureBoot *bool) string {
		i := &KubernetesRuntimeInfraGKE{
			PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: runtime},
			BootDiskKmsKey:  key,
			SecureBoot:      secureBoot,
		}
		return i.workerNodePoolName()
	}

	assert.Equal(t, "threeport-test-nodepool", name("threeport-test", nil, nil))
	assert.Equal(t, "threeport-test-nodepool", name("threeport-test", util.Ptr(""), util.Ptr(false)))

	names := map[string]bool{}
	for _, settings := range []struct {
		key        *string
		secureBoot *bool
	}{
		{key: util.Ptr(testBootDiskKey)},
		{key: util.Ptr(testSecretsKey)},
		{secureBoot: util.Ptr(true)},
		{key: util.Ptr(testBootDiskKey), secureBoot: util.Ptr(true)},
	} {
		named := name("threeport-test", settings.key, settings.secureBoot)
		assert.Equal(t, named, name("threeport-test", settings.key, settings.secureBoot), "the same each time")
		assert.Regexp(t, `^threeport-test-nodepool-[0-9a-f]{6}$`, named)
		names[named] = true
	}
	assert.Len(t, names, 4, "a name for each key and Secure Boot")

	long := name(strings.Repeat("r", 40), util.Ptr(testBootDiskKey), nil)
	assert.Len(t, long, gkeMaxNodePoolNameLength)
	assert.Regexp(t, `^r+-[0-9a-f]{6}$`, long)
}

// TestAGkeUpgradeReplacesANodePoolOfAnotherName covers the node pools a
// stack holds that the program no longer makes: one whose boot disk key or
// Secure Boot the definition changed, which the program replaces with one of
// another name.  The one it still makes isn't replaced.
func TestAGkeUpgradeReplacesANodePoolOfAnotherName(t *testing.T) {
	deployment := []byte(`{"resources":[
		{"urn":"urn:pulumi:s::gke::gcp:container/cluster:Cluster::threeport-test","type":"gcp:container/cluster:Cluster"},
		{"urn":"urn:pulumi:s::gke::gcp:container/nodePool:NodePool::threeport-test-nodepool","type":"gcp:container/nodePool:NodePool"}]}`)

	i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"}}
	assert.Empty(t, i.replacedNodePools(deployment), "the node pool the program makes")

	i.SecureBoot = util.Ptr(true)
	assert.Equal(t, []string{"urn:pulumi:s::gke::gcp:container/nodePool:NodePool::threeport-test-nodepool"}, i.replacedNodePools(deployment))
	assert.Equal(t, []string{"delete gcp:container/nodePool:NodePool threeport-test-nodepool"},
		nodePoolDeletions(i.replacedNodePools(deployment)), "as the upgrade's preview describes it")

	replaceable, err := i.replaceableNodePools(deployment)
	require.NoError(t, err)
	assert.Equal(t, i.replacedNodePools(deployment), replaceable)

	// while DeletionProtection is set the upgrade is refused before its
	// preview, which Pulumi fails on a protected node pool's deletion
	i.DeletionProtection = true
	replaceable, err = i.replaceableNodePools(deployment)
	assert.Empty(t, replaceable)
	var refused *UpgradeRefusedError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, []string{"delete gcp:container/nodePool:NodePool threeport-test-nodepool"}, refused.Steps)
	i.SecureBoot = nil
	replaceable, err = i.replaceableNodePools(deployment)
	assert.NoError(t, err, "nothing to replace")
	assert.Empty(t, replaceable)
	i.SecureBoot = util.Ptr(true)

	assert.Empty(t, i.replacedNodePools(nil))
	assert.Empty(t, i.replacedNodePools([]byte(`{"resources":`)))
}

// TestOnlyTheReplacedNodePoolLosesItsProtection covers what a node pool's
// replacement unprotects in the stack's state: the node pool replaced, and
// nothing else.
func TestOnlyTheReplacedNodePoolLosesItsProtection(t *testing.T) {
	deployment := []byte(`{"resources":[
		{"urn":"urn:c","type":"gcp:container/cluster:Cluster","protect":true},
		{"urn":"urn:old","type":"gcp:container/nodePool:NodePool","protect":true}]}`)

	unprotected, changed, err := unprotectDeploymentResources(deployment, func(urn string) bool { return urn == "urn:old" })
	require.NoError(t, err)
	assert.True(t, changed)
	assert.JSONEq(t, `{"resources":[
		{"urn":"urn:c","type":"gcp:container/cluster:Cluster","protect":true},
		{"urn":"urn:old","type":"gcp:container/nodePool:NodePool"}]}`, string(unprotected))

	_, changed, err = unprotectDeploymentResources(deployment, func(urn string) bool { return urn == "urn:none" })
	require.NoError(t, err)
	assert.False(t, changed)
}

// TestAnUpgradeMakesTheReplacementsItIsAllowed covers the steps an upgrade
// refuses: those its preview would make that it isn't allowed.
func TestAnUpgradeMakesTheReplacementsItIsAllowed(t *testing.T) {
	steps := []string{"delete gcp:container/nodePool:NodePool old", "replace gcp:container/cluster:Cluster c"}
	assert.Equal(t, []string{"replace gcp:container/cluster:Cluster c"},
		refusedSteps(steps, []string{"delete gcp:container/nodePool:NodePool old"}))
	assert.Equal(t, steps, refusedSteps(steps, nil))
	assert.Empty(t, refusedSteps(nil, nil))
}
