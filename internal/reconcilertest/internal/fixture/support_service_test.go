package fixture

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	client_v0 "github.com/threeport/threeport/internal/reconcilertest/pkg/client/v0"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	tpapi "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	"github.com/threeport/threeport/pkg/supportservices"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const supportRuntimeName = "dev"

// TestReconcilerEnsuresSupportServices runs the generated ensure calls for one runtime.
func TestReconcilerEnsuresSupportServices(t *testing.T) {
	// point the fixture at a runtime and serve its component routes
	h := newHarness(t)
	stub := newSupportStub(t, h)
	h.objMu.Lock()
	h.obj.KubernetesRuntimeInstanceID = stub.runtime.ID
	h.objMu.Unlock()

	// each publish is one pass, and the database pass is the fourth
	var posts int
	var databaseSlot *uint
	for pass := 0; pass < 8; pass++ {
		h.publish(notifications.NotificationOperationCreated)
		posts, _, databaseSlot = stub.progress()
		if posts == 4 && databaseSlot != nil {
			break
		}
	}
	require.Equal(t, 4, posts)
	require.NotNil(t, databaseSlot)

	// the create wrote each manifest once, before a later sync appends it
	require.Equal(t, 1, strings.Count(stub.createdYAML(supportservices.IngressComponentName+"-"+supportRuntimeName), "kind: IngressComponent"))
	require.Equal(t, 1, strings.Count(stub.createdYAML(supportservices.CertificatesComponentName+"-"+supportRuntimeName), "kind: CertificatesComponent"))
	require.Equal(t, 1, strings.Count(stub.createdYAML(supportservices.SecretsComponentName+"-"+supportRuntimeName), "kind: SecretsComponent"))
	require.Equal(t, 1, strings.Count(stub.createdYAML(supportservices.DatabaseComponentName+"-"+supportRuntimeName), "kind: DatabaseComponent"))
	require.Equal(t, 1, stub.countNamed(supportservices.IngressComponentName+"-"+supportRuntimeName))
	require.Equal(t, 1, stub.countNamed(supportservices.CertificatesComponentName+"-"+supportRuntimeName))
	require.Equal(t, 1, stub.countNamed(supportservices.SecretsComponentName+"-"+supportRuntimeName))
	require.Equal(t, 1, stub.countNamed(supportservices.DatabaseComponentName+"-"+supportRuntimeName))

	// a reconciled pass creates nothing more
	stored := *databaseSlot
	h.publish(notifications.NotificationOperationCreated)
	posts, _, databaseSlot = stub.progress()
	require.Equal(t, 4, posts)
	require.NotNil(t, databaseSlot)
	require.Equal(t, stored, *databaseSlot)
}

// TestReconcilerRejectsALostSupportServiceClaim deletes the workload a stale caller created.
// The reconciler reads the stored slot, so this caller still holds an empty one.
func TestReconcilerRejectsALostSupportServiceClaim(t *testing.T) {
	// create the ingress workload the stored runtime keeps
	h := newHarness(t)
	stub := newSupportStub(t, h)
	reconciler := &controller.Reconciler{APIServer: h.api.Addr, APIClient: h.api.Client}
	log := logr.Discard()
	err := supportservices.EnsureIngressComponent(
		reconciler,
		stub.runtime,
		h.obj,
		&h.obj.IngressComponentInstanceID,
		client_v0.UpdateReconcilerTestInstance,
		&log,
	)
	require.ErrorContains(t, err, "not reconciled")
	require.NotNil(t, stub.runtime.IngressComponentInstanceID)
	stored := *stub.runtime.IngressComponentInstanceID

	// claim again from a runtime copy that lost the slot
	stub.setRejectClaim(true)
	loser := *stub.runtime
	loser.IngressComponentInstanceID = nil
	var slot *uint
	err = supportservices.EnsureIngressComponent(
		reconciler,
		&loser,
		h.obj,
		&slot,
		client_v0.UpdateReconcilerTestInstance,
		&log,
	)
	require.ErrorContains(t, err, tpapi.ErrMsgSupportServiceSlotHeld)
	require.Equal(t, 1, stub.definitionCount())
	require.Equal(t, 2, stub.instancePosts())
	deleted := stub.deleted()
	require.Len(t, deleted, 1)
	require.NotEqual(t, stored, deleted[0])
	require.NotNil(t, stub.runtime.IngressComponentInstanceID)
	require.Equal(t, stored, *stub.runtime.IngressComponentInstanceID)
	require.Nil(t, loser.IngressComponentInstanceID)
	require.Nil(t, slot)
}

// supportStub records the component workloads the ensure calls create.
type supportStub struct {
	t                 *testing.T
	mu                sync.Mutex
	runtime           *tpapi.KubernetesRuntimeInstance
	runtimeDefinition *tpapi.KubernetesRuntimeDefinition
	definitions       []*tpapi.KubernetesWorkloadDefinition
	instances         []*tpapi.KubernetesWorkloadInstance
	resources         []*tpapi.KubernetesWorkloadResourceInstance
	created           map[string]string
	nextID            uint
	posts             int
	databaseLookups   int
	deletedIDs        []uint
	rejectClaim       bool
}

// newSupportStub registers the runtime and workload routes the ensure calls use.
func newSupportStub(t *testing.T, h *harness) *supportStub {
	t.Helper()

	// register the runtime the components install onto
	runtimeID := uint(7)
	definitionID := uint(8)
	name := supportRuntimeName
	provider := tpapi.KubernetesRuntimeInfraProviderKind
	stub := &supportStub{
		t:       t,
		created: map[string]string{},
		runtime: &tpapi.KubernetesRuntimeInstance{
			Common:                        tpapi.Common{ID: &runtimeID},
			Instance:                      tpapi.Instance{Name: &name},
			KubernetesRuntimeDefinitionID: &definitionID,
		},
		runtimeDefinition: &tpapi.KubernetesRuntimeDefinition{
			Common:        tpapi.Common{ID: &definitionID},
			Definition:    tpapi.Definition{Name: &name},
			InfraProvider: &provider,
		},
	}
	stub.bind(h.api.Mux, tpapi.PathKubernetesRuntimeInstances, stub.runtimeInstance)
	stub.bind(h.api.Mux, tpapi.PathKubernetesRuntimeDefinitions, stub.runtimeDefinitionByID)
	stub.bind(h.api.Mux, tpapi.PathKubernetesWorkloadDefinitions, stub.workloadDefinition)
	stub.bind(h.api.Mux, tpapi.PathKubernetesWorkloadInstances, stub.workloadInstance)
	stub.bind(h.api.Mux, tpapi.PathKubernetesWorkloadResourceDefinitions, stub.resourceDefinition)
	stub.bind(h.api.Mux, tpapi.PathKubernetesWorkloadResourceInstances, stub.resourceInstance)
	return stub
}

// bind registers the collection and the object route.
func (s *supportStub) bind(mux *http.ServeMux, path string, fn http.HandlerFunc) {
	mux.HandleFunc(path, fn)
	mux.HandleFunc(path+"/{id}", fn)
}

// progress returns the instance posts, the database-pass lookups, and the database slot.
func (s *supportStub) progress() (int, int, *uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts, s.databaseLookups, s.runtime.DatabaseComponentInstanceID
}

// createdYAML returns the workload document stored at create.
func (s *supportStub) createdYAML(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.created[name]
}

// countNamed counts workload instances with the name.
func (s *supportStub) countNamed(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	// count instances created under the name
	count := 0
	for _, instance := range s.instances {
		if instance.Name != nil && *instance.Name == name {
			count++
		}
	}
	return count
}

// definitionCount returns the stored workload definitions.
func (s *supportStub) definitionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.definitions)
}

// instancePosts returns how many workload instances were posted.
func (s *supportStub) instancePosts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts
}

// deleted returns the workload ids a lost claim removed.
func (s *supportStub) deleted() []uint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint(nil), s.deletedIDs...)
}

// setRejectClaim makes the next differing slot update fail.
func (s *supportStub) setRejectClaim(reject bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectClaim = reject
}

// runtimeInstance serves the runtime the component slot is claimed on.
func (s *supportStub) runtimeInstance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// serve the stored runtime, or store a slot update
	switch r.Method {
	case http.MethodGet:
		s.write(w, http.StatusOK, s.runtime)
	case http.MethodPatch, http.MethodPut:
		var incoming tpapi.KubernetesRuntimeInstance
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if s.rejectClaim && slotReplaces(s.runtime, &incoming) {
			s.held(w)
			return
		}
		copySlot(&s.runtime.IngressComponentInstanceID, incoming.IngressComponentInstanceID)
		copySlot(&s.runtime.CertificatesComponentInstanceID, incoming.CertificatesComponentInstanceID)
		copySlot(&s.runtime.SecretsComponentInstanceID, incoming.SecretsComponentInstanceID)
		copySlot(&s.runtime.DatabaseComponentInstanceID, incoming.DatabaseComponentInstanceID)
		s.write(w, http.StatusOK, s.runtime)
	default:
		http.NotFound(w, r)
	}
}

// runtimeDefinitionByID serves the stored runtime definition.
func (s *supportStub) runtimeDefinitionByID(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// return the one runtime definition
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	s.write(w, http.StatusOK, s.runtimeDefinition)
}

// workloadDefinition serves create, lookup by name, and the document update.
func (s *supportStub) workloadDefinition(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// create, list, or update the component definition
	idText := r.PathValue("id")
	switch {
	case r.Method == http.MethodPost && idText == "":
		var definition tpapi.KubernetesWorkloadDefinition
		if err := json.NewDecoder(r.Body).Decode(&definition); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := s.alloc()
		definition.ID = &id
		if definition.Name != nil && definition.YAMLDocument != nil {
			s.created[*definition.Name] = *definition.YAMLDocument
		}
		s.definitions = append(s.definitions, &definition)
		s.write(w, http.StatusCreated, &definition)
	case r.Method == http.MethodGet && idText == "":
		name := r.URL.Query().Get("name")
		var listed []apiserver_lib.Object
		for _, definition := range s.definitions {
			if name != "" && (definition.Name == nil || *definition.Name != name) {
				continue
			}
			listed = append(listed, definition)
		}
		s.write(w, http.StatusOK, listed...)
	case idText != "":
		definition := s.definition(idText)
		if definition == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			var incoming tpapi.KubernetesWorkloadDefinition
			if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if incoming.YAMLDocument != nil {
				definition.YAMLDocument = incoming.YAMLDocument
			}
		}
		s.write(w, http.StatusOK, definition)
	default:
		http.NotFound(w, r)
	}
}

// workloadInstance serves the component workload and keeps it reconciled.
func (s *supportStub) workloadInstance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// create, fetch, or delete the component workload
	idText := r.PathValue("id")
	switch {
	case r.Method == http.MethodPost && idText == "":
		var instance tpapi.KubernetesWorkloadInstance
		if err := json.NewDecoder(r.Body).Decode(&instance); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := s.alloc()
		instance.ID = &id
		s.instances = append(s.instances, &instance)
		s.posts++
		s.write(w, http.StatusCreated, &instance)
	case r.Method == http.MethodGet && idText == "":
		var listed []apiserver_lib.Object
		for _, instance := range s.instances {
			listed = append(listed, s.reconciled(instance))
		}
		s.write(w, http.StatusOK, listed...)
	case r.Method == http.MethodDelete && idText != "":
		instance := s.instance(idText)
		if id, err := strconv.ParseUint(idText, 10, 64); err == nil {
			s.deletedIDs = append(s.deletedIDs, uint(id))
		}
		if instance == nil {
			s.write(w, http.StatusOK)
			return
		}
		s.write(w, http.StatusOK, instance)
	case idText != "":
		instance := s.instance(idText)
		if instance == nil {
			http.NotFound(w, r)
			return
		}
		s.write(w, http.StatusOK, s.reconciled(instance))
	default:
		http.NotFound(w, r)
	}
}

// resourceDefinition accepts the resource definition a sync creates.
func (s *supportStub) resourceDefinition(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// store the resource definition and return its id
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var definition tpapi.KubernetesWorkloadResourceDefinition
	if err := json.NewDecoder(r.Body).Decode(&definition); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := s.alloc()
	definition.ID = &id
	s.write(w, http.StatusCreated, &definition)
}

// resourceInstance serves the resource list a sync compares.
func (s *supportStub) resourceInstance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// list the stored resources or store the one a sync posts
	idText := r.PathValue("id")
	switch {
	case r.Method == http.MethodPost && idText == "":
		var instance tpapi.KubernetesWorkloadResourceInstance
		if err := json.NewDecoder(r.Body).Decode(&instance); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := s.alloc()
		instance.ID = &id
		s.resources = append(s.resources, &instance)
		s.write(w, http.StatusCreated, &instance)
	case r.Method == http.MethodGet && idText == "":
		var listed []apiserver_lib.Object
		for _, instance := range s.resources {
			listed = append(listed, instance)
		}
		s.write(w, http.StatusOK, listed...)
	case idText != "":
		for _, instance := range s.resources {
			if instance.ID != nil && strconv.FormatUint(uint64(*instance.ID), 10) == idText {
				s.write(w, http.StatusOK, instance)
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

// alloc returns the next object id.
func (s *supportStub) alloc() uint {
	s.nextID++
	return s.nextID
}

// definition returns the stored definition for the id.
func (s *supportStub) definition(idText string) *tpapi.KubernetesWorkloadDefinition {
	for _, definition := range s.definitions {
		if definition.ID != nil && strconv.FormatUint(uint64(*definition.ID), 10) == idText {
			return definition
		}
	}
	return nil
}

// instance returns the stored workload for the id.
func (s *supportStub) instance(idText string) *tpapi.KubernetesWorkloadInstance {
	for _, instance := range s.instances {
		if instance.ID != nil && strconv.FormatUint(uint64(*instance.ID), 10) == idText {
			return instance
		}
	}
	return nil
}

// reconciled returns the workload with reconciliation forced on.
func (s *supportStub) reconciled(instance *tpapi.KubernetesWorkloadInstance) *tpapi.KubernetesWorkloadInstance {
	shown := *instance
	shown.Reconciled = util.Ptr(true)
	return &shown
}

// write sends one API response.
func (s *supportStub) write(w http.ResponseWriter, status int, objs ...apiserver_lib.Object) {
	if objs == nil {
		objs = []apiserver_lib.Object{}
	}
	machinetest.WriteResponse(s.t, w, status, objs)
}

// held rejects a claim that would replace a stored component id.
func (s *supportStub) held(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"Status": map[string]any{
			"error":   tpapi.ErrMsgSupportServiceSlotHeld,
			"Message": "Bad Request",
			"Code":    http.StatusBadRequest,
		},
	})
}

// slotReplaces reports that a patch would replace a stored component id.
func slotReplaces(stored, incoming *tpapi.KubernetesRuntimeInstance) bool {
	return slotDiffers(stored.IngressComponentInstanceID, incoming.IngressComponentInstanceID) ||
		slotDiffers(stored.CertificatesComponentInstanceID, incoming.CertificatesComponentInstanceID) ||
		slotDiffers(stored.SecretsComponentInstanceID, incoming.SecretsComponentInstanceID) ||
		slotDiffers(stored.DatabaseComponentInstanceID, incoming.DatabaseComponentInstanceID)
}

// slotDiffers reports that both ids are set and they are not equal.
func slotDiffers(stored, incoming *uint) bool {
	return stored != nil && incoming != nil && *stored != *incoming
}

// copySlot copies a component id when the patch includes one.
func copySlot(stored **uint, incoming *uint) {
	// keep a stored id when the patch omits the column
	if incoming != nil {
		*stored = incoming
	}
}
