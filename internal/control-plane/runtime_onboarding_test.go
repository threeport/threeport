package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeAPI is a threeport API held in memory: enough of the REST surface for the
// client library to create objects, list them by a query filter and fetch them
// by ID. Objects are kept as the JSON the client sent, so a test sees exactly
// what went over the wire.
type fakeAPI struct {
	t      *testing.T
	server *httptest.Server

	mu      sync.Mutex
	nextID  int
	objects map[string][]map[string]any
	posted  map[string][]map[string]any
}

// newFakeAPI starts a fake API that numbers the objects created on it from
// firstID, so a test can tell the IDs of one API from another's.
func newFakeAPI(t *testing.T, firstID int) *fakeAPI {
	api := &fakeAPI{
		t:       t,
		nextID:  firstID,
		objects: map[string][]map[string]any{},
		posted:  map[string][]map[string]any{},
	}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)

	return api
}

// target is the API as the onboarder addresses it.
func (a *fakeAPI) target() apiTarget {
	return apiTarget{
		client:   a.server.Client(),
		endpoint: strings.TrimPrefix(a.server.URL, "http://"),
	}
}

// seed adds an object that exists on the API before the test starts.
func (a *fakeAPI) seed(path string, object map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objects[path] = append(a.objects[path], object)
}

// createdOn returns the objects the test's code created at a path.
func (a *fakeAPI) createdOn(path string) []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.posted[path]
}

func (a *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	respond := func(status int, data ...map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		require.NoError(a.t, json.NewEncoder(w).Encode(map[string]any{
			"Meta":   map[string]any{"ObjectCount": len(data)},
			"Data":   data,
			"Status": map[string]any{"code": status},
		}))
	}

	switch r.Method {
	case http.MethodPost:
		var object map[string]any
		require.NoError(a.t, json.NewDecoder(r.Body).Decode(&object))
		a.posted[r.URL.Path] = append(a.posted[r.URL.Path], cloneObject(object))
		object["ID"] = a.nextID
		a.nextID++
		a.objects[r.URL.Path] = append(a.objects[r.URL.Path], object)
		respond(http.StatusCreated, object)

	case http.MethodGet:
		// a numeric last segment is a fetch by ID, anything else a list
		path := r.URL.Path
		if i := strings.LastIndex(path, "/"); i >= 0 {
			if id, err := strconv.Atoi(path[i+1:]); err == nil {
				for _, object := range a.objects[path[:i]] {
					if number(object["ID"]) == id {
						respond(http.StatusOK, object)
						return
					}
				}
				respond(http.StatusNotFound)
				return
			}
		}

		var matches []map[string]any
		for _, object := range a.objects[path] {
			if matchesQuery(object, r) {
				matches = append(matches, object)
			}
		}
		respond(http.StatusOK, matches...)

	default:
		respond(http.StatusMethodNotAllowed)
	}
}

// matchesQuery reports whether an object satisfies every filter in a request's
// query string, comparing field names without regard to case as the real API
// does.
func matchesQuery(object map[string]any, r *http.Request) bool {
	for key, values := range r.URL.Query() {
		var found bool
		for field, value := range object {
			if strings.EqualFold(field, key) && value != nil && toString(value) == values[0] {
				found = true
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func toString(value any) string {
	if f, ok := value.(float64); ok {
		return strconv.Itoa(int(f))
	}
	if i, ok := value.(int); ok {
		return strconv.Itoa(i)
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func number(value any) int {
	n, _ := strconv.Atoi(toString(value))
	return n
}

func cloneObject(object map[string]any) map[string]any {
	raw, _ := json.Marshal(object)
	var clone map[string]any
	_ = json.Unmarshal(raw, &clone)
	return clone
}

// parentGkeRuntime seeds a parent API with a GKE runtime: the GCP provider, and
// the provider's definition and instance for kubernetes runtime definition 10
// and instance 20.
func parentGkeRuntime(t *testing.T, parent *fakeAPI, encryptionKey string, withCredentials bool) {
	provider := map[string]any{
		"ID":            1,
		"Name":          "gcp-account",
		"ProjectID":     "project-a",
		"DefaultRegion": "us-central1",
	}
	if withCredentials {
		encrypted, err := encryption.Encrypt(encryptionKey, `{"type":"service_account"}`)
		require.NoError(t, err)
		provider["ServiceAccountCredentials"] = encrypted
	}
	parent.seed(v0.PathGcpProviders, provider)
	parent.seed(v0.PathGcpGkeKubernetesRuntimeDefinitions, map[string]any{
		"ID":                            5,
		"Name":                          "gke-def",
		"ZoneCount":                     1,
		"KubernetesRuntimeDefinitionID": 10,
	})
	parent.seed(v0.PathGcpGkeKubernetesRuntimeInstances, map[string]any{
		"ID":                                  7,
		"Name":                                "gke-inst",
		"GcpProviderID":                       1,
		"GcpGkeKubernetesRuntimeDefinitionID": 5,
		"KubernetesRuntimeInstanceID":         20,
		"Reconciled":                          false,
	})
}

func TestCopyGkeRecords_CreatesRecordsOnChild(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)
	parentGkeRuntime(t, parent, key, true)

	o := &runtimeOnboarder{parent: parent.target(), child: child.target(), encryptionKey: key}
	err = o.copyGkeRecords(runtimeLink{definitionID: 10, instanceID: 20}, runtimeLink{definitionID: 100, instanceID: 200}, false)
	require.NoError(t, err)

	providers := child.createdOn(v0.PathGcpProviders)
	require.Len(t, providers, 1)
	assert.Equal(t, "gcp-account", providers[0]["Name"])
	assert.Equal(t, "project-a", providers[0]["ProjectID"])
	assert.Equal(t, `{"type":"service_account"}`, providers[0]["ServiceAccountCredentials"],
		"credentials go to the child as plaintext - its API encrypts them with its own key")
	assert.Nil(t, providers[0]["ID"], "the parent's ID must not be carried over")

	definitions := child.createdOn(v0.PathGcpGkeKubernetesRuntimeDefinitions)
	require.Len(t, definitions, 1)
	assert.EqualValues(t, 100, definitions[0]["KubernetesRuntimeDefinitionID"],
		"the definition belongs to the child's copy of the generic definition, not the parent's")

	instances := child.createdOn(v0.PathGcpGkeKubernetesRuntimeInstances)
	require.Len(t, instances, 1)
	assert.Equal(t, true, instances[0]["Reconciled"],
		"the cluster exists already, so the child's gcp-controller must not provision one")
	assert.EqualValues(t, 200, instances[0]["KubernetesRuntimeInstanceID"])
	assert.EqualValues(t, 500, instances[0]["GcpProviderID"], "points at the provider created on the child")
	assert.EqualValues(t, 501, instances[0]["GcpGkeKubernetesRuntimeDefinitionID"], "points at the definition created on the child")
}

func TestCopyGkeRecords_NoCredentialsStaysUnset(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)
	parentGkeRuntime(t, parent, key, false)

	o := &runtimeOnboarder{parent: parent.target(), child: child.target(), encryptionKey: key}
	require.NoError(t, o.copyGkeRecords(
		runtimeLink{definitionID: 10, instanceID: 20}, runtimeLink{definitionID: 100, instanceID: 200}, false))

	providers := child.createdOn(v0.PathGcpProviders)
	require.Len(t, providers, 1)
	assert.Nil(t, providers[0]["ServiceAccountCredentials"],
		"a provider that relies on ambient Workload Identity has no key to hold")
}

func TestCopyGkeRecords_ReusesProviderAndDefinition(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)
	parentGkeRuntime(t, parent, key, true)

	// the child already has this provider and the definition from copying
	// another runtime that shares them
	child.seed(v0.PathGcpProviders, map[string]any{"ID": 77, "Name": "gcp-account", "ProjectID": "project-a"})
	child.seed(v0.PathGcpGkeKubernetesRuntimeDefinitions, map[string]any{"ID": 88, "KubernetesRuntimeDefinitionID": 100})

	o := &runtimeOnboarder{parent: parent.target(), child: child.target(), encryptionKey: key}
	require.NoError(t, o.copyGkeRecords(
		runtimeLink{definitionID: 10, instanceID: 20}, runtimeLink{definitionID: 100, instanceID: 300}, true))

	assert.Empty(t, child.createdOn(v0.PathGcpProviders), "the provider's name is unique, so it must be found rather than created again")
	assert.Empty(t, child.createdOn(v0.PathGcpGkeKubernetesRuntimeDefinitions))
	instances := child.createdOn(v0.PathGcpGkeKubernetesRuntimeInstances)
	require.Len(t, instances, 1)
	assert.EqualValues(t, 77, instances[0]["GcpProviderID"])
	assert.EqualValues(t, 88, instances[0]["GcpGkeKubernetesRuntimeDefinitionID"])
	assert.EqualValues(t, 300, instances[0]["KubernetesRuntimeInstanceID"])
}

func TestCopyGkeRecords_FailsWithoutParentRecords(t *testing.T) {
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)

	o := &runtimeOnboarder{parent: parent.target(), child: child.target(), encryptionKey: "unused"}
	err := o.copyGkeRecords(runtimeLink{definitionID: 10, instanceID: 20}, runtimeLink{definitionID: 100, instanceID: 200}, false)

	require.Error(t, err, "a GKE runtime with no GKE records cannot be onboarded, and must not look onboarded")
	assert.Empty(t, child.createdOn(v0.PathGcpGkeKubernetesRuntimeInstances))
}

func TestCreateRuntimeInstance_SetsFlagsAndDecryptsSecrets(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	encryptedKey, err := encryption.Encrypt(key, "private-key")
	require.NoError(t, err)
	encryptedToken, err := encryption.Encrypt(key, "bearer-token")
	require.NoError(t, err)

	// a runtime that, on the parent, is the host and default - the genesis one
	source := &v0.KubernetesRuntimeInstance{
		Common:                    v0.Common{ID: util.Ptr(uint(3))},
		Instance:                  v0.Instance{Name: util.Ptr("genesis-runtime")},
		CertificateKey:            &encryptedKey,
		ConnectionToken:           &encryptedToken,
		ThreeportControlPlaneHost: util.Ptr(true),
		DefaultRuntime:            util.Ptr(true),
	}

	tests := []struct {
		name           string
		host           bool
		defaultRuntime bool
	}{
		{"runtime the child is installed on", true, true},
		{"parent's runtime registered with the child", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := newFakeAPI(t, 500)
			o := &runtimeOnboarder{child: child.target(), encryptionKey: key}

			created, err := o.createRuntimeInstance(source, util.Ptr(uint(100)), tt.host, tt.defaultRuntime)
			require.NoError(t, err)
			assert.EqualValues(t, 500, *created.ID)

			posted := child.createdOn(v0.PathKubernetesRuntimeInstances)
			require.Len(t, posted, 1)
			assert.Equal(t, tt.host, posted[0]["ThreeportControlPlaneHost"])
			assert.Equal(t, tt.defaultRuntime, posted[0]["DefaultRuntime"])
			assert.EqualValues(t, 100, posted[0]["KubernetesRuntimeDefinitionID"])
			assert.Equal(t, "private-key", posted[0]["CertificateKey"])
			assert.Equal(t, "bearer-token", posted[0]["ConnectionToken"])
			assert.Nil(t, posted[0]["ID"])
		})
	}

	// copying for the child must leave the parent's own record as it was
	assert.True(t, *source.ThreeportControlPlaneHost)
	assert.True(t, *source.DefaultRuntime)
	assert.Equal(t, encryptedKey, *source.CertificateKey)
	assert.Equal(t, encryptedToken, *source.ConnectionToken)
}

func TestCreateRuntimeInstance_RejectsUndecryptableSecret(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	otherKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	encrypted, err := encryption.Encrypt(otherKey, "private-key")
	require.NoError(t, err)

	child := newFakeAPI(t, 500)
	o := &runtimeOnboarder{child: child.target(), encryptionKey: key}

	_, err = o.createRuntimeInstance(
		&v0.KubernetesRuntimeInstance{CertificateKey: &encrypted}, util.Ptr(uint(100)), true, true)

	require.Error(t, err)
	assert.Empty(t, child.createdOn(v0.PathKubernetesRuntimeInstances), "ciphertext must never be passed along as if it were plaintext")
}

func TestGkeRuntimeProjectID(t *testing.T) {
	t.Run("reads the project of the runtime's own provider", func(t *testing.T) {
		parent := newFakeAPI(t, 1)
		parentGkeRuntime(t, parent, "unused", false)

		target := parent.target()
		project, err := gkeRuntimeProjectID(target.client, target.endpoint, 20)

		require.NoError(t, err)
		assert.Equal(t, "project-a", project)
	})

	t.Run("fails when the provider has no project", func(t *testing.T) {
		parent := newFakeAPI(t, 1)
		parent.seed(v0.PathGcpProviders, map[string]any{"ID": 1, "Name": "gcp-account"})
		parent.seed(v0.PathGcpGkeKubernetesRuntimeInstances, map[string]any{
			"ID": 7, "GcpProviderID": 1, "KubernetesRuntimeInstanceID": 20,
		})

		target := parent.target()
		_, err := gkeRuntimeProjectID(target.client, target.endpoint, 20)

		require.Error(t, err, "installing without the project would leave the child's controllers unauthorized, silently")
	})

	t.Run("fails when the runtime has no GKE records", func(t *testing.T) {
		parent := newFakeAPI(t, 1)

		target := parent.target()
		_, err := gkeRuntimeProjectID(target.client, target.endpoint, 20)

		require.Error(t, err)
	})
}
