package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAProductionGkeClusterIsBuiltToLoseAZone covers the GKE cluster tptctl
// up creates for each tier.  A production cluster runs on machines that don't
// burst and keeps a node in each of its three zones, balanced, for the
// database's three replicas.  A development cluster is the one tptctl has
// always created.
func TestAProductionGkeClusterIsBuiltToLoseAZone(t *testing.T) {
	dev, err := gkeControlPlaneProfile(threeport.ControlPlaneTier(threeport.ControlPlaneTierDev))
	require.NoError(t, err)
	assert.Equal(t, gkeClusterProfile{
		NodeSize: "Medium", MachineType: "e2-medium", LocationPolicy: "ANY",
		InitialNodeCount: 2, MinNodeCount: 1, MaxNodeCount: 10,
	}, dev)

	prod, err := gkeControlPlaneProfile(threeport.ControlPlaneTier(threeport.ControlPlaneTierProd))
	require.NoError(t, err)
	assert.Equal(t, gkeClusterProfile{
		NodeSize: "Large", MachineType: "e2-standard-4", LocationPolicy: "BALANCED",
		InitialNodeCount: 1, MinNodeCount: 1, MaxNodeCount: 10,
	}, prod)
	assert.False(t, strings.HasPrefix(prod.MachineType, "e2-medium") || strings.HasPrefix(prod.MachineType, "e2-small"),
		"%s is a shared-core machine, which bursts", prod.MachineType)
	assert.GreaterOrEqual(t, 3*prod.MinNodeCount, int32(3), "three replicas need three nodes in three zones")
}

// TestTheEksClusterIsTheOneTptctlHasAlwaysCreated covers the EKS cluster
// tptctl up creates, now read from the node size mapping.
func TestTheEksClusterIsTheOneTptctlHasAlwaysCreated(t *testing.T) {
	profile, err := eksControlPlaneProfile()
	require.NoError(t, err)
	assert.Equal(t, eksClusterProfile{
		NodeSize: "Medium", InstanceType: "t3.medium", ZoneCount: 2,
		InitialNodeCount: 3, MinNodeCount: 3, MaxNodeCount: 250,
	}, profile)
}

// TestTheGenesisDefinitionDescribesItsCluster covers the
// KubernetesRuntimeDefinition tptctl up records for the cluster it creates:
// its node size and profile map to the machine type the cluster runs, and
// its availability and maximum match the cluster's.
func TestTheGenesisDefinitionDescribesItsCluster(t *testing.T) {
	for _, c := range []struct {
		provider    string
		tier        string
		ha          bool
		size        string
		max         int
		machineType string
		mapProvider string
	}{
		{v0.KubernetesRuntimeInfraProviderGKE, threeport.ControlPlaneTierProd, true, "Large", 10, "e2-standard-4", util.GcpProvider},
		{v0.KubernetesRuntimeInfraProviderGKE, threeport.ControlPlaneTierDev, true, "Medium", 10, "e2-medium", util.GcpProvider},
		{v0.KubernetesRuntimeInfraProviderEKS, threeport.ControlPlaneTierProd, false, "Medium", 250, "t3.medium", util.AwsProvider},
	} {
		t.Run(c.provider+" "+c.tier, func(t *testing.T) {
			definition := &v0.KubernetesRuntimeDefinition{InfraProvider: util.Ptr(c.provider)}
			require.NoError(t, recordClusterProfile(definition, threeport.ControlPlaneTier(c.tier)))

			assert.Equal(t, util.Ptr(c.ha), definition.HighAvailability)
			assert.Equal(t, util.Ptr(c.size), definition.NodeSize)
			assert.Equal(t, util.Ptr(controlPlaneNodeProfile), definition.NodeProfile)
			assert.Equal(t, util.Ptr(c.max), definition.NodeMaximum)
			machineType, err := mapping.GetMachineType(c.mapProvider, *definition.NodeProfile, *definition.NodeSize)
			require.NoError(t, err)
			assert.Equal(t, c.machineType, machineType)
		})
	}

	kind := &v0.KubernetesRuntimeDefinition{InfraProvider: util.Ptr(v0.KubernetesRuntimeInfraProviderKind)}
	require.NoError(t, recordClusterProfile(kind, threeport.ControlPlaneTier(threeport.ControlPlaneTierDev)))
	assert.Nil(t, kind.HighAvailability, "a provider with no profile keeps the definition's defaults")
	assert.Nil(t, kind.NodeSize)
}

// TestTheGenesisGkeDefinitionDescribesItsNodePool covers the GKE definition
// tptctl up registers for its cluster: the node pool's machine type and its
// initial, minimum and maximum node counts.
func TestTheGenesisGkeDefinitionDescribesItsNodePool(t *testing.T) {
	var sent v0.GcpGkeKubernetesRuntimeDefinition
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"Data":[]}`)
			return
		}
		if r.URL.Path == v0.PathGcpGkeKubernetesRuntimeDefinitions {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"Data":[{"ID":4}]}`)
	}))
	defer server.Close()

	// development's initial and minimum counts differ, so the record shows
	// which it took
	profile, err := gkeControlPlaneProfile(threeport.ControlPlaneTier(threeport.ControlPlaneTierDev))
	require.NoError(t, err)
	var infra provider.KubernetesRuntimeInfra = &provider.KubernetesRuntimeInfraGKE{
		ProjectID:              "project",
		Region:                 "us-central1",
		MachineType:            profile.MachineType,
		WorkerNodeInitialCount: profile.InitialNodeCount,
		MinNodeCount:           profile.MinNodeCount,
		MaxNodeCount:           profile.MaxNodeCount,
	}
	require.NoError(t, ConfigureControlPlaneWithGkeConfig(
		&threeport.ControlPlaneInstaller{Opts: threeport.Options{ControlPlaneName: "test", ControlPlaneOnly: true}},
		&Uninstaller{},
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.KubernetesRuntimeDefinition{Common: v0.Common{ID: util.Ptr(uint(2))}},
		&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}},
		&infra,
	))

	assert.Equal(t, util.Ptr("e2-medium"), sent.DefaultNodeGroupInstanceType)
	assert.Equal(t, util.Ptr(2), sent.DefaultNodeGroupInitialSize)
	assert.Equal(t, util.Ptr(1), sent.DefaultNodeGroupMinimumSize)
	assert.Equal(t, util.Ptr(10), sent.DefaultNodeGroupMaximumSize)
	assert.Equal(t, util.Ptr(3), sent.ZoneCount)
}
