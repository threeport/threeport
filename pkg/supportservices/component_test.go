package supportservices

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// appendingEditor appends one extra document to the component manifest.
type appendingEditor struct{}

// GetFullyQualifiedType returns the test attacher type.
func (e *appendingEditor) GetFullyQualifiedType() string {
	return "appendingEditor"
}

// ObjectID returns no id.
func (e *appendingEditor) ObjectID() *uint {
	return nil
}

// EditManifest appends one extra document.
func (e *appendingEditor) EditManifest(base func() (string, error)) (string, error) {
	// append the extra document
	document, err := base()
	if err != nil {
		return "", err
	}
	return document + "\n---\nkind: Extra", nil
}

// TestComponentSlotNullOmitsDeletedAt checks the clear payload the api accepts.
func TestComponentSlotNullOmitsDeletedAt(t *testing.T) {
	// build a stored runtime with an association the update cannot carry
	id := uint(7)
	workloadID := uint(9)
	name := "runtime"
	runtime := &api.KubernetesRuntimeInstance{
		Common:                      api.Common{ID: &id},
		Instance:                    api.Instance{Name: &name},
		IngressComponentInstanceID:  &workloadID,
		KubernetesWorkloadInstances: []*api.KubernetesWorkloadInstance{{}},
	}

	// null the ingress column
	body, err := componentSlotNull(runtime, ingressColumn)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "KubernetesWorkloadInstances"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("%s present in %s", key, body)
		}
	}
	if fields[ingressColumn] != nil {
		t.Fatalf("ingress column %v", fields[ingressColumn])
	}
	if fields["Name"] != name {
		t.Fatalf("name %v", fields["Name"])
	}
	if runtime.ID == nil || *runtime.ID != id {
		t.Fatal("runtime id was cleared")
	}
}

// TestComponentSlotNullKeepsTheOtherSlotID keeps a sibling slot id exact.
func TestComponentSlotNullKeepsTheOtherSlotID(t *testing.T) {
	// use ids past the float64 integer range
	id := uint(7)
	ingressID := uint(1224220641336655873)
	certificatesID := uint(1224220645081022465)
	name := "runtime"
	runtime := &api.KubernetesRuntimeInstance{
		Common:                          api.Common{ID: &id},
		Instance:                        api.Instance{Name: &name},
		IngressComponentInstanceID:      &ingressID,
		CertificatesComponentInstanceID: &certificatesID,
	}

	// null the ingress column and leave the certificates id unchanged
	body, err := componentSlotNull(runtime, ingressColumn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "1224220645081022465") {
		t.Fatalf("certificates id changed in %s", body)
	}
}

// noopGatewayUpdate satisfies the update argument when the attached API instance is not stored.
func noopGatewayUpdate(*http.Client, string, *api.GatewayInstance) (*api.GatewayInstance, error) {
	// skip the write
	return nil, nil
}

// TestEnsureComponentRejectsAMissingRuntime reports the component whose runtime is absent.
func TestEnsureComponentRejectsAMissingRuntime(t *testing.T) {
	gateway := &api.GatewayInstance{}
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "ingress",
			call: func() error {
				return EnsureIngressComponent(nil, nil, gateway, nil, noopGatewayUpdate, nil)
			},
			want: "ingress component runtime is missing",
		},
		{
			name: "editor",
			call: func() error {
				return EnsureIngressComponent(nil, nil, &appendingEditor{}, nil, nil, nil)
			},
			want: "ingress component runtime is missing",
		},
		{
			name: "certificates",
			call: func() error {
				return EnsureCertificatesComponent(nil, nil, gateway, nil, noopGatewayUpdate, nil)
			},
			want: "certificates component runtime is missing",
		},
		{
			name: "secrets",
			call: func() error {
				return EnsureSecretsComponent(nil, nil, &api.SecretInstance{}, nil, nil, nil)
			},
			want: "secrets component runtime is missing",
		},
		{
			name: "database",
			call: func() error {
				return EnsureDatabaseComponent(nil, nil, gateway, nil, noopGatewayUpdate, nil)
			},
			want: "database component runtime is missing",
		},
		{
			name: "release ingress",
			call: func() error {
				return ReleaseIngressComponent(nil, nil, gateway)
			},
			want: "ingress component runtime is missing",
		},
		{
			name: "release certificates",
			call: func() error {
				return ReleaseCertificatesComponent(nil, nil, gateway)
			},
			want: "certificates component runtime is missing",
		},
		{
			name: "release secrets",
			call: func() error {
				return ReleaseSecretsComponent(nil, nil, &api.SecretInstance{})
			},
			want: "secrets component runtime is missing",
		},
		{
			name: "release database",
			call: func() error {
				return ReleaseDatabaseComponent(nil, nil, gateway)
			},
			want: "database component runtime is missing",
		},
	}
	// reject each component whose runtime is absent
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

// TestEnsureSecretsComponentInstalls creates the secrets workload and records its id.
// The install returns while that workload is still unreconciled.
func TestEnsureSecretsComponentInstalls(t *testing.T) {
	name := "dev"
	runtimeID := uint(1)
	definitionID := uint(11)
	workloadID := uint(22)
	runtime := &api.KubernetesRuntimeInstance{
		Common:   api.Common{ID: &runtimeID},
		Instance: api.Instance{Name: &name},
	}

	// serve a missing workload definition, then the created workload
	var calls []string
	stub := machinetest.NewAPIStub(t)
	stub.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == api.PathKubernetesWorkloadDefinitions:
			if r.URL.Query().Get("name") != SecretsComponentName+"-"+name {
				t.Fatalf("lookup %s", r.URL.RawQuery)
			}
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{})
		case r.Method == http.MethodPost && r.URL.Path == api.PathKubernetesWorkloadDefinitions:
			var got api.KubernetesWorkloadDefinition
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Name == nil || *got.Name != SecretsComponentName+"-"+name {
				t.Fatalf("definition name %v", got.Name)
			}
			if got.YAMLDocument == nil || *got.YAMLDocument == "" {
				t.Fatal("definition has no manifest")
			}
			got.ID = &definitionID
			machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{&got})
		case r.Method == http.MethodPost && r.URL.Path == api.PathKubernetesWorkloadInstances:
			var got api.KubernetesWorkloadInstance
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.KubernetesWorkloadDefinitionID == nil || *got.KubernetesWorkloadDefinitionID != definitionID {
				t.Fatalf("instance definition %v", got.KubernetesWorkloadDefinitionID)
			}
			if got.KubernetesRuntimeInstanceID == nil || *got.KubernetesRuntimeInstanceID != runtimeID {
				t.Fatalf("instance runtime %v", got.KubernetesRuntimeInstanceID)
			}
			got.ID = &workloadID
			machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{&got})
		case r.Method == http.MethodGet && r.URL.Path == api.PathKubernetesRuntimeInstances+"/1":
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{runtime})
		case r.Method == http.MethodPatch && r.URL.Path == api.PathKubernetesRuntimeInstances+"/1":
			var got api.KubernetesRuntimeInstance
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.SecretsComponentInstanceID == nil || *got.SecretsComponentInstanceID != workloadID {
				t.Fatalf("claimed slot %v", got.SecretsComponentInstanceID)
			}
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{&got})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.RequestURI())
		}
	})

	// install the secrets component
	log := logr.Discard()
	var recorded *uint
	err := EnsureSecretsComponent(
		&controller.Reconciler{APIClient: stub.Client, APIServer: stub.Addr},
		runtime,
		&api.SecretInstance{},
		&recorded,
		nil,
		&log,
	)
	want := fmt.Sprintf("secrets component workload %d not reconciled", workloadID)
	if err == nil || err.Error() != want {
		t.Fatalf("error %v, want %q", err, want)
	}

	// record the workload id on the runtime and the attached instance
	if runtime.SecretsComponentInstanceID == nil || *runtime.SecretsComponentInstanceID != workloadID {
		t.Fatalf("runtime slot %v", runtime.SecretsComponentInstanceID)
	}
	if recorded == nil || *recorded != workloadID {
		t.Fatalf("recorded id %v", recorded)
	}
	if len(calls) != 5 {
		t.Fatalf("calls %v", calls)
	}
}

// TestReleaseReturnsAnErrorWhenTheClearIsRejectedAndNoClaimRemains reports the
// rejected clear when no live requires row still names the component workload.
func TestReleaseReturnsAnErrorWhenTheClearIsRejectedAndNoClaimRemains(t *testing.T) {
	runtimeID := uint(1)
	workloadID := uint(7)
	gatewayID := uint(3)
	runtime := &api.KubernetesRuntimeInstance{
		Common:                     api.Common{ID: &runtimeID},
		IngressComponentInstanceID: &workloadID,
	}
	stub := machinetest.NewAPIStub(t)
	stub.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{runtime})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathAttachedObjectReferences):
			machinetest.WriteResponse(t, w, http.StatusOK, nil)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{
					Code:  http.StatusBadRequest,
					Error: api.ErrMsgSupportServiceSlotHeld,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.RequestURI())
		}
	})
	reconciler := &controller.Reconciler{APIServer: stub.Addr, APIClient: stub.Client}

	// release the component after the claim is already gone
	err := release(
		reconciler,
		runtimeID,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		ingressColumn,
		"threeport.io/v0.GatewayInstance",
		&gatewayID,
	)
	if err == nil || !strings.Contains(err.Error(), api.ErrMsgSupportServiceSlotHeld) {
		t.Fatalf("error %v, want %q", err, api.ErrMsgSupportServiceSlotHeld)
	}
}

// TestReleaseReturnsNilWhenTheClearIsRejectedAndAClaimRemains leaves the
// workload when another object still requires it.
func TestReleaseReturnsNilWhenTheClearIsRejectedAndAClaimRemains(t *testing.T) {
	runtimeID := uint(1)
	workloadID := uint(7)
	gatewayID := uint(3)
	otherGatewayID := uint(9)
	referenceID := uint(11)
	runtime := &api.KubernetesRuntimeInstance{
		Common:                     api.Common{ID: &runtimeID},
		IngressComponentInstanceID: &workloadID,
	}
	stub := machinetest.NewAPIStub(t)
	stub.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{runtime})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, api.PathAttachedObjectReferences):
			var references []apiserver_lib.Object
			if !strings.Contains(r.URL.RawQuery, "attachedobjectid=") {
				references = []apiserver_lib.Object{&api.AttachedObjectReference{
					ID:               &referenceID,
					ObjectID:         &workloadID,
					AttachedObjectID: &otherGatewayID,
				}}
			}
			machinetest.WriteResponse(t, w, http.StatusOK, references)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, api.PathKubernetesRuntimeInstances+"/"):
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{
					Code:  http.StatusBadRequest,
					Error: api.ErrMsgSupportServiceSlotHeld,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.RequestURI())
		}
	})
	reconciler := &controller.Reconciler{APIServer: stub.Addr, APIClient: stub.Client}

	// release the component while another gateway still requires the workload
	err := release(
		reconciler,
		runtimeID,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		ingressColumn,
		"threeport.io/v0.GatewayInstance",
		&gatewayID,
	)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != workloadID {
		t.Fatalf("slot = %v, want %d", runtime.IngressComponentInstanceID, workloadID)
	}
}

// TestReleaseComponentLeavesAnEmptySlot returns before deleting a workload.
func TestReleaseComponentLeavesAnEmptySlot(t *testing.T) {
	// release a runtime that stores no ingress workload id
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(9))}}
	err := release(
		nil,
		*runtime.ID,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		ingressColumn,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if runtime.IngressComponentInstanceID != nil {
		t.Fatalf("slot = %v, want empty", runtime.IngressComponentInstanceID)
	}
}

// TestAttacherManifestUsesAnEditor reports an API instance that edits a support services operator component manifest.
func TestAttacherManifestUsesAnEditor(t *testing.T) {
	// a gateway instance stores the workload id and keeps the stock ingress manifest
	if attacherManifest(&api.GatewayInstance{}) != nil {
		t.Fatal("gateway instance supplied an editor")
	}

	// an attached instance that edits the manifest supplies the synced document
	if attacherManifest(&appendingEditor{}) == nil {
		t.Fatal("editor supplied no document")
	}
}

// TestComponentDocumentAppendsTheEditor joins the component manifest with the editor output.
func TestComponentDocumentAppendsTheEditor(t *testing.T) {
	// build the synced document
	document, err := componentDocument(&appendingEditor{}, func() (string, error) {
		return "kind: Component", nil
	})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	if document != "kind: Component\n---\nkind: Extra" {
		t.Fatalf("document %q, want the edited manifest", document)
	}
}
