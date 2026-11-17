package provider

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// gkeProgramClusterInputs runs the GKE program on infra and returns the inputs of
// the cluster it registers.
func gkeProgramClusterInputs(t *testing.T, infra *KubernetesRuntimeInfraGKE) map[string]any {
	t.Helper()

	infra.PulumiWorkspace = PulumiWorkspace{RuntimeInstanceName: "gke-upgrade-test", ProjectName: "gke"}
	infra.ProjectID = "test-project"
	infra.Region = "us-central1"
	infra.WorkerNodeInitialCount = 1
	mocks := &gkeRecordingMocks{}
	require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))
	for _, r := range mocks.resources {
		if r.typeToken == gkeClusterTypeToken {
			return r.inputs
		}
	}
	t.Fatal("the program registered no cluster")
	return nil
}

// TestTheGkeClusterIsEnrolledInItsReleaseChannel covers the release channel
// the GKE program enrolls the cluster in: the definition's, or REGULAR.
func TestTheGkeClusterIsEnrolledInItsReleaseChannel(t *testing.T) {
	for asked, want := range map[string]string{"STABLE": "STABLE", "REGULAR": "REGULAR", "": "REGULAR"} {
		inputs := gkeProgramClusterInputs(t, &KubernetesRuntimeInfraGKE{ReleaseChannel: asked})
		assert.Equal(t, map[string]any{"channel": want}, inputs["releaseChannel"], "asked for %q", asked)
	}
}

// TestTheGkeClusterUpgradesOnlyInItsMaintenanceWindow covers the maintenance
// window the GKE program sets: the definition's days, start and hours in UTC,
// or Saturday and Sunday from 04:00 to 12:00.
func TestTheGkeClusterUpgradesOnlyInItsMaintenanceWindow(t *testing.T) {
	window := func(infra *KubernetesRuntimeInfraGKE) any {
		policy, _ := gkeProgramClusterInputs(t, infra)["maintenancePolicy"].(map[string]any)
		return policy["recurringWindow"]
	}

	assert.Equal(t, map[string]any{
		"startTime":  "2026-01-01T04:00:00Z",
		"endTime":    "2026-01-01T12:00:00Z",
		"recurrence": "FREQ=WEEKLY;BYDAY=SA,SU",
	}, window(&KubernetesRuntimeInfraGKE{}), "the default")

	assert.Equal(t, map[string]any{
		"startTime":  "2026-01-01T22:30:00Z",
		"endTime":    "2026-01-02T04:30:00Z",
		"recurrence": "FREQ=WEEKLY;BYDAY=TU,TH",
	}, window(&KubernetesRuntimeInfraGKE{
		MaintenanceWindowDays:  "TU,TH",
		MaintenanceWindowStart: "22:30",
		MaintenanceWindowHours: 6,
	}), "a window past midnight ends the next day")

	infra := &KubernetesRuntimeInfraGKE{MaintenanceWindowStart: "4am"}
	infra.PulumiWorkspace = PulumiWorkspace{RuntimeInstanceName: "gke-upgrade-test", ProjectName: "gke"}
	err := pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", &gkeRecordingMocks{}))
	assert.ErrorContains(t, err, "failed to parse the maintenance window's start")
}

// TestAResumedGkeCreateKeepsTheClustersReleaseChannel covers the release
// channel the GKE program asks for on a stack that already holds the cluster:
// the one it was created with, and none for a cluster created before the
// channel was set, whose channel GKE chose and may not let it leave.
func TestAResumedGkeCreateKeepsTheClustersReleaseChannel(t *testing.T) {
	deployment := []byte(`{"resources":[
		{"type":"gcp:container/cluster:Cluster","inputs":{"releaseChannel":{"channel":"REGULAR"}}}
	]}`)
	channel, held := clusterInputInStack(deployment, "gcp:container/cluster:Cluster", "releaseChannel.channel")
	assert.True(t, held)
	assert.Equal(t, "REGULAR", channel)
	channel, held = clusterInputInStack([]byte(`{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{}}]}`),
		"gcp:container/cluster:Cluster", "releaseChannel.channel")
	assert.True(t, held, "a cluster created before the channel was set")
	assert.Empty(t, channel)

	infra := &KubernetesRuntimeInfraGKE{ReleaseChannel: "STABLE"}
	infra.keepStackReleaseChannel(deployment)
	assert.Equal(t, map[string]any{"channel": "REGULAR"}, gkeProgramClusterInputs(t, infra)["releaseChannel"])

	infra = &KubernetesRuntimeInfraGKE{ReleaseChannel: "STABLE"}
	infra.keepStackReleaseChannel([]byte(`{"resources":[]}`))
	assert.Nil(t, infra.stackReleaseChannel)
	assert.Equal(t, map[string]any{"channel": "STABLE"}, gkeProgramClusterInputs(t, infra)["releaseChannel"], "a new stack")

	infra = &KubernetesRuntimeInfraGKE{ReleaseChannel: "STABLE", stackReleaseChannel: util.Ptr("")}
	assert.NotContains(t, gkeProgramClusterInputs(t, infra), "releaseChannel")
}

// TestTheEksClusterIsUpgradedWhenStandardSupportEnds covers the EKS upgrade
// policy: AWS upgrades the control plane when its version leaves standard
// support, rather than keep it on extended support at a higher charge.
func TestTheEksClusterIsUpgradedWhenStandardSupportEnds(t *testing.T) {
	clusters := runEksPulumiProgram(t, testEksInfra(2)).byType(eksClusterTypeToken)
	require.Len(t, clusters, 1)
	assert.Equal(t, map[string]any{"supportType": "STANDARD"}, clusters[0].inputs["upgradePolicy"])
}

// TestAResumedEksCreateKeepsTheClustersUpgradePolicy covers the upgrade
// policy the EKS program asks for on a stack that already holds the cluster:
// the one it was created with, and none for a cluster created before the
// policy was set, which EKS put on extended support and may not let leave it.
func TestAResumedEksCreateKeepsTheClustersUpgradePolicy(t *testing.T) {
	upgradePolicy := func(deployment string) any {
		infra := testEksInfra(2)
		infra.keepStackSupportType([]byte(deployment))
		clusters := runEksPulumiProgram(t, infra).byType(eksClusterTypeToken)
		require.Len(t, clusters, 1)
		return clusters[0].inputs["upgradePolicy"]
	}

	assert.Equal(t, map[string]any{"supportType": "EXTENDED"}, upgradePolicy(
		`{"resources":[{"type":"aws:eks/cluster:Cluster","inputs":{"upgradePolicy":{"supportType":"EXTENDED"}}}]}`,
	))
	assert.Nil(t, upgradePolicy(`{"resources":[{"type":"aws:eks/cluster:Cluster","inputs":{"version":"1.32"}}]}`),
		"a cluster created before the policy was set")
	assert.Equal(t, map[string]any{"supportType": "STANDARD"}, upgradePolicy(`{"resources":[]}`), "a new stack")
}

// TestTheClusterCreateKeptIsReported covers what each provider reports it
// created the cluster with, for tptctl up to record: the version and channel
// asked for on a new stack, and the ones a resumed stack kept, none for a GKE
// cluster created before they were set.
func TestTheClusterCreateKeptIsReported(t *testing.T) {
	gke := &KubernetesRuntimeInfraGKE{Version: "1.36", ReleaseChannel: "STABLE"}
	assert.Equal(t, "1.36", gke.CreatedKubernetesVersion())
	assert.Equal(t, "STABLE", gke.CreatedReleaseChannel())
	assert.Equal(t, DefaultEksKubernetesVersion, (&KubernetesRuntimeInfraGKE{}).CreatedKubernetesVersion())
	assert.Equal(t, "REGULAR", (&KubernetesRuntimeInfraGKE{}).CreatedReleaseChannel())

	gke.keepStackMinMasterVersion([]byte(`{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{"minMasterVersion":"1.35","releaseChannel":{"channel":"REGULAR"}}}]}`))
	gke.keepStackReleaseChannel([]byte(`{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{"minMasterVersion":"1.35","releaseChannel":{"channel":"REGULAR"}}}]}`))
	assert.Equal(t, "1.35", gke.CreatedKubernetesVersion())
	assert.Equal(t, "REGULAR", gke.CreatedReleaseChannel())

	old := &KubernetesRuntimeInfraGKE{Version: "1.36", ReleaseChannel: "STABLE"}
	old.keepStackMinMasterVersion([]byte(`{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{}}]}`))
	old.keepStackReleaseChannel([]byte(`{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{}}]}`))
	assert.Empty(t, old.CreatedKubernetesVersion())
	assert.Empty(t, old.CreatedReleaseChannel())

	eks := &KubernetesRuntimeInfraEKS{KubernetesVersion: "1.36"}
	eks.keepStackVersion([]byte(`{"resources":[{"type":"aws:eks/cluster:Cluster","inputs":{"version":"1.32"}}]}`))
	assert.Equal(t, "1.32", eks.CreatedKubernetesVersion())

	oke := &KubernetesRuntimeInfraOKE{Version: "1.36"}
	assert.Equal(t, "1.36", oke.CreatedKubernetesVersion(), "before the release is resolved")
	oke.keepStackVersion([]byte(`{"resources":[{"type":"oci:ContainerEngine/cluster:Cluster","inputs":{"kubernetesVersion":"v1.35.10"}}]}`))
	assert.Equal(t, "1.35", oke.CreatedKubernetesVersion())
	assert.Empty(t, (&KubernetesRuntimeInfraOKE{Version: "garbage"}).CreatedKubernetesVersion())
}
