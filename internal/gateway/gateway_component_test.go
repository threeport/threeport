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
	"github.com/threeport/threeport/pkg/supportservices"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// noopGatewayUpdate skips the caller write in a component test.
func noopGatewayUpdate(*http.Client, string, *api.GatewayInstance) (*api.GatewayInstance, error) {
	// skip the write
	return nil, nil
}

// componentFixture is the api state one gateway component install uses.
// The counts record definitions and instances the install creates.
type componentFixture struct {
	runtime      *api.KubernetesRuntimeInstance
	workload     *api.KubernetesWorkloadInstance
	definition   *api.KubernetesWorkloadDefinition
	gateways     []api.GatewayInstance
	missing      bool
	definitions  int
	instances    int
	deleted      bool
	holdSlot     bool
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

// serve stubs the routes the shared component install calls.
func (f *componentFixture) serve(t *testing.T) *controller.Reconciler {
	t.Helper()

	// answer on the machinetest server
	apiStub := machinetest.NewAPIStub(t)
	apiStub.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.route(t, w, r)
	})
	return &controller.Reconciler{APIServer: apiStub.Addr, APIClient: apiStub.Client}
}

// route answers one api call from the shared component install.
func (f *componentFixture) route(t *testing.T, w http.ResponseWriter, r *http.Request) {
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
		if runtime.IngressComponentInstanceID != nil {
			f.runtime.IngressComponentInstanceID = runtime.IngressComponentInstanceID
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
	case r.Method == http.MethodGet && r.URL.Path == api.PathAttachedObjectReferences:
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{})
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
		var fields map[string]any
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			t.Fatal(err)
		}
		slot, present := fields["IngressComponentInstanceID"]
		if present && slot == nil {
			if f.holdSlot {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"Status":{"error":"support service slot is held","Message":"Bad Request","Code":400}}`))
				return
			}
			f.runtime.IngressComponentInstanceID = nil
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.runtime})
	case r.Method == http.MethodGet && r.URL.Path == api.PathGatewayInstances:
		listed := make([]apiserver_lib.Object, len(f.gateways))
		for i := range f.gateways {
			listed[i] = &f.gateways[i]
		}
		machinetest.WriteResponse(t, w, http.StatusOK, listed)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathKubernetesWorkloadDefinitions+"/"):
		if f.definition == nil {
			yaml := ""
			f.definition = &api.KubernetesWorkloadDefinition{
				Common:       api.Common{ID: util.Ptr(uint(1))},
				YAMLDocument: &yaml,
			}
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.definition})
	case r.Method == http.MethodGet && r.URL.Path == api.PathKubernetesWorkloadResourceInstances:
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{})
	case r.Method == http.MethodPost && r.URL.Path == api.PathKubernetesWorkloadResourceDefinitions:
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{
			&api.KubernetesWorkloadResourceDefinition{Common: api.Common{ID: util.Ptr(uint(1))}},
		})
	case r.Method == http.MethodPost && r.URL.Path == api.PathKubernetesWorkloadResourceInstances:
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{
			&api.KubernetesWorkloadResourceInstance{Common: api.Common{ID: util.Ptr(uint(1))}},
		})
	case (r.Method == http.MethodPut || r.Method == http.MethodPatch) &&
		strings.HasPrefix(r.URL.Path, api.PathKubernetesWorkloadDefinitions+"/"):
		var updated api.KubernetesWorkloadDefinition
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			t.Fatal(err)
		}
		if f.definition == nil {
			f.definition = &updated
		} else if updated.YAMLDocument != nil {
			f.definition.YAMLDocument = updated.YAMLDocument
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.definition})
	case (r.Method == http.MethodPut || r.Method == http.MethodPatch) &&
		strings.HasPrefix(r.URL.Path, api.PathKubernetesWorkloadInstances+"/"):
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{f.workload})
	default:
		t.Errorf("unexpected %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	}
}

// TestEnsureIngressComponentAlreadySet keeps a live component install.
func TestEnsureIngressComponentAlreadySet(t *testing.T) {
	// return before creating another install when the workload is live
	f := &componentFixture{
		runtime: kindRuntime(),
		workload: &api.KubernetesWorkloadInstance{
			Common:                         api.Common{ID: util.Ptr(uint(9))},
			KubernetesWorkloadDefinitionID: util.Ptr(uint(1)),
		},
	}
	f.runtime.IngressComponentInstanceID = util.Ptr(uint(9))
	reconciled := true
	f.workload.Reconciled = &reconciled
	log := logr.Discard()
	reconciler := f.serve(t)
	gateway := &api.GatewayInstance{}
	err := supportservices.EnsureIngressComponent(reconciler, f.runtime, gateway, &gateway.IngressComponentInstanceID, noopGatewayUpdate, &log)
	if err != nil {
		t.Fatal(err)
	}
	if f.definitions != 0 || f.instances != 0 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
}

// TestEnsureIngressComponentCreatesOnePair installs one shared pair.
func TestEnsureIngressComponentCreatesOnePair(t *testing.T) {
	// create one definition and one instance on the first gateway
	f := &componentFixture{runtime: kindRuntime(), definitionID: 10, instanceID: 20}
	log := logr.Discard()
	reconciler := f.serve(t)
	gateway := &api.GatewayInstance{}
	err := supportservices.EnsureIngressComponent(reconciler, f.runtime, gateway, &gateway.IngressComponentInstanceID, noopGatewayUpdate, &log)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("expected the new workload to wait for reconciliation, got %v", err)
	}
	if f.definitions != 1 || f.instances != 1 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
	document := *f.definition.YAMLDocument
	if strings.Count(document, "kind: IngressComponent") != 1 {
		t.Fatalf("ingress components:\n%s", document)
	}
	if strings.Count(document, "kind: CertificatesComponent") != 0 {
		t.Fatalf("certificates components:\n%s", document)
	}
	if !strings.Contains(document, "name: ingress") || strings.Contains(document, "name: certificates") {
		t.Fatalf("component names:\n%s", document)
	}

	// keep that install once the workload has reconciled
	ready := true
	f.workload.Reconciled = &ready
	gateway = &api.GatewayInstance{}
	err = supportservices.EnsureIngressComponent(reconciler, f.runtime, gateway, &gateway.IngressComponentInstanceID, noopGatewayUpdate, &log)
	if err != nil {
		t.Fatal(err)
	}
	if f.definitions != 1 || f.instances != 1 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
}

// TestEnsureIngressComponentReplacesMissing recreates a gone workload.
func TestEnsureIngressComponentReplacesMissing(t *testing.T) {
	// reuse the definition and create a replacement instance
	f := &componentFixture{
		runtime:    kindRuntime(),
		missing:    true,
		definition: &api.KubernetesWorkloadDefinition{Common: api.Common{ID: util.Ptr(uint(11))}},
		instanceID: 20,
	}
	f.runtime.IngressComponentInstanceID = util.Ptr(uint(9))
	f.definition.Name = util.Ptr("ingress-dev")
	log := logr.Discard()
	reconciler := f.serve(t)
	gateway := &api.GatewayInstance{}
	err := supportservices.EnsureIngressComponent(reconciler, f.runtime, gateway, &gateway.IngressComponentInstanceID, noopGatewayUpdate, &log)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("expected the replacement to wait for reconciliation, got %v", err)
	}
	if f.definitions != 0 || f.instances != 1 {
		t.Fatalf("definitions %d instances %d", f.definitions, f.instances)
	}
	if f.runtime.IngressComponentInstanceID == nil || *f.runtime.IngressComponentInstanceID == 9 {
		t.Fatalf("runtime component id %v", f.runtime.IngressComponentInstanceID)
	}
}

// TestEnsureIngressComponentWaitsWhileDeleting retries a deleting workload.
func TestEnsureIngressComponentWaitsWhileDeleting(t *testing.T) {
	// return an error while the stored workload is still deleting
	now := time.Now()
	f := &componentFixture{
		runtime: kindRuntime(),
		workload: &api.KubernetesWorkloadInstance{
			Common:         api.Common{ID: util.Ptr(uint(9))},
			Reconciliation: api.Reconciliation{DeletionScheduled: &now},
		},
	}
	f.runtime.IngressComponentInstanceID = util.Ptr(uint(9))
	log := logr.Discard()
	reconciler := f.serve(t)
	gateway := &api.GatewayInstance{}
	err := supportservices.EnsureIngressComponent(reconciler, f.runtime, gateway, &gateway.IngressComponentInstanceID, noopGatewayUpdate, &log)
	if err == nil {
		t.Fatal("expected an error")
	}
	if f.instances != 0 {
		t.Fatalf("instances %d", f.instances)
	}
}
