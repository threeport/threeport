package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/go-logr/logr/funcr"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAStateWithoutEgressCidrsHasNone covers clusters provisioned before the
// output existed, and states that cannot be read.
func TestAStateWithoutEgressCidrsHasNone(t *testing.T) {
	state := func(raw string) *datatypes.JSON {
		value := datatypes.JSON(raw)
		return &value
	}

	cidrs, err := EgressCidrsFromState(nil)
	require.NoError(t, err)
	assert.Nil(t, cidrs)

	cidrs, err = EgressCidrsFromState(state(checkpointState("p", "s", "no-outputs")))
	require.NoError(t, err)
	assert.Nil(t, cidrs, "a stack that exports nothing")

	cidrs, err = EgressCidrsFromState(state(`{"checkpoint":{"latest":{"resources":[{"type":"pulumi:pulumi:Stack","outputs":{"egressCidrs":[]}}]}}}`))
	require.NoError(t, err)
	assert.Empty(t, cidrs, "a GKE cluster whose NAT addresses Google allocates")

	_, err = EgressCidrsFromState(state(`{"checkpoint":{"latest":{"resources":[{"type":"pulumi:pulumi:Stack","outputs":{"egressCidrs":"203.0.113.7/32"}}]}}}`))
	assert.ErrorContains(t, err, "failed to read the egressCidrs output")

	_, err = EgressCidrsFromState(state(`not json`))
	assert.ErrorContains(t, err, "failed to read Pulumi state")
}

// TestTheControlPlanesAddressesJoinARestrictedAllowlist covers the list a
// workload cluster's endpoint admits.
func TestTheControlPlanesAddressesJoinARestrictedAllowlist(t *testing.T) {
	controlPlane := []string{"198.51.100.20/32", "198.51.100.21/32"}

	allowlist, err := kubeApiAllowlist("eks", []string{"203.0.113.0/24"}, controlPlane)
	require.NoError(t, err)
	assert.Equal(t, []string{"203.0.113.0/24", "198.51.100.20/32", "198.51.100.21/32"}, allowlist)

	allowlist, err = kubeApiAllowlist("eks", []string{"198.51.100.20/32"}, controlPlane)
	require.NoError(t, err)
	assert.Equal(t, []string{"198.51.100.20/32", "198.51.100.21/32"}, allowlist, "an address already listed is not repeated")

	allowlist, err = kubeApiAllowlist("gke", nil, controlPlane)
	require.NoError(t, err)
	assert.Nil(t, allowlist, "an unrestricted endpoint stays unrestricted")

	allowlist, err = kubeApiAllowlist("oke", []string{"203.0.113.0/24"}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"203.0.113.0/24"}, allowlist, "a control plane with no recorded addresses adds none")

	_, err = kubeApiAllowlist("eks", blocksForTest(39), controlPlane)
	require.Error(t, err)
	assert.Equal(t,
		"AllowedCidrs has 39 entries and the control plane adds 2 of its own addresses, but eks admits at most 40 - leave at most 38 entries in AllowedCidrs",
		err.Error(),
	)

	_, err = kubeApiAllowlist("eks", blocksForTest(38), controlPlane)
	assert.NoError(t, err, "a list that fits with the control plane's addresses")
}

// blocksForTest returns n different /32 blocks outside the ones the tests
// give the control plane.
func blocksForTest(n int) []string {
	blocks := make([]string, n)
	for i := range blocks {
		blocks[i] = fmt.Sprintf("203.0.113.%d/32", i)
	}
	return blocks
}

// runtimeAPI serves the runtime with ID 5 and, when host is not nil, the
// runtime that hosts the control plane, recording whether the host was
// looked up.
func runtimeAPI(t *testing.T, workloadIsHost bool, host *v0.KubernetesRuntimeInstance) (*machinetest.APIStub, *bool) {
	t.Helper()
	api := machinetest.NewAPIStub(t)
	lookedUp := false
	api.Mux.HandleFunc(v0.PathKubernetesRuntimeInstances+"/5", func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{v0.KubernetesRuntimeInstance{
			Common:                    v0.Common{ID: util.Ptr(uint(5))},
			ThreeportControlPlaneHost: util.Ptr(workloadIsHost),
		}})
	})
	api.Mux.HandleFunc(v0.PathKubernetesRuntimeInstances, func(w http.ResponseWriter, r *http.Request) {
		lookedUp = true
		assert.Equal(t, "threeportcontrolplanehost=true", r.URL.RawQuery)
		hosts := []apiserver_lib.Object{}
		if host != nil {
			hosts = append(hosts, *host)
		}
		machinetest.WriteResponse(t, w, http.StatusOK, hosts)
	})
	return api, &lookedUp
}

// TestAWorkloadClusterAdmitsTheAddressesOfTheControlPlanesHost covers the
// lookup the EKS, GKE and OKE controllers make for a cluster restricted with
// AllowedCidrs.
func TestAWorkloadClusterAdmitsTheAddressesOfTheControlPlanesHost(t *testing.T) {
	egress := datatypes.JSON(`["198.51.100.20/32"]`)
	host := &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: util.Ptr(true), EgressCidrs: &egress}

	t.Run("a workload cluster", func(t *testing.T) {
		api, _ := runtimeAPI(t, false, host)
		cidrs, err := ControlPlaneEgressCidrs(api.Client, api.Addr, 5, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"198.51.100.20/32"}, cidrs)
	})
	t.Run("the control plane's own cluster", func(t *testing.T) {
		api, lookedUp := runtimeAPI(t, true, host)
		cidrs, err := ControlPlaneEgressCidrs(api.Client, api.Addr, 5, nil)
		require.NoError(t, err)
		assert.Nil(t, cidrs, "the control plane reaches its own cluster from inside")
		assert.False(t, *lookedUp)
	})
	t.Run("a host with no recorded addresses", func(t *testing.T) {
		api, _ := runtimeAPI(t, false, &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: util.Ptr(true)})
		var logged []string
		log := funcr.New(func(prefix, args string) { logged = append(logged, args) }, funcr.Options{})
		cidrs, err := ControlPlaneEgressCidrs(api.Client, api.Addr, 5, &log)
		require.NoError(t, err)
		assert.Nil(t, cidrs)
		require.Len(t, logged, 1)
		assert.Contains(t, logged[0], "has no EgressCidrs")
	})
	t.Run("no host registered", func(t *testing.T) {
		api, _ := runtimeAPI(t, false, nil)
		cidrs, err := ControlPlaneEgressCidrs(api.Client, api.Addr, 5, nil)
		require.NoError(t, err)
		assert.Nil(t, cidrs)
	})
}

// recordEgressExports replaces exportEgressCidrs for the test and returns
// how many addresses each call exported.
func recordEgressExports(t *testing.T) *[]int {
	t.Helper()
	var exported []int
	original := exportEgressCidrs
	exportEgressCidrs = func(ctx *pulumi.Context, addresses []pulumi.StringOutput) {
		exported = append(exported, len(addresses))
		original(ctx, addresses)
	}
	t.Cleanup(func() { exportEgressCidrs = original })
	return &exported
}

// TestEachProgramRecordsTheAddressesItsTrafficLeavesFrom covers which
// addresses the EKS and GKE programs record: one per NAT gateway on EKS, the
// reserved NAT address on GKE, and none on a GKE cluster whose NAT addresses
// Google allocates.
func TestEachProgramRecordsTheAddressesItsTrafficLeavesFrom(t *testing.T) {
	t.Run("EKS across two zones", func(t *testing.T) {
		exported := recordEgressExports(t)
		runEksPulumiProgram(t, testEksInfra(2))
		assert.Equal(t, []int{2}, *exported)
	})
	t.Run("EKS across three zones", func(t *testing.T) {
		exported := recordEgressExports(t)
		runEksPulumiProgram(t, testEksInfra(3))
		assert.Equal(t, []int{3}, *exported)
	})
	t.Run("GKE with a reserved address", func(t *testing.T) {
		exported := recordEgressExports(t)
		infra := testGkeInfra()
		infra.ReserveNatAddress = true
		gkeResources(t, infra)
		assert.Equal(t, []int{1}, *exported)
	})
	t.Run("GKE created before", func(t *testing.T) {
		exported := recordEgressExports(t)
		gkeResources(t, testGkeInfra())
		assert.Equal(t, []int{0}, *exported)
	})
}

// TestAnAllowlistOverTheLimitIsRefusedBeforeAnythingIsCreated covers an
// AllowedCidrs list that fits the provider's limit on its own but not with
// the control plane's addresses: Create and CreateInfra refuse it before
// they create IAM, a service account, a compartment or a stack, so there is
// nothing to clean up.  None of the infra here has credentials or a region,
// so a call that went on to the cloud would fail with another error.
func TestAnAllowlistOverTheLimitIsRefusedBeforeAnythingIsCreated(t *testing.T) {
	fill := func(infraProvider string) []string {
		limit, _ := v0.AllowedCidrsLimit(infraProvider, false)
		cidrs := make([]string, limit-1)
		for i := range cidrs {
			cidrs[i] = fmt.Sprintf("203.0.%d.%d/32", i/256, i%256)
		}
		return cidrs
	}
	controlPlane := []string{"198.51.100.20/32", "198.51.100.21/32"}

	eks := &KubernetesRuntimeInfraEKS{
		AllowedCidrs:            fill(v0.KubernetesRuntimeInfraProviderEKS),
		ControlPlaneEgressCidrs: controlPlane,
	}
	gke := &KubernetesRuntimeInfraGKE{
		AllowedCidrs:            fill(v0.KubernetesRuntimeInfraProviderGKE),
		ControlPlaneEgressCidrs: controlPlane,
	}
	oke := &KubernetesRuntimeInfraOKE{
		AllowedCidrs:            fill(v0.KubernetesRuntimeInfraProviderOKE),
		ControlPlaneEgressCidrs: controlPlane,
	}
	for name, create := range map[string]func() (*kube.KubeConnectionInfo, error){
		"eks Create":      eks.Create,
		"eks CreateInfra": eks.CreateInfra,
		"gke Create":      gke.Create,
		"gke CreateInfra": gke.CreateInfra,
		"oke Create":      oke.Create,
		"oke CreateInfra": oke.CreateInfra,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := create()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the control plane adds 2 of its own addresses")
		})
	}
}
