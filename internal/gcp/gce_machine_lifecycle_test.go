package gcp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	logr "github.com/go-logr/logr"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	machine "github.com/threeport/threeport/internal/provider/machine"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
)

// gceTestInstanceID is the default API ID used by single-instance test cases.
const gceTestInstanceID uint = 42

// gceTestInstanceName is the default name used by single-instance test cases.
const gceTestInstanceName = "gce-test-instance"

// gceTestProviderID is the GCP provider ID referenced by test instances.
const gceTestProviderID uint = 7

// gceTestDefinitionID is the GCE definition ID referenced by test instances.
const gceTestDefinitionID uint = 11

// gceTestMachineRuntimeInstanceID is the married machine runtime instance ID
// referenced by test instances.
const gceTestMachineRuntimeInstanceID uint = 19

// gceAPIStub is an httptest.Server plus the client and address threeport
// client helpers expect. PATCH bodies are recorded keyed by path.
type gceAPIStub struct {
	server        *httptest.Server
	mux           *http.ServeMux
	client        *http.Client
	addr          string
	mu            sync.Mutex
	patches       map[string][][]byte
	encryptionKey string
}

// gceNewAPIStub returns a gceAPIStub with an empty mux. Addr drops the
// http:// scheme because the threeport client prepends one.
func gceNewAPIStub(t *testing.T) *gceAPIStub {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	return &gceAPIStub{
		server:        srv,
		mux:           mux,
		client:        &http.Client{},
		addr:          strings.TrimPrefix(srv.URL, "http://"),
		patches:       make(map[string][][]byte),
		encryptionKey: key,
	}
}

// gceReconciler builds a controller.Reconciler pointed at the stub.
func (s *gceAPIStub) gceReconciler() *controller.Reconciler {
	return &controller.Reconciler{
		APIClient:     s.client,
		APIServer:     s.addr,
		EncryptionKey: s.encryptionKey,
	}
}

// gceRecordPatch appends a captured PATCH body under path.
func (s *gceAPIStub) gceRecordPatch(path string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patches[path] = append(s.patches[path], body)
}

// gceCountAckPatches returns how many captured PATCH bodies for the path carry a
// non-nil CreationAcknowledged, counting how many times the path was acked.
func (s *gceAPIStub) gceCountAckPatches(t *testing.T, path string) int {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, body := range s.patches[path] {
		var updated v0.GcpGceMachineRuntimeInstance
		require.NoError(t, json.Unmarshal(body, &updated))
		if updated.CreationAcknowledged != nil {
			count++
		}
	}
	return count
}

// gceLastPatch returns the most recently captured PATCH body for the path.
func (s *gceAPIStub) gceLastPatch(t *testing.T, path string) v0.GcpGceMachineRuntimeInstance {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	bodies := s.patches[path]
	require.NotEmpty(t, bodies, "expected at least one PATCH to %s", path)
	var updated v0.GcpGceMachineRuntimeInstance
	require.NoError(t, json.Unmarshal(bodies[len(bodies)-1], &updated))
	return updated
}

// gceWriteResponse marshals data into an apiserver_lib.Response envelope and
// writes it with status.
func gceWriteResponse(t *testing.T, w http.ResponseWriter, status int, data []apiserver_lib.Object) {
	t.Helper()
	body, err := json.Marshal(apiserver_lib.Response{Data: data})
	require.NoError(t, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// gceInstancePath returns the GET/PATCH path for an instance ID.
func gceInstancePath(id uint) string {
	return fmt.Sprintf("%s/%d", v0.PathGcpGceMachineRuntimeInstances, id)
}

// gceProviderPath returns the GET path for a GCP provider ID.
func gceProviderPath(id uint) string {
	return fmt.Sprintf("%s/%d", v0.PathGcpProviders, id)
}

// gceHandleInstance registers a handler that returns get on GET and records
// the body on PATCH.
func (s *gceAPIStub) gceHandleInstance(t *testing.T, id uint, get *v0.GcpGceMachineRuntimeInstance) {
	t.Helper()
	path := gceInstancePath(id)
	s.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gceWriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{get})
		case http.MethodPatch:
			body, _ := io.ReadAll(r.Body)
			s.gceRecordPatch(path, body)
			var updated v0.GcpGceMachineRuntimeInstance
			require.NoError(t, json.Unmarshal(body, &updated))
			updated.ID = gcePtr(id)
			gceWriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{&updated})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

// gceHandleInstance500 registers an instance handler that always returns 500.
func (s *gceAPIStub) gceHandleInstance500(t *testing.T, id uint) {
	t.Helper()
	s.mux.HandleFunc(gceInstancePath(id), func(w http.ResponseWriter, r *http.Request) {
		gceWriteResponse(t, w, http.StatusInternalServerError, []apiserver_lib.Object{})
	})
}

// gceHandleProvider registers a GET handler for a GCP provider path.
func (s *gceAPIStub) gceHandleProvider(t *testing.T, id uint, provider *v0.GcpProvider) {
	t.Helper()
	s.mux.HandleFunc(gceProviderPath(id), func(w http.ResponseWriter, r *http.Request) {
		gceWriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{provider})
	})
}

// gceHandleProvider500 registers a GCP provider handler that returns 500.
func (s *gceAPIStub) gceHandleProvider500(t *testing.T, id uint) {
	t.Helper()
	s.mux.HandleFunc(gceProviderPath(id), func(w http.ResponseWriter, r *http.Request) {
		gceWriteResponse(t, w, http.StatusInternalServerError, []apiserver_lib.Object{})
	})
}

// gcePtr returns a pointer to its argument.
func gcePtr[T any](v T) *T {
	return &v
}

// gceTestSSHPrivateKeyPEM returns a PKCS1 PEM for rehydrate tests.
func gceTestSSHPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

// gceBaseInstance returns a minimal valid instance referencing the test
// provider and definition.
func gceBaseInstance(id uint, name string) *v0.GcpGceMachineRuntimeInstance {
	return &v0.GcpGceMachineRuntimeInstance{
		Common:                           v0.Common{ID: gcePtr(id)},
		Instance:                         v0.Instance{Name: gcePtr(name)},
		GcpProviderID:                    gcePtr(gceTestProviderID),
		GcpGceMachineRuntimeDefinitionID: gcePtr(gceTestDefinitionID),
	}
}

// gceBaseDefinition returns a GCE definition carrying the fields
// buildGceMachineInfra copies onto infra.
func gceBaseDefinition() *v0.GcpGceMachineRuntimeDefinition {
	return &v0.GcpGceMachineRuntimeDefinition{
		Common:      v0.Common{ID: gcePtr(gceTestDefinitionID)},
		Definition:  v0.Definition{Name: gcePtr("gce-test-definition")},
		MachineType: gcePtr("e2-medium"),
		ImageID:     gcePtr("debian-12"),
	}
}

// gceDefinitionPath returns the GET path for a GCE definition ID.
func gceDefinitionPath(id uint) string {
	return fmt.Sprintf("%s/%d", v0.PathGcpGceMachineRuntimeDefinitions, id)
}

// gceHandleDefinition registers a GET handler for a GCE definition path.
func (s *gceAPIStub) gceHandleDefinition(t *testing.T, id uint, def *v0.GcpGceMachineRuntimeDefinition) {
	t.Helper()
	s.mux.HandleFunc(gceDefinitionPath(id), func(w http.ResponseWriter, r *http.Request) {
		gceWriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{def})
	})
}

// gceHandleDefinition500 registers a GCE definition handler that returns 500.
func (s *gceAPIStub) gceHandleDefinition500(t *testing.T, id uint) {
	t.Helper()
	s.mux.HandleFunc(gceDefinitionPath(id), func(w http.ResponseWriter, r *http.Request) {
		gceWriteResponse(t, w, http.StatusInternalServerError, []apiserver_lib.Object{})
	})
}

// gceMachineRuntimeInstancePath returns the GET/PATCH path for a machine
// runtime instance ID.
func gceMachineRuntimeInstancePath(id uint) string {
	return fmt.Sprintf("%s/%d", v0.PathMachineRuntimeInstances, id)
}

// gceHandleMachineRuntimeInstance registers a handler that returns get on GET
// and records the body on PATCH.
func (s *gceAPIStub) gceHandleMachineRuntimeInstance(t *testing.T, id uint, get *v0.MachineRuntimeInstance) {
	t.Helper()
	path := gceMachineRuntimeInstancePath(id)
	s.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gceWriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{get})
		case http.MethodPatch:
			body, _ := io.ReadAll(r.Body)
			s.gceRecordPatch(path, body)
			var updated v0.MachineRuntimeInstance
			require.NoError(t, json.Unmarshal(body, &updated))
			updated.ID = gcePtr(id)
			gceWriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{&updated})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

// gceLastMachineRuntimeInstancePatch returns the most recently captured PATCH
// body for the machine runtime instance path.
func (s *gceAPIStub) gceLastMachineRuntimeInstancePatch(t *testing.T, path string) v0.MachineRuntimeInstance {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	bodies := s.patches[path]
	require.NotEmpty(t, bodies, "expected at least one PATCH to %s", path)
	var updated v0.MachineRuntimeInstance
	require.NoError(t, json.Unmarshal(bodies[len(bodies)-1], &updated))
	return updated
}

// gceBaseMachineRuntimeInstance returns a married machine runtime instance.
func gceBaseMachineRuntimeInstance(id uint, name string) *v0.MachineRuntimeInstance {
	return &v0.MachineRuntimeInstance{
		Common:   v0.Common{ID: gcePtr(id)},
		Instance: v0.Instance{Name: gcePtr(name)},
	}
}

// gceBaseProvider returns a GCP provider with project ID set.
func gceBaseProvider() *v0.GcpProvider {
	return &v0.GcpProvider{
		Common:                    v0.Common{ID: gcePtr(gceTestProviderID)},
		Name:                      gcePtr("test-provider"),
		ProjectID:                 gcePtr("test-project"),
		ServiceAccountCredentials: nil,
	}
}

// gceFakeInfra is a provider.InfraProvider that is not *machine.GceMachineInfra.
type gceFakeInfra struct{}

func (gceFakeInfra) DeployInfra() error                      { return nil }
func (gceFakeInfra) DestroyInfra() error                     { return nil }
func (gceFakeInfra) SetStackState(_ *datatypes.JSON) error   { return nil }
func (gceFakeInfra) GetStackState() (*datatypes.JSON, error) { return nil, nil }

var _ provider.InfraProvider = gceFakeInfra{}

// gceFakeJetStream is a nats.JetStreamContext that records Publish subjects
// and returns a configured error. Other methods on the embedded nil must not be called.
type gceFakeJetStream struct {
	nats.JetStreamContext
	err      error
	subjects []string
}

// Publish records the subject and returns the configured error.
func (f *gceFakeJetStream) Publish(subj string, _ []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.subjects = append(f.subjects, subj)
	if f.err != nil {
		return nil, f.err
	}
	return &nats.PubAck{}, nil
}

// gceNewLifecycle constructs the adapter against the stub for a given instance.
func gceNewLifecycle(s *gceAPIStub, instance *v0.GcpGceMachineRuntimeInstance) *gceMachineLifecycle {
	log := logr.Discard()
	return newGceMachineLifecycleProvider(s.gceReconciler(), instance, &log)
}

// TestGceLifecycleGetReconciliation covers GetReconciliation snapshot mapping,
// a nil CreationFailed as false, and a wrapped GET 500.
func TestGceLifecycleGetReconciliation(t *testing.T) {
	t.Run("happy mapping", func(t *testing.T) {
		s := gceNewAPIStub(t)
		now := time.Now().UTC()
		inventory := datatypes.JSON([]byte(`{"a":1}`))
		latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
		latest.CreationAcknowledged = gcePtr(now)
		latest.CreationConfirmed = gcePtr(now)
		latest.CreationFailed = gcePtr(true)
		latest.DeletionScheduled = gcePtr(now)
		latest.DeletionAcknowledged = gcePtr(now)
		latest.DeletionConfirmed = gcePtr(now)
		latest.ResourceInventory = &inventory
		s.gceHandleInstance(t, gceTestInstanceID, latest)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		snap, err := g.GetReconciliation()
		require.NoError(t, err)
		assert.True(t, snap.CreationFailed)
		assert.NotNil(t, snap.CreationAcknowledged)
		assert.NotNil(t, snap.CreationConfirmed)
		assert.NotNil(t, snap.DeletionScheduled)
		assert.NotNil(t, snap.DeletionAcknowledged)
		assert.NotNil(t, snap.DeletionConfirmed)
		require.NotNil(t, snap.ResourceInventory)
		assert.JSONEq(t, `{"a":1}`, string(*snap.ResourceInventory))
	})

	t.Run("nil CreationFailed reads false", func(t *testing.T) {
		s := gceNewAPIStub(t)
		latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
		latest.CreationFailed = nil
		s.gceHandleInstance(t, gceTestInstanceID, latest)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		snap, err := g.GetReconciliation()
		require.NoError(t, err)
		assert.False(t, snap.CreationFailed)
	})

	t.Run("GET 500 wraps error", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance500(t, gceTestInstanceID)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.GetReconciliation()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get latest GCE instance")
	})
}

// TestGceLifecycleBuildInfra covers BuildInfra field copy, decrypt, and GET
// failures for instance, provider, and definition.
func TestGceLifecycleBuildInfra(t *testing.T) {
	t.Run("happy path populates fields", func(t *testing.T) {
		s := gceNewAPIStub(t)
		latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
		latest.Region = gcePtr("us-central1")
		latest.Zone = gcePtr("us-central1-a")
		latest.SSHUser = gcePtr("threeport")
		s.gceHandleInstance(t, gceTestInstanceID, latest)
		prov := gceBaseProvider()
		enc, err := encryption.Encrypt(s.encryptionKey, "creds-json")
		require.NoError(t, err)
		prov.ServiceAccountCredentials = gcePtr(enc)
		s.gceHandleProvider(t, gceTestProviderID, prov)
		s.gceHandleDefinition(t, gceTestDefinitionID, gceBaseDefinition())

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		infra, err := g.BuildInfra()
		require.NoError(t, err)
		gceInfra, ok := infra.(*machine.GceMachineInfra)
		require.True(t, ok)
		assert.Equal(t, "test-project", gceInfra.ProjectID)
		assert.Equal(t, "us-central1", gceInfra.Region)
		assert.Equal(t, "us-central1-a", gceInfra.Zone)
		assert.Equal(t, "e2-medium", gceInfra.MachineType)
		assert.Equal(t, "debian-12", gceInfra.ImageID)
		assert.Equal(t, "threeport", gceInfra.SSHUser)
		assert.Equal(t, "creds-json", gceInfra.ServiceAccountCredentials)
		require.NotNil(t, gceInfra.PersistSSHKey)
		require.NoError(t, gceInfra.PersistSSHKey("PRIVATE-KEY"))
		patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
		require.NotNil(t, patch.SSHKey)
		assert.Equal(t, "PRIVATE-KEY", *patch.SSHKey)
	})

	t.Run("rehydrates encrypted SSHKey on rebuild", func(t *testing.T) {
		s := gceNewAPIStub(t)
		priv := gceTestSSHPrivateKeyPEM(t)
		enc, err := encryption.Encrypt(s.encryptionKey, priv)
		require.NoError(t, err)
		latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
		latest.SSHKey = gcePtr(enc)
		s.gceHandleInstance(t, gceTestInstanceID, latest)
		prov := gceBaseProvider()
		provEnc, err := encryption.Encrypt(s.encryptionKey, "creds-json")
		require.NoError(t, err)
		prov.ServiceAccountCredentials = gcePtr(provEnc)
		s.gceHandleProvider(t, gceTestProviderID, prov)
		s.gceHandleDefinition(t, gceTestDefinitionID, gceBaseDefinition())

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		infra, err := g.BuildInfra()
		require.NoError(t, err)
		gceInfra := infra.(*machine.GceMachineInfra)
		_, _, got := gceInfra.CreateOutputs()
		assert.Equal(t, priv, got)
	})

	t.Run("instance GET fails", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance500(t, gceTestInstanceID)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.BuildInfra()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get GCE instance for infra build")
	})

	t.Run("provider GET fails", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		s.gceHandleProvider500(t, gceTestProviderID)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.BuildInfra()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to retrieve GCP provider by ID")
	})

	t.Run("empty service account credentials rejected", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		prov := gceBaseProvider()
		// reject empty credentials at BuildInfra so a misconfigured provider
		// fails before later steps fall through to ambient ADC
		prov.ServiceAccountCredentials = gcePtr("")
		s.gceHandleProvider(t, gceTestProviderID, prov)
		s.gceHandleDefinition(t, gceTestDefinitionID, gceBaseDefinition())

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.BuildInfra()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no service account credentials")
	})

	t.Run("nil service account credentials rejected", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		// gceBaseProvider leaves ServiceAccountCredentials nil; reject nil
		// the same as an empty string
		s.gceHandleProvider(t, gceTestProviderID, gceBaseProvider())
		s.gceHandleDefinition(t, gceTestDefinitionID, gceBaseDefinition())

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.BuildInfra()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no service account credentials")
	})

	t.Run("definition GET fails", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		s.gceHandleProvider(t, gceTestProviderID, gceBaseProvider())
		s.gceHandleDefinition500(t, gceTestDefinitionID)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.BuildInfra()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to retrieve GCE machine runtime definition by ID")
	})
}

// TestGceBuildInfraNilRequiredFields covers buildGceMachineInfra errors for
// nil GcpProviderID, ProjectID, and GcpGceMachineRuntimeDefinitionID.
func TestGceBuildInfraNilRequiredFields(t *testing.T) {
	t.Run("nil GcpProviderID returns clean error", func(t *testing.T) {
		s := gceNewAPIStub(t)
		instance := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
		instance.GcpProviderID = nil

		_, err := buildGceMachineInfra(s.gceReconciler(), instance, gceDiscardLog())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gcp provider id is empty")
	})

	t.Run("nil provider ProjectID returns clean error", func(t *testing.T) {
		s := gceNewAPIStub(t)
		prov := gceBaseProvider()
		prov.ProjectID = nil
		s.gceHandleProvider(t, gceTestProviderID, prov)

		_, err := buildGceMachineInfra(s.gceReconciler(), gceBaseInstance(gceTestInstanceID, gceTestInstanceName), gceDiscardLog())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gcp project id is empty")
	})

	t.Run("nil GcpGceMachineRuntimeDefinitionID returns clean error", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleProvider(t, gceTestProviderID, gceBaseProvider())
		instance := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
		instance.GcpGceMachineRuntimeDefinitionID = nil

		_, err := buildGceMachineInfra(s.gceReconciler(), instance, gceDiscardLog())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gce machine runtime definition id is empty")
	})
}

// gceDiscardLog returns a discard logger pointer for direct builder calls.
func gceDiscardLog() *logr.Logger {
	log := logr.Discard()
	return &log
}

// TestGceLifecycleIsCreateComplete covers IsCreateComplete inventory cases
// and a wrapped GET error.
func TestGceLifecycleIsCreateComplete(t *testing.T) {
	// inventory bytes must be valid JSON: the stub marshals them through
	// datatypes.JSON, which refuses invalid bytes
	cases := []struct {
		name      string
		inventory *datatypes.JSON
		want      bool
	}{
		{"nil inventory", nil, false},
		{"empty object", gceJSON("{}"), false},
		{"json null literal", gceJSON("null"), false},
		{"populated object", gceJSON(`{"a":1}`), true},
		{"padded empty object", gceJSON(" {} "), false},
		{"tabbed empty object", gceJSON("\t{}\n"), false},
		{"empty array", gceJSON("[]"), false},
		{"quoted null", gceJSON(`"null"`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := gceNewAPIStub(t)
			latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
			latest.ResourceInventory = tc.inventory
			s.gceHandleInstance(t, gceTestInstanceID, latest)

			g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
			got, err := g.IsCreateComplete()
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("GET error", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance500(t, gceTestInstanceID)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		_, err := g.IsCreateComplete()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check GCE creation status")
	})
}

// gceJSON returns a pointer to a datatypes.JSON from s.
func gceJSON(s string) *datatypes.JSON {
	j := datatypes.JSON([]byte(s))
	return &j
}

// TestGceSaveCreateOutputsWritesHostnameIPKey covers SaveCreateOutputs writing
// hostname, external IP, SSH key, and inventory onto the instance PATCH.
func TestGceSaveCreateOutputsWritesHostnameIPKey(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	gceInfra := machine.NewGceMachineInfra(gceTestInstanceName)
	gceInfra.SetCreateOutputs("vm.example", "203.0.113.7", "PRIVATE-KEY")
	state := datatypes.JSON([]byte(`{"checkpoint":{}}`))

	require.NoError(t, g.SaveCreateOutputs(gceInfra, &state))

	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.Hostname)
	assert.Equal(t, "vm.example", *patch.Hostname)
	require.NotNil(t, patch.ExternalIP)
	assert.Equal(t, "203.0.113.7", *patch.ExternalIP)
	require.NotNil(t, patch.SSHKey)
	assert.Equal(t, "PRIVATE-KEY", *patch.SSHKey)
	require.NotNil(t, patch.ResourceInventory)
	assert.JSONEq(t, `{"checkpoint":{}}`, string(*patch.ResourceInventory))
}

// TestGceSaveCreateOutputsUpdateErrorWraps covers SaveCreateOutputs wrapping a
// PATCH 500.
func TestGceSaveCreateOutputsUpdateErrorWraps(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance500(t, gceTestInstanceID)

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	gceInfra := machine.NewGceMachineInfra(gceTestInstanceName)
	gceInfra.SetCreateOutputs("h", "ip", "k")
	state := datatypes.JSON([]byte(`{}`))

	err := g.SaveCreateOutputs(gceInfra, &state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update GCE instance with create outputs")
}

// TestGceSaveCreateOutputsWrongConcreteTypeReturnsError covers SaveCreateOutputs
// rejecting a provider.InfraProvider that is not *machine.GceMachineInfra.
func TestGceSaveCreateOutputsWrongConcreteTypeReturnsError(t *testing.T) {
	s := gceNewAPIStub(t)
	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	state := datatypes.JSON([]byte(`{}`))

	err := g.SaveCreateOutputs(gceFakeInfra{}, &state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected *machine.GceMachineInfra")
}

// TestGceLifecycleAckCreation covers AckCreation setting CreationAcknowledged
// and clearing CreationFailed, and a PATCH error.
func TestGceLifecycleAckCreation(t *testing.T) {
	t.Run("sets acknowledged and clears failed", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		require.NoError(t, g.AckCreation())

		patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
		require.NotNil(t, patch.CreationAcknowledged)
		assert.WithinDuration(t, time.Now().UTC(), *patch.CreationAcknowledged, time.Minute)
		require.NotNil(t, patch.CreationFailed)
		assert.False(t, *patch.CreationFailed)
	})

	t.Run("PATCH error propagates", func(t *testing.T) {
		s := gceNewAPIStub(t)
		s.gceHandleInstance500(t, gceTestInstanceID)

		g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
		require.Error(t, g.AckCreation())
	})
}

// TestGceLifecycleRefreshCreationAck covers RefreshCreationAck writing
// CreationAcknowledged and a PATCH error.
func TestGceLifecycleRefreshCreationAck(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.RefreshCreationAck())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.CreationAcknowledged)

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.RefreshCreationAck())
}

// TestGceLifecycleSetCreationFailed covers SetCreationFailed writing
// CreationFailed true and a PATCH error.
func TestGceLifecycleSetCreationFailed(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.SetCreationFailed())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.CreationFailed)
	assert.True(t, *patch.CreationFailed)

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.SetCreationFailed())
}

// TestGceLifecycleConfirmCreation covers ConfirmCreation writing Reconciled
// and CreationConfirmed, and a PATCH error.
func TestGceLifecycleConfirmCreation(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.ConfirmCreation())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.Reconciled)
	assert.True(t, *patch.Reconciled)
	require.NotNil(t, patch.CreationConfirmed)

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.ConfirmCreation())
}

// TestGceLifecycleAckDeletion covers AckDeletion writing DeletionAcknowledged
// and a PATCH error.
func TestGceLifecycleAckDeletion(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.AckDeletion())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.DeletionAcknowledged)

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.AckDeletion())
}

// TestGceLifecycleRefreshDeletionAck covers RefreshDeletionAck writing
// DeletionAcknowledged and a PATCH error.
func TestGceLifecycleRefreshDeletionAck(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.RefreshDeletionAck())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.DeletionAcknowledged)

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.RefreshDeletionAck())
}

// TestGceLifecycleConfirmDeletion covers ConfirmDeletion writing
// DeletionConfirmed and a PATCH error.
func TestGceLifecycleConfirmDeletion(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.ConfirmDeletion())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.DeletionConfirmed)

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.ConfirmDeletion())
}

// TestGceLifecycleSaveState covers SaveState writing ResourceInventory and a
// PATCH error.
func TestGceLifecycleSaveState(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	state := datatypes.JSON([]byte(`{"deployment":{}}`))
	require.NoError(t, g.SaveState(&state))
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.ResourceInventory)
	assert.JSONEq(t, `{"deployment":{}}`, string(*patch.ResourceInventory))

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.SaveState(&state))
}

// TestGceLifecycleClearInventory covers ClearInventory writing "{}" as
// ResourceInventory and a PATCH error.
func TestGceLifecycleClearInventory(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.ClearInventory())
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.ResourceInventory)
	assert.Equal(t, "{}", string(*patch.ResourceInventory))

	s500 := gceNewAPIStub(t)
	s500.gceHandleInstance500(t, gceTestInstanceID)
	g500 := gceNewLifecycle(s500, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.Error(t, g500.ClearInventory())
}

// gceFakeOrphanCloud is an orphanReclaimCloud whose presence, probe errors,
// delete errors, and post-delete lingering are configurable, so the reclaim
// logic is driven through every branch without a live compute API. It records
// probe and delete counts so a test can assert the probe-delete-reprobe shape.
type gceFakeOrphanCloud struct {
	instancePresent  bool
	firewallPresent  bool
	instanceProbeErr error
	firewallProbeErr error
	instanceDelErr   error
	firewallDelErr   error
	instanceLingers  bool
	firewallLingers  bool
	instanceProbes   int
	firewallProbes   int
	instanceDeletes  int
	firewallDeletes  int
}

func (f *gceFakeOrphanCloud) instanceExists() (bool, error) {
	f.instanceProbes++
	if f.instanceProbeErr != nil {
		return false, f.instanceProbeErr
	}
	return f.instancePresent, nil
}

func (f *gceFakeOrphanCloud) deleteInstance() error {
	f.instanceDeletes++
	if f.instanceDelErr != nil {
		return f.instanceDelErr
	}
	if !f.instanceLingers {
		f.instancePresent = false
	}
	return nil
}

func (f *gceFakeOrphanCloud) firewallExists() (bool, error) {
	f.firewallProbes++
	if f.firewallProbeErr != nil {
		return false, f.firewallProbeErr
	}
	return f.firewallPresent, nil
}

func (f *gceFakeOrphanCloud) deleteFirewall() error {
	f.firewallDeletes++
	if f.firewallDelErr != nil {
		return f.firewallDelErr
	}
	if !f.firewallLingers {
		f.firewallPresent = false
	}
	return nil
}

func TestGceLifecycleOnDeleteConfirmedRejectsWrongInfraType(t *testing.T) {
	// the post-destroy reclaim needs the concrete GCE infra to address the VM;
	// a nil or foreign infra must refuse rather than confirm a deletion it
	// cannot validate against the cloud
	s := gceNewAPIStub(t)
	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	// a nil infra is rejected
	require.Error(t, g.OnDeleteConfirmed(nil))
	// a foreign concrete type is rejected with a message naming the wanted type
	err := g.OnDeleteConfirmed(gceFakeInfra{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected *machine.GceMachineInfra")
}

func TestGceNewComputeOrphanReclaimCloudRejectsMissingCoordinates(t *testing.T) {
	// building the reclaim client against an infra missing its addressing fields
	// must fail before any cloud call, so a false not-found cannot read as gone
	gceInfra := machine.NewGceMachineInfra("")
	gceInfra.ProjectID = ""
	gceInfra.Zone = ""

	_, err := newComputeOrphanReclaimCloud(gceInfra)
	require.Error(t, err)
	// the accumulated error names every missing coordinate
	assert.Contains(t, err.Error(), "ProjectID")
	assert.Contains(t, err.Error(), "Zone")
	assert.Contains(t, err.Error(), "RuntimeInstanceName")
}

func TestReclaimOrphansNoSurvivorsConfirms(t *testing.T) {
	// the common no-drift path: neither resource present, so the reclaim probes
	// once each, deletes nothing, and returns nil to let deletion confirm
	cloud := &gceFakeOrphanCloud{}
	require.NoError(t, reclaimOrphans(cloud))
	assert.Equal(t, 1, cloud.instanceProbes)
	assert.Equal(t, 1, cloud.firewallProbes)
	assert.Equal(t, 0, cloud.instanceDeletes)
	assert.Equal(t, 0, cloud.firewallDeletes)
}

func TestReclaimOrphansDeletesSurvivingInstance(t *testing.T) {
	// an instance the destroy abandoned is deleted, then re-probed gone, so the
	// reclaim returns nil and lets deletion confirm
	cloud := &gceFakeOrphanCloud{instancePresent: true}
	require.NoError(t, reclaimOrphans(cloud))
	// the instance is deleted once and probed before and after the delete
	assert.Equal(t, 1, cloud.instanceDeletes)
	assert.Equal(t, 2, cloud.instanceProbes)
}

func TestReclaimOrphansDeletesSurvivingFirewall(t *testing.T) {
	// a firewall the destroy abandoned is deleted and re-probed gone
	cloud := &gceFakeOrphanCloud{firewallPresent: true}
	require.NoError(t, reclaimOrphans(cloud))
	assert.Equal(t, 1, cloud.firewallDeletes)
	assert.Equal(t, 2, cloud.firewallProbes)
}

func TestReclaimOrphansRejectsDeleteFailure(t *testing.T) {
	// a delete that errors surfaces so the teardown requeues instead of
	// confirming a deletion that left the instance live
	cloud := &gceFakeOrphanCloud{instancePresent: true, instanceDelErr: fmt.Errorf("delete denied")}
	err := reclaimOrphans(cloud)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete orphaned instance")
}

func TestReclaimOrphansRejectsResourceLingeringAfterDelete(t *testing.T) {
	// a delete the cloud accepts but that leaves the resource present is caught
	// by the re-probe and returns an error so the teardown retries
	cloud := &gceFakeOrphanCloud{instancePresent: true, instanceLingers: true}
	err := reclaimOrphans(cloud)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still present after delete")
}

func TestReclaimOrphansRejectsProbeFailure(t *testing.T) {
	// a probe that errors surfaces and no delete is attempted, so a transient
	// cloud failure requeues rather than confirming on incomplete information
	cloud := &gceFakeOrphanCloud{instanceProbeErr: fmt.Errorf("api down")}
	err := reclaimOrphans(cloud)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to probe instance")
	assert.Equal(t, 0, cloud.instanceDeletes)
}

// TestGceLifecycleOnCreateConfirmedWritesHostnameSSHOntoMarriedInstance covers
// OnCreateConfirmed writing ExternalIP onto Hostname with SSHUser, SSHKey, and Reconciled false.
func TestGceLifecycleOnCreateConfirmedWritesHostnameSSHOntoMarriedInstance(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.MachineRuntimeInstanceID = gcePtr(gceTestMachineRuntimeInstanceID)
	latest.ExternalIP = gcePtr("203.0.113.7")
	latest.SSHUser = gcePtr("threeport")
	latest.SSHKey = gcePtr("PRIVATE-KEY")
	s.gceHandleInstance(t, gceTestInstanceID, latest)
	s.gceHandleMachineRuntimeInstance(t, gceTestMachineRuntimeInstanceID, gceBaseMachineRuntimeInstance(gceTestMachineRuntimeInstanceID, gceTestInstanceName))

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	require.NoError(t, g.OnCreateConfirmed(nil))

	// inspect the PATCH sent to the married machine runtime instance
	patch := s.gceLastMachineRuntimeInstancePatch(t, gceMachineRuntimeInstancePath(gceTestMachineRuntimeInstanceID))
	require.NotNil(t, patch.Hostname)
	assert.Equal(t, "203.0.113.7", *patch.Hostname)
	require.NotNil(t, patch.SSHUser)
	assert.Equal(t, "threeport", *patch.SSHUser)
	require.NotNil(t, patch.SSHKey)
	assert.Equal(t, "PRIVATE-KEY", *patch.SSHKey)
	require.NotNil(t, patch.Reconciled)
	assert.False(t, *patch.Reconciled)
}

// TestGceLifecycleOnCreateConfirmedNilMarriedIDReturnsError covers
// OnCreateConfirmed rejecting a nil MachineRuntimeInstanceID.
func TestGceLifecycleOnCreateConfirmedNilMarriedIDReturnsError(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.MachineRuntimeInstanceID = nil
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	err := g.OnCreateConfirmed(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "machine runtime instance id is empty")
}

// TestGceLifecycleOnCreateConfirmedInstanceGETErrorWraps covers
// OnCreateConfirmed wrapping a GCE instance GET 500.
func TestGceLifecycleOnCreateConfirmedInstanceGETErrorWraps(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance500(t, gceTestInstanceID)

	g := gceNewLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	err := g.OnCreateConfirmed(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get GCE instance for machine runtime update")
}

// TestGceLifecyclePublishCreateNotification covers PublishCreateNotification
// publishing gcpGceMachineRuntimeInstance.create and wrapping a NATS error.
func TestGceLifecyclePublishCreateNotification(t *testing.T) {
	t.Run("publish success", func(t *testing.T) {
		s := gceNewAPIStub(t)
		r := s.gceReconciler()
		js := &gceFakeJetStream{}
		r.JetStreamContext = js
		log := logr.Discard()
		g := newGceMachineLifecycleProvider(r, gceBaseInstance(gceTestInstanceID, gceTestInstanceName), &log)

		require.NoError(t, g.PublishCreateNotification())
		assert.Equal(t, []string{"gcpGceMachineRuntimeInstance.create"}, js.subjects)
	})

	t.Run("publish error wraps", func(t *testing.T) {
		s := gceNewAPIStub(t)
		r := s.gceReconciler()
		js := &gceFakeJetStream{err: fmt.Errorf("nats down")}
		r.JetStreamContext = js
		log := logr.Discard()
		g := newGceMachineLifecycleProvider(r, gceBaseInstance(gceTestInstanceID, gceTestInstanceName), &log)

		err := g.PublishCreateNotification()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to publish create notification")
	})
}

// TestGceLifecyclePublishDeleteNotification covers PublishDeleteNotification
// publishing gcpGceMachineRuntimeInstance.delete and wrapping a NATS error.
func TestGceLifecyclePublishDeleteNotification(t *testing.T) {
	t.Run("publish success", func(t *testing.T) {
		s := gceNewAPIStub(t)
		r := s.gceReconciler()
		js := &gceFakeJetStream{}
		r.JetStreamContext = js
		log := logr.Discard()
		g := newGceMachineLifecycleProvider(r, gceBaseInstance(gceTestInstanceID, gceTestInstanceName), &log)

		require.NoError(t, g.PublishDeleteNotification())
		assert.Equal(t, []string{"gcpGceMachineRuntimeInstance.delete"}, js.subjects)
	})

	t.Run("publish error wraps", func(t *testing.T) {
		s := gceNewAPIStub(t)
		r := s.gceReconciler()
		js := &gceFakeJetStream{err: fmt.Errorf("nats down")}
		r.JetStreamContext = js
		log := logr.Discard()
		g := newGceMachineLifecycleProvider(r, gceBaseInstance(gceTestInstanceID, gceTestInstanceName), &log)

		err := g.PublishDeleteNotification()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to publish delete notification")
	})
}

// TestGceInstanceCreatedConfirmedNoop covers v0GcpGceMachineRuntimeInstanceCreated
// returning delay 0 when CreationConfirmed is already set.
func TestGceInstanceCreatedConfirmedNoop(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.CreationConfirmed = gcePtr(time.Now().UTC())
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceCreated(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
}

// TestGceInstanceCreatedRequeuesWhenAckedNotStale covers
// v0GcpGceMachineRuntimeInstanceCreated returning delay 120 for a fresh ack.
func TestGceInstanceCreatedRequeuesWhenAckedNotStale(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.CreationConfirmed = nil
	latest.CreationFailed = gcePtr(false)
	latest.CreationAcknowledged = gcePtr(time.Now().UTC())
	latest.ResourceInventory = nil
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceCreated(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(120), delay)
}

// gceNewUpdateLifecycle constructs the update-pass adapter against the stub for
// a given instance.
func gceNewUpdateLifecycle(s *gceAPIStub, instance *v0.GcpGceMachineRuntimeInstance) *gceMachineUpdateLifecycle {
	return &gceMachineUpdateLifecycle{gceMachineLifecycle: gceNewLifecycle(s, instance)}
}

// TestGceUpdateLifecycleGetReconciliationClearsConfirmation asserts the update
// adapter reports the creation as unconfirmed and unacknowledged while leaving
// the resource inventory intact, so the create state machine relaunches the up
// against the saved stack state rather than short-circuiting or provisioning
// fresh.
func TestGceUpdateLifecycleGetReconciliationClearsConfirmation(t *testing.T) {
	s := gceNewAPIStub(t)
	// the latest state shows a fully confirmed prior create with saved inventory;
	// a settled create acknowledges before it confirms, so the ack predates the
	// confirmation and belongs to that prior create rather than this update
	now := time.Now().UTC()
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.CreationConfirmed = gcePtr(now)
	latest.CreationAcknowledged = gcePtr(now.Add(-time.Minute))
	inventory := datatypes.JSON([]byte(`{"checkpoint":{}}`))
	latest.ResourceInventory = &inventory
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	g := gceNewUpdateLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	snap, err := g.GetReconciliation()
	require.NoError(t, err)
	// verify the confirmation fields are cleared so the create handler reopens
	assert.Nil(t, snap.CreationConfirmed, "confirmation must be cleared to reopen the create state machine")
	assert.Nil(t, snap.CreationAcknowledged, "the prior create ack must be cleared so the first update pass relaunches the up")
	// verify the saved inventory survives so the up diffs rather than provisions fresh
	require.NotNil(t, snap.ResourceInventory)
	assert.JSONEq(t, `{"checkpoint":{}}`, string(*snap.ResourceInventory))
}

// TestGceUpdateLifecycleSaveCreateOutputsMarksReconciled asserts the update
// adapter persists the surfaced outputs and then marks the instance reconciled,
// since the update pass relaunches the up but never reaches the confirm step a
// first create uses to settle the object.
func TestGceUpdateLifecycleSaveCreateOutputsMarksReconciled(t *testing.T) {
	s := gceNewAPIStub(t)
	s.gceHandleInstance(t, gceTestInstanceID, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))

	g := gceNewUpdateLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	gceInfra := machine.NewGceMachineInfra(gceTestInstanceName)
	gceInfra.SetCreateOutputs("vm.example", "203.0.113.7", "PRIVATE-KEY")
	state := datatypes.JSON([]byte(`{"checkpoint":{}}`))

	require.NoError(t, g.SaveCreateOutputs(gceInfra, &state))

	// the last PATCH must carry Reconciled=true so the update settles the object
	patch := s.gceLastPatch(t, gceInstancePath(gceTestInstanceID))
	require.NotNil(t, patch.Reconciled)
	assert.True(t, *patch.Reconciled, "the update pass must mark the instance reconciled after the up succeeds")
}

// TestGceUpdateLifecycleGetReconciliationReconciledNoop asserts the update
// adapter leaves the confirmation intact once the instance is reconciled, so a
// requeued update pass short-circuits instead of relaunching the up
// indefinitely.
func TestGceUpdateLifecycleGetReconciliationReconciledNoop(t *testing.T) {
	s := gceNewAPIStub(t)
	// the latest state shows a confirmed, reconciled instance from a settled
	// prior update
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.CreationConfirmed = gcePtr(time.Now().UTC())
	latest.CreationAcknowledged = gcePtr(time.Now().UTC())
	latest.Reconciled = gcePtr(true)
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	g := gceNewUpdateLifecycle(s, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	snap, err := g.GetReconciliation()
	require.NoError(t, err)
	// verify the confirmation survives so the create handler short-circuits
	assert.NotNil(t, snap.CreationConfirmed, "a reconciled instance must keep its confirmation so the update no-ops")
}

// TestGceUpdateLifecycleSecondPassPreservesFreshAck asserts the update adapter
// preserves the acknowledgement once an update pass has acked and launched the
// up. The first pass clears the prior create's ack so the create handler
// relaunches; the second pass, finding a fresh ack that post-dates the
// confirmation, leaves it intact so the create handler's not-stale short-circuit
// requeues rather than acknowledging and launching a second concurrent up.
func TestGceUpdateLifecycleSecondPassPreservesFreshAck(t *testing.T) {
	confirmed := time.Now().UTC()

	// first pass: the ack belongs to the settled prior create, so it predates
	// the confirmation and the adapter clears it to relaunch the up
	s1 := gceNewAPIStub(t)
	firstPass := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	firstPass.CreationConfirmed = gcePtr(confirmed)
	firstPass.CreationAcknowledged = gcePtr(confirmed.Add(-time.Minute))
	inventory := datatypes.JSON([]byte(`{"checkpoint":{}}`))
	firstPass.ResourceInventory = &inventory
	s1.gceHandleInstance(t, gceTestInstanceID, firstPass)

	g1 := gceNewUpdateLifecycle(s1, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	snap1, err := g1.GetReconciliation()
	require.NoError(t, err)
	// the prior create ack is cleared so the create handler falls through to ack
	// and launch the up on the first pass
	assert.Nil(t, snap1.CreationAcknowledged, "first pass must clear the prior create ack so the up launches")

	// second pass: the up launched by the first pass has acknowledged, so the
	// ack now post-dates the confirmation and is fresh against the stale
	// threshold; the adapter must leave it intact so the create handler requeues
	s2 := gceNewAPIStub(t)
	secondPass := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	secondPass.CreationConfirmed = gcePtr(confirmed)
	secondPass.CreationAcknowledged = gcePtr(confirmed.Add(time.Second))
	secondPass.ResourceInventory = &inventory
	s2.gceHandleInstance(t, gceTestInstanceID, secondPass)

	g2 := gceNewUpdateLifecycle(s2, gceBaseInstance(gceTestInstanceID, gceTestInstanceName))
	snap2, err := g2.GetReconciliation()
	require.NoError(t, err)
	// the fresh update ack survives so the create handler's not-stale check fires
	require.NotNil(t, snap2.CreationAcknowledged, "second pass must preserve the fresh update ack")
	assert.True(t, snap2.CreationAcknowledged.Equal(confirmed.Add(time.Second)), "the preserved ack must be the fresh update ack")

	// drive the full Updated handler on the second pass: with the fresh ack
	// preserved and the inventory incomplete, the create handler requeues at 120
	// seconds without acknowledging again or launching a second up
	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceUpdated(
		s2.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(120), delay, "the second pass must requeue rather than launch a second up")
	// no PATCH on the second pass carried an acknowledgement, proving the create
	// handler did not re-ack and therefore did not launch a second concurrent up
	assert.Equal(t, 0, s2.gceCountAckPatches(t, gceInstancePath(gceTestInstanceID)), "the second pass must not acknowledge again")
}

// TestGceInstanceUpdatedReconciledNoop drives the Updated reconciler against a
// reconciled instance and asserts it returns cleanly without launching an up,
// proving a requeue after a settled update does not loop.
func TestGceInstanceUpdatedReconciledNoop(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.CreationConfirmed = gcePtr(time.Now().UTC())
	latest.CreationAcknowledged = gcePtr(time.Now().UTC())
	latest.Reconciled = gcePtr(true)
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceUpdated(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay, "a reconciled instance must no-op on update rather than relaunch the up")
}

// TestGceInstanceUpdatedAbortsWhenDeletionScheduled drives the Updated
// reconciler through the create state machine to the deletion-abort branch:
// with the creation confirmation cleared by the update adapter and a deletion
// already scheduled, the handler returns cleanly without launching an up.
func TestGceInstanceUpdatedAbortsWhenDeletionScheduled(t *testing.T) {
	s := gceNewAPIStub(t)
	// the latest state has a deletion scheduled, so the create handler aborts
	// after acknowledging rather than launching the up; the ack predates the
	// confirmation so this models the first update pass, which clears the prior
	// create ack and falls through to acknowledge before the abort check
	now := time.Now().UTC()
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.CreationConfirmed = gcePtr(now)
	latest.CreationAcknowledged = gcePtr(now.Add(-time.Minute))
	latest.DeletionScheduled = gcePtr(now)
	s.gceHandleInstance(t, gceTestInstanceID, latest)
	// BuildInfra reads the provider and definition before the deletion re-check
	prov := gceBaseProvider()
	enc, encErr := encryption.Encrypt(s.encryptionKey, "creds-json")
	require.NoError(t, encErr)
	prov.ServiceAccountCredentials = gcePtr(enc)
	s.gceHandleProvider(t, gceTestProviderID, prov)
	s.gceHandleDefinition(t, gceTestDefinitionID, gceBaseDefinition())

	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceUpdated(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay, "a scheduled deletion aborts the update up")
}

// TestGceInstanceDeletedConfirmedNoop covers v0GcpGceMachineRuntimeInstanceDeleted
// returning delay 0 when DeletionConfirmed is already set.
func TestGceInstanceDeletedConfirmedNoop(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.DeletionScheduled = gcePtr(time.Now().UTC())
	latest.DeletionConfirmed = gcePtr(time.Now().UTC())
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceDeleted(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
}

// TestGceInstanceDeletedRequeuesWhenCreateInProgress covers
// v0GcpGceMachineRuntimeInstanceDeleted returning delay 60 while create is acked.
func TestGceInstanceDeletedRequeuesWhenCreateInProgress(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.DeletionScheduled = gcePtr(time.Now().UTC())
	latest.DeletionConfirmed = nil
	latest.CreationConfirmed = nil
	latest.CreationAcknowledged = gcePtr(time.Now().UTC())
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	log := logr.Discard()
	delay, err := v0GcpGceMachineRuntimeInstanceDeleted(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(60), delay)
}

// TestGceInstanceDeletedNotScheduled covers v0GcpGceMachineRuntimeInstanceDeleted
// erroring when DeletionScheduled is nil.
func TestGceInstanceDeletedNotScheduled(t *testing.T) {
	s := gceNewAPIStub(t)
	latest := gceBaseInstance(gceTestInstanceID, gceTestInstanceName)
	latest.DeletionScheduled = nil
	s.gceHandleInstance(t, gceTestInstanceID, latest)

	log := logr.Discard()
	_, err := v0GcpGceMachineRuntimeInstanceDeleted(
		s.gceReconciler(),
		gceBaseInstance(gceTestInstanceID, gceTestInstanceName),
		&log,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion notification received but not scheduled")
}

// TestGceLifecycleConcurrentInstancesNoRace covers concurrent adapters for
// distinct instance IDs with no cross-instance PATCH bleed.
func TestGceLifecycleConcurrentInstancesNoRace(t *testing.T) {
	const n = 200
	s := gceNewAPIStub(t)
	// register a GET/PATCH handler per instance ID
	for i := 0; i < n; i++ {
		id := uint(1000 + i)
		s.gceHandleInstance(t, id, gceBaseInstance(id, fmt.Sprintf("gce-%d", id)))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, n)
	// run GetReconciliation, AckCreation, SaveState, ConfirmCreation per ID
	for i := 0; i < n; i++ {
		id := uint(1000 + i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			g := gceNewLifecycle(s, gceBaseInstance(id, fmt.Sprintf("gce-%d", id)))
			if _, err := g.GetReconciliation(); err != nil {
				errCh <- err
				return
			}
			if err := g.AckCreation(); err != nil {
				errCh <- err
				return
			}
			st := datatypes.JSON([]byte(`{"deployment":{}}`))
			if err := g.SaveState(&st); err != nil {
				errCh <- err
				return
			}
			if err := g.ConfirmCreation(); err != nil {
				errCh <- err
				return
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// check each path recorded AckCreation, SaveState, and ConfirmCreation
	for i := 0; i < n; i++ {
		id := uint(1000 + i)
		bodies := s.patches[gceInstancePath(id)]
		assert.Len(t, bodies, 3, "instance %d should have exactly its own 3 PATCHes", id)
	}
}
