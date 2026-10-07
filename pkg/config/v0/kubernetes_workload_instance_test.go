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

// respondWithObject writes obj as a single-item apiserver_lib.Response with a
// 200 status.
func respondWithObject(t *testing.T, w http.ResponseWriter, obj apiserver_lib.Object) {
	t.Helper()
	respondWithObjectStatus(t, w, obj, http.StatusOK)
}

// respondWithObjectStatus writes obj as a single-item apiserver_lib.Response
// with the given status code - needed for Create, which the client only
// accepts as a 201.
func respondWithObjectStatus(t *testing.T, w http.ResponseWriter, obj apiserver_lib.Object, statusCode int) {
	t.Helper()
	body, err := json.Marshal(apiserver_lib.Response{
		Status: apiserver_lib.Status{Code: statusCode, Message: http.StatusText(statusCode)},
		Data:   []apiserver_lib.Object{obj},
	})
	require.NoError(t, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
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

// TestKubernetesWorkloadInstanceConfig_Replace_PreservesInstanceState is a
// regression test covering two related bugs in Replace(), both stemming
// from the same root cause: Replace sends a PUT (full replace), so any
// field left unset on the outgoing object is written as nil, clearing
// whatever the server previously had.
//
//   - KustomizeOverlay: an omitted value was sent as an explicit clear and
//     rejected by the API, since KustomizeOverlay is immutable once set
//     (kubernetes_workload_validate.go) - so replacing an overlaid instance
//     to change an unrelated field (e.g. renaming it) was impossible.
//   - Status and Reconciled: these are controller-owned state, not
//     something Replace's caller is trying to set. An omitted value
//     silently cleared an instance's reconciliation progress on every
//     replace, and since Replace dereferences the response's Status
//     immediately afterward, an omitted Status also made every replace
//     panic once the server actually cleared it.
//
// This also exercises the getKubernetesRuntimeInstanceAndCheckId fix above
// end to end: Replace() would have failed with "may not be moved" before
// ever reaching the PUT if that bug were still present.
func TestKubernetesWorkloadInstanceConfig_Replace_PreservesInstanceState(t *testing.T) {
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
		Reconciliation:                 api_v0.Reconciliation{Reconciled: util.Ptr(true)},
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
			// Status and Reconciled are controller-owned state, not
			// something Replace's caller is trying to set - PUT is a full
			// replace, so omitting them here would clear them server-side,
			// and Status is dereferenced by Replace after this call
			// returns, so an omitted (nil) Status also used to panic.
			assert.Equal(t, existing.Status, payload.Status, "Status must be preserved in the replace payload, not cleared")
			assert.Equal(t, existing.Reconciled, payload.Reconciled, "Reconciled must be preserved in the replace payload, not cleared")

			// echo back exactly what was received, plus only the fields the
			// client legitimately strips from the outgoing payload itself
			// (ID/CreatedAt/UpdatedAt) - a real API response would still
			// include these. Deliberately NOT force-setting Status/Reconciled
			// here: doing so would let Replace's own bug (never setting them
			// on the request) go uncaught, since the response would show the
			// right value regardless of what was actually sent.
			payload.ID = &existingID
			payload.CreatedAt = &createdAt
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

// TestKubernetesWorkloadInstanceConfig_Create_SetsKustomizeOverlay is a
// regression test for a review finding on PR #564: KustomizeOverlay was
// reachable only via a direct client_v0.CreateKubernetesWorkloadInstance
// call, not from KubernetesWorkloadInstanceValues - a config file following
// the documented tptctl apply -f pattern (e.g.
// samples/kubernetes-workload/wordpress-kubernetes-workload-instance-local.yaml)
// had no way to set it; the field was silently dropped with no error.
func TestKubernetesWorkloadInstanceConfig_Create_SetsKustomizeOverlay(t *testing.T) {
	definitionID := uint(1)
	runtimeInstanceID := uint(2)
	createdID := uint(3)
	name := "test-instance"
	definitionName := "test-definition"
	runtimeName := "default-runtime"
	overlay := "patches:\n  - target:\n      kind: Deployment\n      name: myapp\n"
	yamlDoc := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n"

	createdAt := time.Now()
	handlerErrCh := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == api_v0.PathKubernetesRuntimeInstances && r.URL.Query().Get("defaultruntime") == "true":
			// no KubernetesRuntimeInstance given in the config, so Create
			// falls back to the control plane's default runtime.
			respondWithObject(t, w, api_v0.KubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &runtimeInstanceID},
				Instance: api_v0.Instance{Name: &runtimeName},
			})

		case r.Method == http.MethodGet && r.URL.Path == api_v0.PathKubernetesWorkloadDefinitions:
			require.Equal(t, definitionName, r.URL.Query().Get("name"))
			respondWithObject(t, w, api_v0.KubernetesWorkloadDefinition{
				Common:       api_v0.Common{ID: &definitionID},
				YAMLDocument: &yamlDoc,
			})

		case r.Method == http.MethodPost && r.URL.Path == api_v0.PathKubernetesWorkloadInstances:
			var payload api_v0.KubernetesWorkloadInstance
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				handlerErrCh <- fmt.Errorf("failed to decode POST request body: %w", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			assert.Equal(t, &overlay, payload.KustomizeOverlay, "KustomizeOverlay must be set on the create payload")

			payload.ID = &createdID
			payload.CreatedAt = &createdAt
			payload.Status = util.Ptr("Pending")
			respondWithObjectStatus(t, w, payload, http.StatusCreated)

		default:
			err := fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			handlerErrCh <- err
			http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	config := &KubernetesWorkloadInstanceConfig{
		KubernetesWorkloadInstance: KubernetesWorkloadInstanceValues{
			Name: &name,
			KubernetesWorkloadDefinition: &KubernetesWorkloadDefinitionValues{
				Name: &definitionName,
			},
			KustomizeOverlay: &overlay,
		},
	}

	created, err := config.Create(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"))

	select {
	case handlerErr := <-handlerErrCh:
		t.Fatalf("mock server handler error: %v", handlerErr)
	default:
	}

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, &overlay, created.KubernetesWorkloadInstance.KustomizeOverlay, "KustomizeOverlay must be returned in the created config")
}

// TestKubernetesWorkloadInstanceConfig_Replace_PassesThroughExplicitOverlayChange
// covers the other half of the Replace() KustomizeOverlay logic: when a
// config does explicitly set a different value (as opposed to leaving the
// field unset, covered by TestKubernetesWorkloadInstanceConfig_Replace_PreservesInstanceState),
// that new value is sent through as-is rather than silently overridden back
// to the existing one - the API's own immutability check is what rejects it,
// not client-side logic duplicating that rule.
func TestKubernetesWorkloadInstanceConfig_Replace_PassesThroughExplicitOverlayChange(t *testing.T) {
	existingID := uint(1)
	runtimeInstanceID := uint(2)
	definitionID := uint(3)
	name := "test-instance"
	definitionName := "test-definition"
	existingOverlay := "patches:\n  - target:\n      kind: Deployment\n      name: myapp\n"
	newOverlay := "patches:\n  - target:\n      kind: Deployment\n      name: other\n"

	createdAt := time.Now()
	existing := api_v0.KubernetesWorkloadInstance{
		Common:                         api_v0.Common{ID: &existingID, CreatedAt: &createdAt},
		Instance:                       api_v0.Instance{Name: &name},
		Reconciliation:                 api_v0.Reconciliation{Reconciled: util.Ptr(true)},
		KubernetesRuntimeInstanceID:    &runtimeInstanceID,
		KubernetesWorkloadDefinitionID: &definitionID,
		KustomizeOverlay:               &existingOverlay,
		Status:                         util.Ptr("Up"),
	}

	handlerErrCh := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == api_v0.PathKubernetesWorkloadInstances:
			require.Equal(t, name, r.URL.Query().Get("name"))
			respondWithObject(t, w, existing)

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("%s/%s", api_v0.PathKubernetesRuntimeInstances, strconv.Itoa(int(runtimeInstanceID))):
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

			assert.Equal(t, &newOverlay, payload.KustomizeOverlay, "an explicitly-set new overlay value must be sent through, not silently replaced with the existing one")

			payload.ID = &existingID
			payload.CreatedAt = &createdAt
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
			Name: &name,
			KubernetesWorkloadDefinition: &KubernetesWorkloadDefinitionValues{
				Name: &definitionName,
			},
			KustomizeOverlay: &newOverlay,
		},
	}

	_, err := config.Replace(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), name)

	select {
	case handlerErr := <-handlerErrCh:
		t.Fatalf("mock server handler error: %v", handlerErr)
	default:
	}

	require.NoError(t, err)
}
