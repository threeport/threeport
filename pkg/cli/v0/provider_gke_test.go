package v0

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestApplyRecordedGkePoolKeepsFourNodes checks a recorded 4-node pool is not resized to zero.
func TestApplyRecordedGkePoolKeepsFourNodes(t *testing.T) {
	// copy a recorded pool
	infra := &provider.KubernetesRuntimeInfraGKE{}
	definition := &api.GcpGkeKubernetesRuntimeDefinition{
		DefaultNodeGroupInstanceType: util.Ptr("e2-standard-2"),
		DefaultNodeGroupInitialSize:  util.Ptr(4),
		DefaultNodeGroupMinimumSize:  util.Ptr(4),
		DefaultNodeGroupMaximumSize:  util.Ptr(6),
		PrivateApi:                   util.Ptr(true),
		AllowedCidrs: func() *datatypes.JSON {
			raw := datatypes.JSON([]byte(`["10.0.0.0/8"]`))
			return &raw
		}(),
	}
	instance := &api.GcpGkeKubernetesRuntimeInstance{
		ReservedNatAddress: util.Ptr(true),
	}

	require.NoError(t, applyRecordedGkePool(infra, definition, instance))
	require.Equal(t, "e2-standard-2", infra.MachineType)
	require.Equal(t, int32(4), infra.WorkerNodeInitialCount)
	require.Equal(t, int32(4), infra.MinNodeCount)
	require.Equal(t, int32(6), infra.MaxNodeCount)
	require.True(t, infra.ReserveNatAddress)
	require.True(t, infra.PrivateApi)
	require.Equal(t, []string{"10.0.0.0/8"}, infra.AllowedCidrs)

	err := applyRecordedGkePool(infra, &api.GcpGkeKubernetesRuntimeDefinition{}, instance)
	require.EqualError(t, err, "recorded machine type is empty")
}

// TestKeepTierGkeMaxNodesRaisesProductionOnly covers the maximum worker node count an upgrade takes per tier.
func TestKeepTierGkeMaxNodesRaisesProductionOnly(t *testing.T) {
	// raise a production pool recorded below the profile's maximum
	infra := &provider.KubernetesRuntimeInfraGKE{MaxNodeCount: 2}
	require.NoError(t, KeepTierGkeMaxNodes(infra, "production"))
	require.Equal(t, int32(10), infra.MaxNodeCount)

	// keep the recorded count on a development tier and on an empty tier
	for _, tier := range []string{"development", ""} {
		infra = &provider.KubernetesRuntimeInfraGKE{MaxNodeCount: 2}
		require.NoError(t, KeepTierGkeMaxNodes(infra, tier))
		require.Equal(t, int32(2), infra.MaxNodeCount, "tier %q", tier)
	}
}
