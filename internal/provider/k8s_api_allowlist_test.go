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

	mocks := &gkeRecordingMocks{}
	require.NoError(t, pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := core.NewSecurityList(ctx, "public", &core.SecurityListArgs{
			CompartmentId:        pulumi.String("compartment"),
			VcnId:                pulumi.String("vcn"),
			IngressSecurityRules: infra.publicKubeApiIngressRules(),
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
