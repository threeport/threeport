package v0

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestGkeInfraForUpgradeBuildsTheTierProfile covers the node pool and endpoint access an upgrade takes from the tier and the recorded config.
func TestGkeInfraForUpgradeBuildsTheTierProfile(t *testing.T) {
	// build a production upgrade from a recorded config
	controlPlane := &ControlPlane{
		Name:     "prod",
		Provider: api.KubernetesRuntimeInfraProviderGKE,
		GKEProviderConfig: GKEProviderConfig{
			GcpProjectId: "test-project",
			GcpRegion:    "us-central1",
			PrivateApi:   util.Ptr(true),
			AllowedCidrs: []string{"10.0.0.0/8"},
		},
	}
	infra, err := GkeInfraForUpgrade(controlPlane, "production")
	require.NoError(t, err)

	// assert the production profile, with no recorded sizes copied over it
	require.Equal(t, "production", infra.Tier)
	require.Equal(t, int32(1), infra.MinNodeCount)
	require.Equal(t, int32(10), infra.MaxNodeCount)
	require.Equal(t, "BALANCED", infra.LocationPolicy)
	require.True(t, infra.ProvisionBackups)
	require.True(t, infra.ReserveNatAddress)

	// assert the recorded project, region, and endpoint access
	require.Equal(t, "test-project", infra.ProjectID)
	require.Equal(t, "us-central1", infra.Region)
	require.True(t, infra.PrivateApi)
	require.Equal(t, []string{"10.0.0.0/8"}, infra.AllowedCidrs)

	// assert the development profile for a development or empty tier
	for _, tier := range []string{"development", ""} {
		infra, err = GkeInfraForUpgrade(controlPlane, tier)
		require.NoError(t, err)
		require.Equal(t, int32(2), infra.WorkerNodeInitialCount, "tier %q", tier)
		require.Equal(t, "ANY", infra.LocationPolicy, "tier %q", tier)
		require.False(t, infra.ProvisionBackups, "tier %q", tier)
	}
}
