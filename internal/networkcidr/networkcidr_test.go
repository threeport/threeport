package networkcidr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeControlPlane is an API holding kubernetes runtime instances and control
// plane instances, which records the ranges written to runtimes.
type fakeControlPlane struct {
	mu            sync.Mutex
	runtimes      map[uint]*v0.KubernetesRuntimeInstance
	controlPlanes []v0.ControlPlaneInstance
	// The number of range writes still to refuse, as the API does a range
	// another runtime took first
	refusals int
	written  []string
}

// pool returns a pool as the JSON a control plane instance stores.
func pool(blocks ...string) *datatypes.JSON {
	encoded, _ := json.Marshal(blocks)
	raw := datatypes.JSON(encoded)
	return &raw
}

// serve starts the fake API.
func (f *fakeControlPlane) serve(t *testing.T) *machinetest.APIStub {
	t.Helper()
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(v0.PathControlPlaneInstances, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var objects []apiserver_lib.Object
		for _, controlPlane := range f.controlPlanes {
			if r.URL.Query().Get("isself") == "true" && (controlPlane.IsSelf == nil || !*controlPlane.IsSelf) {
				continue
			}
			objects = append(objects, controlPlane)
		}
		machinetest.WriteResponse(t, w, http.StatusOK, objects)
	})
	api.Mux.HandleFunc(v0.PathKubernetesRuntimeInstances, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var objects []apiserver_lib.Object
		for _, runtime := range f.runtimes {
			objects = append(objects, *runtime)
		}
		machinetest.WriteResponse(t, w, http.StatusOK, objects)
	})
	api.Mux.HandleFunc(v0.PathKubernetesRuntimeInstances+"/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var id uint
		for candidate := range f.runtimes {
			if strings.HasSuffix(r.URL.Path, "/"+strconv.FormatUint(uint64(candidate), 10)) {
				id = candidate
			}
		}
		runtime := f.runtimes[id]
		require.NotNil(t, runtime, r.URL.Path)
		if r.Method == http.MethodPatch {
			var update v0.KubernetesRuntimeInstance
			require.NoError(t, json.NewDecoder(r.Body).Decode(&update))
			if f.refusals > 0 {
				f.refusals--
				machinetest.WriteResponse(t, w, http.StatusBadRequest, nil)
				return
			}
			runtime.NetworkCidr = update.NetworkCidr
			f.written = append(f.written, *update.NetworkCidr)
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*runtime})
	})

	return api
}

// runtime returns a kubernetes runtime instance with a range.
func runtime(id uint, networkCidr *string) *v0.KubernetesRuntimeInstance {
	return &v0.KubernetesRuntimeInstance{
		Common:      v0.Common{ID: util.Ptr(id)},
		Instance:    v0.Instance{Name: util.Ptr("runtime-" + strconv.FormatUint(uint64(id), 10))},
		NetworkCidr: networkCidr,
	}
}

// TestARuntimeIsAllocatedTheFirstFreeBlockOfThePool covers allocating a
// range: the first /16 of this control plane's pool no runtime has and no
// child's pool holds, recorded on the runtime.
func TestARuntimeIsAllocatedTheFirstFreeBlockOfThePool(t *testing.T) {
	f := &fakeControlPlane{
		runtimes: map[uint]*v0.KubernetesRuntimeInstance{
			1: runtime(1, util.Ptr("10.16.0.0/16")),
			2: runtime(2, nil),
			3: runtime(3, util.Ptr("10.0.0.0/16")),
		},
		controlPlanes: []v0.ControlPlaneInstance{
			{Common: v0.Common{ID: util.Ptr(uint(1))}, IsSelf: util.Ptr(true)},
			{Common: v0.Common{ID: util.Ptr(uint(2))}, Instance: v0.Instance{Name: util.Ptr("child")}, NetworkCidrPool: pool("10.17.0.0/16")},
			{Common: v0.Common{ID: util.Ptr(uint(3))}, Instance: v0.Instance{Name: util.Ptr("parent")}},
		},
	}
	api := f.serve(t)

	cidr, err := EnsureRuntimeNetworkCidr(api.Client, api.Addr, 2)
	require.NoError(t, err)
	assert.Equal(t, "10.18.0.0/16", cidr, "past a runtime's range and a child's pool")
	assert.Equal(t, []string{"10.18.0.0/16"}, f.written)

	// a runtime with a range keeps it
	cidr, err = EnsureRuntimeNetworkCidr(api.Client, api.Addr, 2)
	require.NoError(t, err)
	assert.Equal(t, "10.18.0.0/16", cidr)
	assert.Len(t, f.written, 1)

	// this control plane's own pool
	f.controlPlanes[0].NetworkCidrPool = pool("172.24.0.0/14")
	f.runtimes[4] = runtime(4, nil)
	cidr, err = EnsureRuntimeNetworkCidr(api.Client, api.Addr, 4)
	require.NoError(t, err)
	assert.Equal(t, "172.24.0.0/16", cidr)
}

// TestAChildAllocatesOnlyFromThePoolItsParentGaveIt covers a child control
// plane: with no pool it allocates nothing, since the default pool is its
// parent's, whose ranges its database doesn't hold, so its clusters keep the
// layout every cluster used to get.  With a pool, it allocates from it.
func TestAChildAllocatesOnlyFromThePoolItsParentGaveIt(t *testing.T) {
	f := &fakeControlPlane{
		runtimes: map[uint]*v0.KubernetesRuntimeInstance{1: runtime(1, nil)},
		controlPlanes: []v0.ControlPlaneInstance{
			{Common: v0.Common{ID: util.Ptr(uint(1))}, IsSelf: util.Ptr(true), ParentControlPlaneInstanceID: util.Ptr(uint(2))},
			{Common: v0.Common{ID: util.Ptr(uint(2))}, Instance: v0.Instance{Name: util.Ptr("parent")}},
		},
	}
	api := f.serve(t)

	cidr, err := EnsureRuntimeNetworkCidr(api.Client, api.Addr, 1)
	require.NoError(t, err)
	assert.Empty(t, cidr)
	assert.Empty(t, f.written, "nothing is recorded")
	cidr, err = ClusterNetworkCidr(api.Client, api.Addr, util.Ptr(uint(1)), v0.Reconciliation{}, nil)
	require.NoError(t, err)
	assert.Empty(t, cidr, "a new cluster gets the layout every cluster used to get")

	f.controlPlanes[0].NetworkCidrPool = pool("10.48.0.0/12")
	cidr, err = EnsureRuntimeNetworkCidr(api.Client, api.Addr, 1)
	require.NoError(t, err)
	assert.Equal(t, "10.48.0.0/16", cidr)

	// an empty pool is no pool
	f.controlPlanes[0].NetworkCidrPool = pool()
	f.runtimes[2] = runtime(2, nil)
	cidr, err = EnsureRuntimeNetworkCidr(api.Client, api.Addr, 2)
	require.NoError(t, err)
	assert.Empty(t, cidr)
}

// TestARangeAnotherRuntimeTookIsAllocatedAgain covers the API refusing a range
// another runtime took at the same time: the allocation reads the ranges in
// use again and retries, a bounded number of times.
func TestARangeAnotherRuntimeTookIsAllocatedAgain(t *testing.T) {
	f := &fakeControlPlane{
		runtimes:      map[uint]*v0.KubernetesRuntimeInstance{1: runtime(1, nil)},
		controlPlanes: []v0.ControlPlaneInstance{{Common: v0.Common{ID: util.Ptr(uint(1))}, IsSelf: util.Ptr(true)}},
		refusals:      2,
	}
	api := f.serve(t)

	cidr, err := EnsureRuntimeNetworkCidr(api.Client, api.Addr, 1)
	require.NoError(t, err)
	assert.Equal(t, "10.16.0.0/16", cidr)

	f.runtimes[2] = runtime(2, nil)
	f.refusals = allocationAttempts
	_, err = EnsureRuntimeNetworkCidr(api.Client, api.Addr, 2)
	assert.ErrorContains(t, err, "after 5 attempts")
	assert.Nil(t, f.runtimes[2].NetworkCidr)
}

// TestOnlyAClusterYetToBeProvisionedIsAllocatedARange covers which clusters
// are allocated a range: one with nothing provisioned.  One already
// provisioned, or being deleted, keeps what its runtime records, none for a
// cluster created before ranges were allocated, whose network a new layout
// would replace.
func TestOnlyAClusterYetToBeProvisionedIsAllocatedARange(t *testing.T) {
	newPlane := func() *fakeControlPlane {
		return &fakeControlPlane{
			runtimes:      map[uint]*v0.KubernetesRuntimeInstance{1: runtime(1, nil)},
			controlPlanes: []v0.ControlPlaneInstance{{Common: v0.Common{ID: util.Ptr(uint(1))}, IsSelf: util.Ptr(true)}},
		}
	}
	inventory := datatypes.JSON(`{"deployment":{"resources":[{"urn":"urn:vpc"}]}}`)
	empty := datatypes.JSON(`{}`)

	for name, c := range map[string]struct {
		reconciliation v0.Reconciliation
		inventory      *datatypes.JSON
		want           string
	}{
		"new":                    {v0.Reconciliation{}, nil, "10.16.0.0/16"},
		"new, empty inventory":   {v0.Reconciliation{}, &empty, "10.16.0.0/16"},
		"provisioning":           {v0.Reconciliation{}, &inventory, ""},
		"confirmed":              {v0.Reconciliation{CreationConfirmed: util.Ptr(time.Now())}, nil, ""},
		"deleted before created": {v0.Reconciliation{DeletionScheduled: util.Ptr(time.Now())}, nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPlane()
			api := f.serve(t)
			cidr, err := ClusterNetworkCidr(api.Client, api.Addr, util.Ptr(uint(1)), c.reconciliation, c.inventory)
			require.NoError(t, err)
			assert.Equal(t, c.want, cidr)
			if c.want == "" {
				assert.Empty(t, f.written, "nothing is allocated")
			}
		})
	}

	f := newPlane()
	f.runtimes[1].NetworkCidr = util.Ptr("10.40.0.0/16")
	api := f.serve(t)
	cidr, err := ClusterNetworkCidr(api.Client, api.Addr, util.Ptr(uint(1)), v0.Reconciliation{CreationConfirmed: util.Ptr(time.Now())}, &inventory)
	require.NoError(t, err)
	assert.Equal(t, "10.40.0.0/16", cidr, "a provisioned cluster's recorded range")

	_, err = ClusterNetworkCidr(api.Client, api.Addr, nil, v0.Reconciliation{}, nil)
	assert.ErrorContains(t, err, "names no kubernetes runtime instance")
}
