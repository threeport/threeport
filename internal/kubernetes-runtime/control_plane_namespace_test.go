package kubernetesruntime

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// controlPlaneInstancesServer serves the given control plane instances.
func controlPlaneInstancesServer(t *testing.T, instances ...v0.ControlPlaneInstance) *controller.Reconciler {
	t.Helper()
	api := machinetest.NewAPIStub(t)

	api.Mux.HandleFunc(v0.PathControlPlaneInstances, func(w http.ResponseWriter, r *http.Request) {
		objects := make([]apiserver_lib.Object, 0, len(instances))
		for _, instance := range instances {
			objects = append(objects, instance)
		}
		machinetest.WriteResponse(t, w, http.StatusOK, objects)
	})

	return &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
}

// controlPlaneInstance builds an instance with the given namespace.
func controlPlaneInstance(name, namespace string, genesis bool) v0.ControlPlaneInstance {
	return v0.ControlPlaneInstance{
		Instance:  v0.Instance{Name: util.Ptr(name)},
		Namespace: util.Ptr(namespace),
		Genesis:   util.Ptr(genesis),
	}
}

// The workload identity principal a managed cluster is told to authorize names
// the namespace the control plane's controllers run in. A genesis install can be
// given any namespace, and assuming the default authorizes a principal that
// never connects.

func TestControlPlaneHostNamespace_ReadsTheGenesisNamespace(t *testing.T) {
	r := controlPlaneInstancesServer(t, controlPlaneInstance("genesis", "a-custom-namespace", true))

	namespace, err := controlPlaneHostNamespace(r)
	require.NoError(t, err)
	assert.Equal(t, "a-custom-namespace", namespace)
}

// a child control plane has its own namespace, which is not where the
// controllers doing this work run
func TestControlPlaneHostNamespace_IgnoresChildControlPlanes(t *testing.T) {
	r := controlPlaneInstancesServer(
		t,
		controlPlaneInstance("a-child", "child-namespace", false),
		controlPlaneInstance("genesis", "a-custom-namespace", true),
	)

	namespace, err := controlPlaneHostNamespace(r)
	require.NoError(t, err)
	assert.Equal(t, "a-custom-namespace", namespace)
}

// guessing here would authorize a principal that never connects, so it says so
func TestControlPlaneHostNamespace_WithoutAGenesisInstance(t *testing.T) {
	r := controlPlaneInstancesServer(t, controlPlaneInstance("a-child", "child-namespace", false))

	_, err := controlPlaneHostNamespace(r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no genesis control plane instance")
}

func TestControlPlaneHostNamespace_WithoutARecordedNamespace(t *testing.T) {
	instance := controlPlaneInstance("genesis", "", true)
	r := controlPlaneInstancesServer(t, instance)

	_, err := controlPlaneHostNamespace(r)
	require.Error(t, err)
}
