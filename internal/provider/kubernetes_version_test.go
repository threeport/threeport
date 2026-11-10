package provider

import (
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// eksEndOfStandardSupport is when each Kubernetes version Threeport creates
// EKS clusters on leaves EKS standard support, from
// `aws eks describe-cluster-versions --include-all`.  After it, a cluster
// runs in extended support, at a higher hourly charge, until AWS upgrades it.
var eksEndOfStandardSupport = map[string]time.Time{
	"1.35": time.Date(2027, time.March, 27, 0, 0, 0, 0, time.UTC),
	"1.36": time.Date(2027, time.August, 2, 0, 0, 0, 0, time.UTC),
}

// defaultVersionMinimumSupportLeft is how long the default version must have
// left in EKS standard support.
const defaultVersionMinimumSupportLeft = 6 * 30 * 24 * time.Hour

// TestNewEksClustersStartInStandardSupport fails when a version Threeport
// creates EKS clusters on has left standard support, or when the default has
// less than six months of it left.  Move SupportedKubernetesVersions and
// DefaultKubernetesVersion forward then, choosing from `aws eks
// describe-cluster-versions`, and check the version is offered in GKE's
// REGULAR and STABLE channels and as a production release on OKE.
func TestNewEksClustersStartInStandardSupport(t *testing.T) {
	now := time.Now()
	for _, version := range v0.SupportedKubernetesVersions {
		end, ok := eksEndOfStandardSupport[version]
		if !assert.True(t, ok, "no end of EKS standard support is recorded for %s", version) {
			continue
		}
		assert.True(t, now.Before(end), "Kubernetes %s left EKS standard support on %s", version, end.Format(time.DateOnly))
	}

	end := eksEndOfStandardSupport[v0.DefaultKubernetesVersion]
	assert.True(t, end.Sub(now) > defaultVersionMinimumSupportLeft,
		"the default Kubernetes version, %s, leaves EKS standard support on %s",
		v0.DefaultKubernetesVersion, end.Format(time.DateOnly),
	)
}

// TestTheEksClusterAndNodeGroupRunTheDefinitionsVersion covers the version
// the EKS program creates the cluster and its node group on.
func TestTheEksClusterAndNodeGroupRunTheDefinitionsVersion(t *testing.T) {
	infra := testEksInfra(2)
	infra.KubernetesVersion = "1.35"
	mocks := runEksPulumiProgram(t, infra)

	clusters := mocks.byType(eksClusterTypeToken)
	require.Len(t, clusters, 1)
	assert.Equal(t, "1.35", clusters[0].inputs["version"])
	nodeGroups := mocks.byType(eksNodeGroupTypeToken)
	require.Len(t, nodeGroups, 1)
	assert.Equal(t, "1.35", nodeGroups[0].inputs["version"])

	infra.KubernetesVersion = ""
	mocks = runEksPulumiProgram(t, infra)
	assert.Equal(t, v0.DefaultKubernetesVersion, mocks.byType(eksClusterTypeToken)[0].inputs["version"])
}

// TestTheGkeClusterStartsOnTheDefinitionsVersion covers the version the GKE
// program asks for: a minimum for the control plane, which GKE creates on its
// channel's release of that minor version and upgrades past on its own.
func TestTheGkeClusterStartsOnTheDefinitionsVersion(t *testing.T) {
	for asked, want := range map[string]string{"1.35": "1.35", "": v0.DefaultKubernetesVersion} {
		infra := &KubernetesRuntimeInfraGKE{
			PulumiWorkspace:        PulumiWorkspace{RuntimeInstanceName: "gke-version-test", ProjectName: "gke"},
			Version:                asked,
			ProjectID:              "test-project",
			Region:                 "us-central1",
			WorkerNodeInitialCount: 1,
		}
		mocks := &gkeRecordingMocks{}
		require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))

		var clusters []gkeRecordedResource
		for _, r := range mocks.resources {
			if r.typeToken == gkeClusterTypeToken {
				clusters = append(clusters, r)
			}
		}
		require.Len(t, clusters, 1)
		assert.Equal(t, want, clusters[0].inputs["minMasterVersion"], "asked for %q", asked)
	}
}

// TestAnOkeMinorVersionResolvesToItsNewestReleaseWithAnImage covers the OKE
// release a minor version resolves to: the newest OKE offers that has a
// worker node image for the node shape's architecture.
func TestAnOkeMinorVersionResolvesToItsNewestReleaseWithAnImage(t *testing.T) {
	offered := []string{"v1.34.10", "v1.35.2", "v1.36.1", "v1.36.4", "v1.36.10", "v1.37.0"}
	images := []string{
		"Oracle-Linux-8.10-2026.09.30-0-OKE-1.36.4-1001",
		"Oracle-Linux-8.10-aarch64-2026.09.30-0-OKE-1.36.4-1001",
		"Oracle-Linux-8.10-2026.09.30-0-OKE-1.36.10-1002",
		"Oracle-Linux-8.10-aarch64-2026.07.10-0-OKE-1.36.1-998",
		"Oracle-Linux-8.10-2026.05.01-0-OKE-1.35.2-990",
		"Oracle-Linux-8.10-Gen2-GPU-2026.10.20-0-OKE-1.37.0-1010",
	}

	version, err := newestOkePatch("1.36", offered, images, "x86_64")
	require.NoError(t, err)
	assert.Equal(t, "v1.36.10", version, "1.36.10 sorts after 1.36.4 by number, not by text")

	version, err = newestOkePatch("1.36", offered, images, "aarch64")
	require.NoError(t, err)
	assert.Equal(t, "v1.36.4", version, "the newest release has no Arm image yet")

	_, err = newestOkePatch("1.35", offered, images, "aarch64")
	assert.ErrorContains(t, err, "OKE has no aarch64 worker node image for any release of Kubernetes 1.35")

	_, err = newestOkePatch("1.32", offered, images, "x86_64")
	assert.ErrorContains(t, err, "OKE offers no release of Kubernetes 1.32 for new clusters")

	_, err = newestOkePatch("1.3", offered, images, "x86_64")
	assert.Error(t, err, "1.3 is not a prefix of 1.36")

	_, err = newestOkePatch("1.37", offered, images, "x86_64")
	assert.ErrorContains(t, err, "no x86_64 worker node image", "a GPU image needs a GPU shape")

	assert.Equal(t, "aarch64", okeImageArch("VM.Standard.A1.Flex"))
	assert.Equal(t, "x86_64", okeImageArch("VM.Standard.E4.Flex"))
	assert.Empty(t, okeImageFor(images, "v1.36.1", "x86_64"), "1.36.1 doesn't match an image of 1.36.10")
}

// TestAResumedOkeCreateKeepsItsClustersRelease covers a create resumed on a
// stack that holds the cluster: it keeps the release the cluster was created
// on rather than moving it to a newer one.
func TestAResumedOkeCreateKeepsItsClustersRelease(t *testing.T) {
	version, ok := okeClusterVersionInStack([]byte(`{"resources":[
		{"type":"oci:core/vcn:Vcn","inputs":{}},
		{"type":"oci:ContainerEngine/cluster:Cluster","inputs":{"kubernetesVersion":"v1.36.1"}}
	]}`))
	assert.True(t, ok)
	assert.Equal(t, "v1.36.1", version)

	for name, deployment := range map[string]string{
		"a new stack":          ``,
		"no cluster yet":       `{"resources":[{"type":"oci:core/vcn:Vcn","inputs":{}}]}`,
		"an unreadable record": `{"resources":`,
	} {
		_, ok := okeClusterVersionInStack([]byte(deployment))
		assert.False(t, ok, name)
	}
}
