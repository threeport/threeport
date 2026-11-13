package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestTheNetworkFlagsGiveTheClustersRangeAndThePool covers --network-cidr and
// --network-cidr-pool: the range defaults to the pool's first block for a
// cluster tptctl creates, is recorded as given for an existing cluster, and
// doesn't apply to kind, while the pool applies everywhere.
func TestTheNetworkFlagsGiveTheClustersRangeAndThePool(t *testing.T) {
	eks := v0.KubernetesRuntimeInfraProviderEKS

	cidr, pool, err := ResolveNetworkCidrFlags("", "", eks, false)
	require.NoError(t, err)
	assert.Equal(t, "10.16.0.0/16", cidr, "the default pool's first block")
	assert.Empty(t, pool)

	cidr, pool, err = ResolveNetworkCidrFlags("", " 172.24.0.0/14 , 10.200.0.0/16", eks, false)
	require.NoError(t, err)
	assert.Equal(t, "172.24.0.0/16", cidr, "the given pool's first block")
	assert.Equal(t, []string{"172.24.0.0/14", "10.200.0.0/16"}, pool)

	cidr, _, err = ResolveNetworkCidrFlags("10.50.0.0/16", "172.24.0.0/14", v0.KubernetesRuntimeInfraProviderGKE, false)
	require.NoError(t, err)
	assert.Equal(t, "10.50.0.0/16", cidr, "a range given need not be in the pool")

	cidr, _, err = ResolveNetworkCidrFlags("", "", eks, true)
	require.NoError(t, err)
	assert.Empty(t, cidr, "an existing cluster's range isn't known")
	cidr, _, err = ResolveNetworkCidrFlags("10.0.0.0/16", "", eks, true)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.0/16", cidr, "an existing cluster's range, recorded so no runtime is allocated it")

	cidr, pool, err = ResolveNetworkCidrFlags("", "172.24.0.0/14", v0.KubernetesRuntimeInfraProviderKind, false)
	require.NoError(t, err)
	assert.Empty(t, cidr)
	assert.Equal(t, []string{"172.24.0.0/14"}, pool, "a control plane on kind can create clusters elsewhere")
	_, _, err = ResolveNetworkCidrFlags("10.16.0.0/16", "", v0.KubernetesRuntimeInfraProviderKind, false)
	assert.ErrorContains(t, err, "--network-cidr does not apply to a kind cluster")

	for _, bad := range []string{"10.16.0.0/17", "10.16.1.0/16", "10.96.0.0/16", "172.20.0.0/16", "nope"} {
		_, _, err := ResolveNetworkCidrFlags(bad, "", eks, false)
		assert.Error(t, err, bad)
	}
	for _, bad := range []string{"10.16.0.0/17", "10.16.0.0/16,10.16.0.0/15", "nope"} {
		_, _, err := ResolveNetworkCidrFlags("", bad, eks, false)
		assert.Error(t, err, bad)
	}
	_, _, err = ResolveNetworkCidrFlags("", "10.96.0.0/16", eks, false)
	assert.ErrorContains(t, err, "--network-cidr-pool", "a pool with no block to allocate")
}

// TestTheInstallerTakesTheResolvedNetworkFlags covers tptctl up's installer
// carrying the range and the pool.
func TestTheInstallerTakesTheResolvedNetworkFlags(t *testing.T) {
	args := GenesisControlPlaneCLIArgs{InfraProvider: v0.KubernetesRuntimeInfraProviderGKE, NetworkCidrPool: "172.24.0.0/14"}
	cpi, err := args.CreateInstaller()
	require.NoError(t, err)
	assert.Equal(t, "172.24.0.0/16", cpi.Opts.NetworkCidr)
	assert.Equal(t, []string{"172.24.0.0/14"}, cpi.Opts.NetworkCidrPool)

	args.NetworkCidr = "10.16.1.0/16"
	_, err = args.CreateInstaller()
	assert.Error(t, err)
}

// TestTheGenesisRuntimeRecordsItsClustersRange covers the range recorded on
// the control plane cluster's runtime: the one its infra laid it out in, which
// a resumed create keeps from the stack, and with --control-plane-only the one
// given.
func TestTheGenesisRuntimeRecordsItsClustersRange(t *testing.T) {
	opts := threeport.Options{NetworkCidr: "10.16.0.0/16"}
	assert.Equal(t, "10.30.0.0/16", genesisNetworkCidr(opts, &provider.KubernetesRuntimeInfraEKS{NetworkCidr: "10.30.0.0/16"}))
	assert.Equal(t, "10.30.0.0/16", genesisNetworkCidr(opts, &provider.KubernetesRuntimeInfraGKE{NetworkCidr: "10.30.0.0/16"}))
	assert.Equal(t, "10.30.0.0/16", genesisNetworkCidr(opts, &provider.KubernetesRuntimeInfraOKE{NetworkCidr: "10.30.0.0/16"}))
	assert.Empty(t, genesisNetworkCidr(opts, &provider.KubernetesRuntimeInfraEKS{}), "a stack laid out before ranges were allocated")
	assert.Empty(t, genesisNetworkCidr(opts, &provider.KubernetesRuntimeInfraKind{}))

	opts.ControlPlaneOnly = true
	assert.Equal(t, "10.16.0.0/16", genesisNetworkCidr(opts, &provider.KubernetesRuntimeInfraEKS{}))

	raw, err := NetworkCidrPoolJSON([]string{"172.24.0.0/14"})
	require.NoError(t, err)
	assert.JSONEq(t, `["172.24.0.0/14"]`, string(*raw))
	raw, err = NetworkCidrPoolJSON(nil)
	require.NoError(t, err)
	assert.Nil(t, raw, "the default pool")
}
