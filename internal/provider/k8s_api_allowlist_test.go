package provider

import (
	"testing"

	"github.com/pulumi/pulumi-oci/sdk/v3/go/oci/core"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eksClusterVpcConfig returns the vpcConfig the EKS program gives the
// cluster.
func eksClusterVpcConfig(t *testing.T, infra *KubernetesRuntimeInfraEKS) map[string]any {
	t.Helper()
	clusters := runEksPulumiProgram(t, infra).byType(eksClusterTypeToken)
	require.Len(t, clusters, 1)
	vpcConfig, ok := clusters[0].inputs["vpcConfig"].(map[string]any)
	require.True(t, ok, "the cluster has a vpcConfig")
	return vpcConfig
}

// TestAnEksClusterAdmitsOnlyItsAllowedCidrs covers #729 on EKS: with
// AllowedCidrs the public endpoint admits only those blocks, while the
// private endpoint the nodes use stays on.  Without them the cluster is
// provisioned as before, with no list, which EKS reads as every address.
func TestAnEksClusterAdmitsOnlyItsAllowedCidrs(t *testing.T) {
	t.Run("with AllowedCidrs", func(t *testing.T) {
		infra := testEksInfra(2)
		infra.AllowedCidrs = []string{"203.0.113.0/24", "198.51.100.7/32"}

		vpcConfig := eksClusterVpcConfig(t, infra)
		assert.Equal(t, []any{"203.0.113.0/24", "198.51.100.7/32"}, vpcConfig["publicAccessCidrs"])
		assert.Equal(t, true, vpcConfig["endpointPublicAccess"])
		assert.Equal(t, true, vpcConfig["endpointPrivateAccess"])
	})
	t.Run("without", func(t *testing.T) {
		vpcConfig := eksClusterVpcConfig(t, testEksInfra(2))
		assert.NotContains(t, vpcConfig, "publicAccessCidrs")
		assert.Equal(t, true, vpcConfig["endpointPublicAccess"])
		assert.Equal(t, true, vpcConfig["endpointPrivateAccess"])
	})
}

// gkeClusterInputs returns the inputs the GKE program gives the cluster.
func gkeClusterInputs(t *testing.T, allowedCidrs []string) map[string]any {
	t.Helper()
	infra := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace: PulumiWorkspace{
			RuntimeInstanceName: "gke-allowlist-test",
			ProjectName:         "gke",
		},
		ProjectID:              "test-project",
		Region:                 "us-central1",
		WorkerNodeInitialCount: 1,
		AllowedCidrs:           allowedCidrs,
	}

	mocks := &gkeRecordingMocks{}
	require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))
	for _, recorded := range mocks.resources {
		if recorded.typeToken == gkeClusterTypeToken {
			return recorded.inputs
		}
	}
	t.Fatal("the program registered no cluster")
	return nil
}

// TestAGkeClusterAdmitsOnlyItsAllowedCidrs covers #729 on GKE: with
// AllowedCidrs the control plane's authorized networks are those blocks,
// and Google Cloud's own public addresses are not let past them.  Without
// them the cluster has no authorized networks, as before.
func TestAGkeClusterAdmitsOnlyItsAllowedCidrs(t *testing.T) {
	t.Run("with AllowedCidrs", func(t *testing.T) {
		inputs := gkeClusterInputs(t, []string{"203.0.113.0/24", "198.51.100.7/32"})

		config, ok := inputs["masterAuthorizedNetworksConfig"].(map[string]any)
		require.True(t, ok, "the cluster has authorized networks")
		assert.Equal(t, []any{
			map[string]any{"cidrBlock": "203.0.113.0/24"},
			map[string]any{"cidrBlock": "198.51.100.7/32"},
		}, config["cidrBlocks"])
		assert.Equal(t, false, config["gcpPublicCidrsAccessEnabled"])

		privateCluster, ok := inputs["privateClusterConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, privateCluster["enablePrivateEndpoint"], "the public endpoint stays, behind the list")
	})
	t.Run("without", func(t *testing.T) {
		inputs := gkeClusterInputs(t, nil)
		assert.NotContains(t, inputs, "masterAuthorizedNetworksConfig")
	})
}

// okeApiRuleSources returns the source of each rule the OKE program puts in
// front of the Kubernetes API, as Pulumi registers them.
func okeApiRuleSources(t *testing.T, allowedCidrs []string) []any {
	t.Helper()
	infra := &KubernetesRuntimeInfraOKE{AllowedCidrs: allowedCidrs}
	apiRules, err := infra.publicKubeApiIngressRules()
	require.NoError(t, err)

	mocks := &gkeRecordingMocks{}
	require.NoError(t, pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := core.NewSecurityList(ctx, "public", &core.SecurityListArgs{
			CompartmentId:        pulumi.String("compartment"),
			VcnId:                pulumi.String("vcn"),
			IngressSecurityRules: apiRules,
		})
		return err
	}, pulumi.WithMocks("oke", "test-stack", mocks)))
	require.Len(t, mocks.resources, 1)

	rules, ok := mocks.resources[0].inputs["ingressSecurityRules"].([]any)
	require.True(t, ok)
	var sources []any
	for _, rule := range rules {
		rule := rule.(map[string]any)
		assert.Equal(t, "6", rule["protocol"])
		assert.Equal(t, map[string]any{"max": float64(6443), "min": float64(6443)}, rule["tcpOptions"])
		sources = append(sources, rule["source"])
	}
	return sources
}

// TestAnOkeClusterAdmitsOnlyItsAllowedCidrs covers #729 on OKE: with
// AllowedCidrs the endpoint's subnet admits the Kubernetes API port from
// each block in place of from anywhere.  Without them it admits every
// address, as before.
func TestAnOkeClusterAdmitsOnlyItsAllowedCidrs(t *testing.T) {
	assert.Equal(t, []any{"203.0.113.0/24", "198.51.100.7/32"}, okeApiRuleSources(t, []string{"203.0.113.0/24", "198.51.100.7/32"}))
	assert.Equal(t, []any{"0.0.0.0/0"}, okeApiRuleSources(t, nil))
}

// gkeResources runs the GKE program and returns what it registered, keyed
// by type token.
func gkeResources(t *testing.T, infra *KubernetesRuntimeInfraGKE) map[string][]gkeRecordedResource {
	t.Helper()
	mocks := &gkeRecordingMocks{}
	require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))
	byType := map[string][]gkeRecordedResource{}
	for _, recorded := range mocks.resources {
		byType[recorded.typeToken] = append(byType[recorded.typeToken], recorded)
	}
	return byType
}

// testGkeInfra returns a GKE infra object for the program tests.
func testGkeInfra() *KubernetesRuntimeInfraGKE {
	return &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:        PulumiWorkspace{RuntimeInstanceName: "gke-egress-test", ProjectName: "gke"},
		ProjectID:              "test-project",
		Region:                 "us-central1",
		WorkerNodeInitialCount: 1,
	}
}

// TestANewGkeClusterSendsItsTrafficFromAReservedAddress covers #729's
// recorded addresses on GKE: a cluster created with ReserveNatAddress gives
// its Cloud NAT one reserved address, and a cluster created before keeps the
// addresses Google allocates, so running its program again leaves its NAT
// as it is.
func TestANewGkeClusterSendsItsTrafficFromAReservedAddress(t *testing.T) {
	t.Run("reserved", func(t *testing.T) {
		infra := testGkeInfra()
		infra.ReserveNatAddress = true
		resources := gkeResources(t, infra)

		addresses := resources["gcp:compute/address:Address"]
		require.Len(t, addresses, 1)
		assert.Equal(t, "gke-egress-test-nat-ip", addresses[0].inputs["name"])
		assert.Equal(t, "EXTERNAL", addresses[0].inputs["addressType"])
		assert.Equal(t, "us-central1", addresses[0].inputs["region"])
		labels, ok := addresses[0].inputs["labels"].(map[string]any)
		require.True(t, ok, "the reserved address is labelled like the cluster")
		for key, value := range GcpResourceLabels(infra.RuntimeInstanceName) {
			assert.Equal(t, value, labels[key])
		}

		nats := resources[gkeNatTypeToken]
		require.Len(t, nats, 1)
		assert.Equal(t, "MANUAL_ONLY", nats[0].inputs["natIpAllocateOption"])
		assert.Len(t, nats[0].inputs["natIps"], 1)
	})
	t.Run("created before", func(t *testing.T) {
		resources := gkeResources(t, testGkeInfra())

		assert.Empty(t, resources["gcp:compute/address:Address"])
		nats := resources[gkeNatTypeToken]
		require.Len(t, nats, 1)
		assert.Equal(t, "AUTO_ONLY", nats[0].inputs["natIpAllocateOption"])
		assert.NotContains(t, nats[0].inputs, "natIps")
	})
}

// TestARestrictedClusterAdmitsTheControlPlaneThatManagesIt covers #729's
// workload clusters: the control plane's addresses join AllowedCidrs on each
// provider, and a combined list longer than the provider admits stops the
// program before it provisions anything.
func TestARestrictedClusterAdmitsTheControlPlaneThatManagesIt(t *testing.T) {
	allowed := []string{"203.0.113.0/24"}
	controlPlane := []string{"198.51.100.20/32"}
	want := []string{"203.0.113.0/24", "198.51.100.20/32"}

	t.Run("EKS", func(t *testing.T) {
		infra := testEksInfra(2)
		infra.AllowedCidrs, infra.ControlPlaneEgressCidrs = allowed, controlPlane
		assert.Equal(t, []any{want[0], want[1]}, eksClusterVpcConfig(t, infra)["publicAccessCidrs"])

		infra.AllowedCidrs = blocksForTest(40)
		err := pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("eks", "test-stack", &eksRecordingMocks{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "leave at most 39 entries in AllowedCidrs")
	})
	t.Run("GKE", func(t *testing.T) {
		infra := testGkeInfra()
		infra.AllowedCidrs, infra.ControlPlaneEgressCidrs = allowed, controlPlane
		clusters := gkeResources(t, infra)[gkeClusterTypeToken]
		require.Len(t, clusters, 1)
		config := clusters[0].inputs["masterAuthorizedNetworksConfig"].(map[string]any)
		assert.Equal(t, []any{
			map[string]any{"cidrBlock": want[0]},
			map[string]any{"cidrBlock": want[1]},
		}, config["cidrBlocks"])

		infra.AllowedCidrs = blocksForTest(50)
		err := pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", &gkeRecordingMocks{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gke admits at most 50")
	})
	t.Run("OKE", func(t *testing.T) {
		infra := &KubernetesRuntimeInfraOKE{AllowedCidrs: allowed, ControlPlaneEgressCidrs: controlPlane}
		rules, err := infra.publicKubeApiIngressRules()
		require.NoError(t, err)
		require.Len(t, rules, 2)

		infra.AllowedCidrs = blocksForTest(120)
		_, err = infra.publicKubeApiIngressRules()
		assert.ErrorContains(t, err, "oke admits at most 120")
	})
	t.Run("an unrestricted cluster stays open", func(t *testing.T) {
		infra := testEksInfra(2)
		infra.ControlPlaneEgressCidrs = controlPlane
		assert.NotContains(t, eksClusterVpcConfig(t, infra), "publicAccessCidrs")
	})
}
