package gateway

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	logr "github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// v0GatewayDefinitionUpdated writes the manifests it generates to the workload
// definition on every invocation. These cover when that write is worth making.

// definitionFixture wires a stub API serving the three reads the definition path
// makes, and records the writes it receives.
type definitionFixture struct {
	t        *testing.T
	r        *controller.Reconciler
	gd       *v0.GatewayDefinition
	wd       *v0.KubernetesWorkloadDefinition
	writesMu sync.Mutex
	writes   []string
}

func newDefinitionFixture(t *testing.T) *definitionFixture {
	t.Helper()
	api := machinetest.NewAPIStub(t)

	f := &definitionFixture{
		t: t,
		r: &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr},
		gd: &v0.GatewayDefinition{
			Common:                         v0.Common{ID: util.Ptr(uint(1))},
			Definition:                     v0.Definition{Name: util.Ptr("a-gateway")},
			KubernetesWorkloadDefinitionID: util.Ptr(uint(2)),
		},
		wd: &v0.KubernetesWorkloadDefinition{
			Common:     v0.Common{ID: util.Ptr(uint(2))},
			Definition: v0.Definition{Name: util.Ptr("a-gateway")},
		},
	}

	// the ports the manifests are built from
	empty := func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{})
	}
	api.Mux.HandleFunc(v0.PathGatewayHttpPorts, empty)
	api.Mux.HandleFunc(v0.PathGatewayTcpPorts, empty)

	api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", v0.PathKubernetesWorkloadDefinitions, *f.wd.ID),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				f.recordWrite("kubernetes-workload-definition", r)
			}
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*f.wd})
		},
	)
	api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", v0.PathGatewayDefinitions, *f.gd.ID),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				f.recordWrite("gateway-definition", r)
			}
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*f.gd})
		},
	)

	return f
}

func (f *definitionFixture) recordWrite(object string, r *http.Request) {
	_, _ = io.ReadAll(r.Body)
	f.writesMu.Lock()
	defer f.writesMu.Unlock()
	f.writes = append(f.writes, object)
}

func (f *definitionFixture) writtenObjects() []string {
	f.writesMu.Lock()
	defer f.writesMu.Unlock()
	out := make([]string, len(f.writes))
	copy(out, f.writes)

	return out
}

// generated returns the document this gateway definition produces.
func (f *definitionFixture) generated() string {
	f.t.Helper()
	document, err := createGatewayDefinitionYamlDocument(f.r, f.gd)
	require.NoError(f.t, err)

	return document
}

// TestGatewayDefinitionUpdated_UnchangedWritesNothing covers a notification
// against a definition whose manifests already match what is stored.
func TestGatewayDefinitionUpdated_UnchangedWritesNothing(t *testing.T) {
	f := newDefinitionFixture(t)
	log := logr.Discard()

	f.wd.YAMLDocument = util.Ptr(f.generated())

	delay, err := v0GatewayDefinitionUpdated(f.r, f.gd, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)

	assert.Empty(t, f.writtenObjects(), "an unchanged definition should write nothing")
}

// TestGatewayDefinitionUpdated_ChangedWritesBoth covers the case the function
// exists for: the stored manifests are not what the definition now produces.
func TestGatewayDefinitionUpdated_ChangedWritesBoth(t *testing.T) {
	f := newDefinitionFixture(t)
	log := logr.Discard()

	f.wd.YAMLDocument = util.Ptr("kind: VirtualService\n# something else entirely\n")

	_, err := v0GatewayDefinitionUpdated(f.r, f.gd, &log)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"kubernetes-workload-definition", "gateway-definition"},
		f.writtenObjects(),
	)
}

// a workload definition with no document yet is not unchanged
func TestGatewayDefinitionUpdated_AbsentDocumentWrites(t *testing.T) {
	f := newDefinitionFixture(t)
	log := logr.Discard()

	f.wd.YAMLDocument = nil

	_, err := v0GatewayDefinitionUpdated(f.r, f.gd, &log)
	require.NoError(t, err)

	assert.Contains(t, f.writtenObjects(), "kubernetes-workload-definition")
}
