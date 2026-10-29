package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// controllerFixture is the api state one gateway controller install uses.
// The counts record definitions and instances the install creates.
type controllerFixture struct {
	runtime      *api.KubernetesRuntimeInstance
	workload     *api.KubernetesWorkloadInstance
	definition   *api.KubernetesWorkloadDefinition
	gateways     []api.GatewayInstance
	missing      bool
	definitions  int
	instances    int
	deleted      bool
	definitionID uint
	instanceID   uint
}

// kindRuntime returns the kind runtime the install reads.
func kindRuntime() *api.KubernetesRuntimeInstance {
	return &api.KubernetesRuntimeInstance{
		Common:                        api.Common{ID: util.Ptr(uint(3))},
		Instance:                      api.Instance{Name: util.Ptr("dev")},
		KubernetesRuntimeDefinitionID: util.Ptr(uint(4)),
	}
}

// serve stubs the routes the shared controller install calls.
func (f *controllerFixture) serve(t *testing.T) *controller.Reconciler {
	t.Helper()

	// answer on the machinetest server
	apiStub := machinetest.NewAPIStub(t)
	apiStub.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.route(t, w, r)
	})
	return &controller.Reconciler{APIServer: apiStub.Addr, APIClient: apiStub.Client}
}

// route answers one api call from the shared controller install.
func (f *controllerFixture) route(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	// serve the call this install makes
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathKubernetesWorkloadInstances+"/"):
		if f.missing || f.workload == nil {
			machinetest.WriteResponse(t, w, http.StatusNotFound, []apiserver_lib.Object{})
			return
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.workload})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, api.PathKubernetesWorkloadInstances+"/"):
		f.deleted = true
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.workload})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.runtime})
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
		var runtime api.KubernetesRuntimeInstance
		if err := json.NewDecoder(r.Body).Decode(&runtime); err != nil {
			t.Fatal(err)
		}
		if runtime.GatewayControllerInstanceID != nil {
			f.runtime.GatewayControllerInstanceID = runtime.GatewayControllerInstanceID
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.runtime})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeDefinitions+"/"):
		provider := api.KubernetesRuntimeInfraProviderKind
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{
			&api.KubernetesRuntimeDefinition{InfraProvider: &provider},
		})
	case r.Method == http.MethodGet && r.URL.Path == api.PathKubernetesWorkloadDefinitions:
		if f.definition == nil {
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{})
			return
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.definition})
	case r.Method == http.MethodPost && r.URL.Path == api.PathKubernetesWorkloadDefinitions:
		var created api.KubernetesWorkloadDefinition
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		f.definitions++
		f.definitionID++
		created.ID = util.Ptr(f.definitionID)
		f.definition = &created
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{f.definition})
	case r.Method == http.MethodPost && r.URL.Path == api.PathKubernetesWorkloadInstances:
		var created api.KubernetesWorkloadInstance
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		f.instances++
		f.instanceID++
		created.ID = util.Ptr(f.instanceID)
		f.workload = &created
		f.missing = false
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{f.workload})
	case r.Method == http.MethodGet && r.URL.Path == api.PathGatewayInstances:
		listed := make([]apiserver_lib.Object, len(f.gateways))
		for i := range f.gateways {
			listed[i] = &f.gateways[i]
		}
		machinetest.WriteResponse(t, w, http.StatusOK, listed)
	default:
		t.Errorf("unexpected %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	}
}

// TestConfirmGatewayControllerDeployedAlreadySet keeps a live controller install.
func TestConfirmGatewayControllerDeployedAlreadySet(t *testing.T) {
	// return before creating another install when the workload is live
	f := &controllerFixture{
		runtime:  kindRuntime(),
		workload: &api.KubernetesWorkloadInstance{Common: api.Common{ID: util.Ptr(uint(9))}},
	}
	f.runtime.GatewayControllerInstanceID = util.Ptr(uint(9))
	log := logr.Discard()
	err := confirmGatewayControllerDeployed(f.serve(t), f.runtime, &log)
	if err != nil {
		t.Fatal(err)
	}
	if f.definitions != 0 || f.instances != 0 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
}

// TestConfirmGatewayControllerDeployedCreatesOnePair installs one shared pair.
func TestConfirmGatewayControllerDeployedCreatesOnePair(t *testing.T) {
	// create one definition and one instance on the first gateway
	f := &controllerFixture{runtime: kindRuntime(), definitionID: 10, instanceID: 20}
	log := logr.Discard()
	reconciler := f.serve(t)
	err := confirmGatewayControllerDeployed(reconciler, f.runtime, &log)
	if err != nil {
		t.Fatal(err)
	}
	if f.definitions != 1 || f.instances != 1 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
	document := *f.definition.YAMLDocument
	if strings.Count(document, "kind: IngressComponent") != 1 {
		t.Fatalf("ingress components:\n%s", document)
	}
	if strings.Count(document, "kind: CertificatesComponent") != 1 {
		t.Fatalf("certificates components:\n%s", document)
	}
	if !strings.Contains(document, "name: ingress") || !strings.Contains(document, "name: certificates") {
		t.Fatalf("component names:\n%s", document)
	}

	// keep that install when another gateway reconciles
	err = confirmGatewayControllerDeployed(reconciler, f.runtime, &log)
	if err != nil {
		t.Fatal(err)
	}
	if f.definitions != 1 || f.instances != 1 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
}

// TestConfirmGatewayControllerDeployedReplacesMissing recreates a gone workload.
func TestConfirmGatewayControllerDeployedReplacesMissing(t *testing.T) {
	// reuse the definition and create a replacement instance
	f := &controllerFixture{
		runtime:    kindRuntime(),
		missing:    true,
		definition: &api.KubernetesWorkloadDefinition{Common: api.Common{ID: util.Ptr(uint(11))}},
		instanceID: 20,
	}
	f.runtime.GatewayControllerInstanceID = util.Ptr(uint(9))
	f.definition.Name = util.Ptr("ingress-dev")
	log := logr.Discard()
	err := confirmGatewayControllerDeployed(f.serve(t), f.runtime, &log)
	if err != nil {
		t.Fatal(err)
	}
	if f.definitions != 0 || f.instances != 1 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
	if f.runtime.GatewayControllerInstanceID == nil || *f.runtime.GatewayControllerInstanceID == 9 {
		t.Fatalf("runtime controller id %v", f.runtime.GatewayControllerInstanceID)
	}
}

// TestConfirmGatewayControllerDeployedWaitsWhileDeleting retries a deleting workload.
func TestConfirmGatewayControllerDeployedWaitsWhileDeleting(t *testing.T) {
	// return an error while the stored workload is still deleting
	now := time.Now()
	f := &controllerFixture{
		runtime: kindRuntime(),
		workload: &api.KubernetesWorkloadInstance{
			Common:         api.Common{ID: util.Ptr(uint(9))},
			Reconciliation: api.Reconciliation{DeletionScheduled: &now},
		},
	}
	f.runtime.GatewayControllerInstanceID = util.Ptr(uint(9))
	log := logr.Discard()
	err := confirmGatewayControllerDeployed(f.serve(t), f.runtime, &log)
	if err == nil {
		t.Fatal("expected an error")
	}
	if f.instances != 0 {
		t.Fatalf("instances %d", f.instances)
	}
}

// TestReleaseGatewayControllerKeepsSharedInstall leaves the install while a gateway remains.
func TestReleaseGatewayControllerKeepsSharedInstall(t *testing.T) {
	// skip deletion while one gateway on the runtime is still live
	now := time.Now()
	f := &controllerFixture{
		runtime:  kindRuntime(),
		workload: &api.KubernetesWorkloadInstance{Common: api.Common{ID: util.Ptr(uint(9))}},
		gateways: []api.GatewayInstance{
			{Reconciliation: api.Reconciliation{DeletionScheduled: &now}},
			{},
		},
	}
	f.runtime.GatewayControllerInstanceID = util.Ptr(uint(9))
	err := releaseGatewayController(f.serve(t), &api.GatewayInstance{
		KubernetesRuntimeInstanceID: f.runtime.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.deleted {
		t.Fatal("deleted the shared controller")
	}
}

// TestReleaseGatewayControllerDeletesLast removes the install with the last gateway.
func TestReleaseGatewayControllerDeletesLast(t *testing.T) {
	// delete the controller when every gateway on the runtime is deleting
	now := time.Now()
	f := &controllerFixture{
		runtime:  kindRuntime(),
		workload: &api.KubernetesWorkloadInstance{Common: api.Common{ID: util.Ptr(uint(9))}},
		gateways: []api.GatewayInstance{
			{Reconciliation: api.Reconciliation{DeletionScheduled: &now}},
		},
	}
	f.runtime.GatewayControllerInstanceID = util.Ptr(uint(9))
	err := releaseGatewayController(f.serve(t), &api.GatewayInstance{
		KubernetesRuntimeInstanceID: f.runtime.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !f.deleted {
		t.Fatal("shared controller was not deleted")
	}
}

// TestReleaseGatewayControllerNoController skips a runtime with no install.
func TestReleaseGatewayControllerNoController(t *testing.T) {
	// return before listing gateways when no controller id is stored
	f := &controllerFixture{runtime: kindRuntime()}
	err := releaseGatewayController(f.serve(t), &api.GatewayInstance{
		KubernetesRuntimeInstanceID: f.runtime.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.deleted {
		t.Fatal("deleted a controller that was not installed")
	}
}
