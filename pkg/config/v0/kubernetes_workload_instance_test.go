package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// respondWithObject writes obj as a single-item apiserver_lib.Response.
func respondWithObject(t *testing.T, w http.ResponseWriter, obj apiserver_lib.Object) {
	t.Helper()
	body, err := json.Marshal(apiserver_lib.Response{
		Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
		Data:   []apiserver_lib.Object{obj},
	})
	require.NoError(t, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// TestGetKubernetesRuntimeInstanceAndCheckId is a regression test for a bug
// where the "moved" result was inverted: the function returned true (the
// value every caller treats as "reject this replace, the object was moved")
// for the two cases that are NOT a move - no runtime name given (falls back
// to the existing ID) and a given name that resolves back to the existing
// ID - and false, with a nil *api_v0.KubernetesRuntimeInstance, for the one
// case that IS a move. Every one of the 5 Replace() call sites in this
// package (kubernetes_workload_instance.go, gateway_instance.go,
// helm_workload_instance.go, observability_stack_instance.go,
// secret_instance.go) does `if moved { return error }` and then dereferences
// the returned instance's Name to build that error message, so the old
// behavior meant: every ordinary replace call was rejected as "may not be
// moved", and an actual move would nil-pointer-panic instead of returning a
// clean error.
func TestGetKubernetesRuntimeInstanceAndCheckId(t *testing.T) {
	existingRuntimeID := uint(10)
	otherRuntimeID := uint(20)
	existingRuntimeName := "existing-runtime"
	otherRuntimeName := "other-runtime"

	t.Run("no name provided falls back to the existing ID and is not a move", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, fmt.Sprintf("%s/%d", api_v0.PathKubernetesRuntimeInstances, existingRuntimeID), r.URL.Path)
			respondWithObject(t, w, api_v0.KubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &existingRuntimeID},
				Instance: api_v0.Instance{Name: &existingRuntimeName},
			})
		}))
		defer srv.Close()

		kri, moved, err := getKubernetesRuntimeInstanceAndCheckId(
			&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), nil, &existingRuntimeID,
		)
		require.NoError(t, err)
		assert.False(t, moved, "no name provided should never be reported as a move")
		require.NotNil(t, kri)
		assert.Equal(t, existingRuntimeID, *kri.ID)
	})

	t.Run("name provided that matches the existing ID is not a move", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, existingRuntimeName, r.URL.Query().Get("name"))
			respondWithObject(t, w, api_v0.KubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &existingRuntimeID},
				Instance: api_v0.Instance{Name: &existingRuntimeName},
			})
		}))
		defer srv.Close()

		kri, moved, err := getKubernetesRuntimeInstanceAndCheckId(
			&http.Client{}, strings.TrimPrefix(srv.URL, "http://"),
			&KubernetesRuntimeInstanceValues{Name: &existingRuntimeName}, &existingRuntimeID,
		)
		require.NoError(t, err)
		assert.False(t, moved, "a name that resolves back to the existing ID should not be reported as a move")
		require.NotNil(t, kri)
		assert.Equal(t, existingRuntimeID, *kri.ID)
	})

	t.Run("name provided that resolves to a different ID is a move", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, otherRuntimeName, r.URL.Query().Get("name"))
			respondWithObject(t, w, api_v0.KubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &otherRuntimeID},
				Instance: api_v0.Instance{Name: &otherRuntimeName},
			})
		}))
		defer srv.Close()

		kri, moved, err := getKubernetesRuntimeInstanceAndCheckId(
			&http.Client{}, strings.TrimPrefix(srv.URL, "http://"),
			&KubernetesRuntimeInstanceValues{Name: &otherRuntimeName}, &existingRuntimeID,
		)
		require.NoError(t, err)
		assert.True(t, moved, "a name that resolves to a different ID must be reported as a move")
		// callers dereference kri.Name to build the "may not be moved" error
		// message - a nil kri here is exactly what caused the panic.
		require.NotNil(t, kri, "kri must not be nil when moved is true, or callers building the error message will panic")
		assert.Equal(t, otherRuntimeName, *kri.Name)
	})
}

// TestKubernetesWorkloadInstanceConfig_Replace_PreservesKustomizeOverlay is a
// regression test for a bug where Replace() never set KustomizeOverlay on
// the outgoing PUT payload. Since Replace sends a full replacement and
// KustomizeOverlay is immutable once set (kubernetes_workload_validate.go),
// an omitted value was sent as an explicit clear and rejected by the API -
// so replacing an overlaid instance to change an unrelated field (e.g.
// renaming it) was impossible. This also exercises the
// getKubernetesRuntimeInstanceAndCheckId fix above end to end: Replace()
// would have failed with "may not be moved" before ever reaching the PUT
// if that bug were still present.
func TestKubernetesWorkloadInstanceConfig_Replace_PreservesKustomizeOverlay(t *testing.T) {
	existingID := uint(1)
	runtimeInstanceID := uint(2)
	definitionID := uint(3)
	name := "test-instance"
	newName := "renamed-instance"
	definitionName := "test-definition"
	overlay := "patches:\n  - target:\n      kind: Deployment\n      name: myapp\n"

	createdAt := time.Now()
	existing := api_v0.KubernetesWorkloadInstance{
		Common:                         api_v0.Common{ID: &existingID, CreatedAt: &createdAt},
		Instance:                       api_v0.Instance{Name: &name},
		KubernetesRuntimeInstanceID:    &runtimeInstanceID,
		KubernetesWorkloadDefinitionID: &definitionID,
		KustomizeOverlay:               &overlay,
		Status:                         util.Ptr("Up"),
	}

	handlerErrCh := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == api_v0.PathKubernetesWorkloadInstances:
			// Replace() looks up the existing instance by name first.
			require.Equal(t, name, r.URL.Query().Get("name"))
			respondWithObject(t, w, existing)

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("%s/%s", api_v0.PathKubernetesRuntimeInstances, strconv.Itoa(int(runtimeInstanceID))):
			// no KubernetesRuntimeInstance name given in the config, so
			// getKubernetesRuntimeInstanceAndCheckId falls back to fetching
			// by the existing FK.
			respondWithObject(t, w, api_v0.KubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &runtimeInstanceID},
				Instance: api_v0.Instance{Name: util.Ptr("some-runtime")},
			})

		case r.Method == http.MethodGet && r.URL.Path == api_v0.PathKubernetesWorkloadDefinitions:
			require.Equal(t, definitionName, r.URL.Query().Get("name"))
			respondWithObject(t, w, api_v0.KubernetesWorkloadDefinition{
				Common: api_v0.Common{ID: &definitionID},
			})

		case r.Method == http.MethodPut:
			var payload api_v0.KubernetesWorkloadInstance
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				handlerErrCh <- fmt.Errorf("failed to decode PUT request body: %w", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			assert.Equal(t, &overlay, payload.KustomizeOverlay, "KustomizeOverlay must be preserved in the replace payload")
			assert.Equal(t, &newName, payload.Name, "Name must reflect the requested rename")

			// the client strips ID/CreatedAt/UpdatedAt from the outgoing PUT
			// payload and doesn't set Status at all; a real API response
			// would still include all of these.
			payload.ID = &existingID
			payload.CreatedAt = &createdAt
			payload.Status = util.Ptr("Up")
			respondWithObject(t, w, payload)

		default:
			err := fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			handlerErrCh <- err
			http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	config := &KubernetesWorkloadInstanceConfig{
		KubernetesWorkloadInstance: KubernetesWorkloadInstanceValues{
			Name: &newName,
			KubernetesWorkloadDefinition: &KubernetesWorkloadDefinitionValues{
				Name: &definitionName,
			},
		},
	}

	_, err := config.Replace(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), name)

	select {
	case handlerErr := <-handlerErrCh:
		t.Fatalf("mock server handler error: %v", handlerErr)
	default:
	}

	require.NoError(t, err, "Replace should succeed and preserve the existing KustomizeOverlay")
}
