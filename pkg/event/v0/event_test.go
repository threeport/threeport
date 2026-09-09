package v0

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// recordedRequest captures one inbound HTTP request so tests can
// assert on URL, query, method, and parsed body.
type recordedRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

// mockEventAPI is an httptest fake that returns 201 + the inbound
// body on POST /v0/events. The response uses the apiserver_lib.Response
// envelope so client_lib.GetResponse decodes correctly.
type mockEventAPI struct {
	t        *testing.T
	requests []recordedRequest
	mu       sync.Mutex
}

func (m *mockEventAPI) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, recordedRequest{
		method: r.Method,
		path:   r.URL.Path,
		query:  r.URL.Query(),
		body:   body,
	})
	m.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && r.URL.Path == api.PathEvents:
		// echo the inbound payload back with an assigned ID so RecordEvent
		// can read response.Data[0] without erroring on missing fields
		var ev api.Event
		require.NoError(m.t, json.Unmarshal(body, &ev))
		ev.ID = util.Ptr(uint(100))
		writeEnvelope(m.t, w, http.StatusCreated, []apiserver_lib.Object{ev})
	default:
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

// writeEnvelope marshals data into an apiserver_lib.Response envelope
// and writes it with the given status. RecordEvent's callers use
// client_lib.GetResponse, which expects this exact shape.
func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, data []apiserver_lib.Object) {
	t.Helper()
	body, err := json.Marshal(apiserver_lib.Response{Data: data})
	require.NoError(t, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// newRecorderForTest stands up the mock API and returns a recorder
// bound to its URL.
func newRecorderForTest(t *testing.T) (*EventRecorder, *mockEventAPI, func()) {
	t.Helper()
	mock := &mockEventAPI{t: t}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	// client_lib.GetResponse prepends "http://" to the configured
	// APIServer, so strip the scheme from the httptest URL to avoid
	// "http://http://..." double-prefix.
	rec := &EventRecorder{
		APIClient:           &http.Client{},
		APIServer:           strings.TrimPrefix(srv.URL, "http://"),
		ReportingController: "test-controller",
	}
	return rec, mock, srv.Close
}

// findRequest returns the first recorded request matching method and
// a path prefix. Fails the test if none match.
func findRequest(t *testing.T, mock *mockEventAPI, method, pathPrefix string) recordedRequest {
	t.Helper()
	mock.mu.Lock()
	defer mock.mu.Unlock()
	for _, r := range mock.requests {
		if r.method == method && strings.HasPrefix(r.path, pathPrefix) {
			return r
		}
	}
	t.Fatalf("expected %s %s* request, got %+v", method, pathPrefix, mock.requests)
	return recordedRequest{}
}

// baseEvent returns an Event populated as a controller would call
// RecordEvent with. Tests override fields as needed.
func baseEvent() *api.Event {
	return &api.Event{
		Reason: util.Ptr("ScriptFailed"),
		Note:   util.Ptr("script returned exit 1"),
		Type:   util.Ptr(TypeWarning),
	}
}

// TestRecordEvent_AlwaysPostsRawRowWithCountOne covers the writer
// contract: every emit POSTs Count=1. The api server upserts on
// idx_events_dedup and increments the stored count. The POST body
// carries the subject fields, the reporting controller, and non-nil
// EventTime / LastObservedTime.
func TestRecordEvent_AlwaysPostsRawRowWithCountOne(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// drive a single emit through the recorder; the fake API records
	// every inbound request so the assertions below check both what
	// was called and what was omitted.
	err := rec.RecordEvent(baseEvent(), 42, "threeport.io/v0.KubernetesWorkloadInstance")
	require.NoError(t, err)

	// exactly one request should have fired, and it must be the POST
	// on the events collection endpoint.
	mock.mu.Lock()
	require.Len(t, mock.requests, 1, "recorder should not issue a preceding GET; the api server upserts on insert")
	assert.Equal(t, http.MethodPost, mock.requests[0].method)
	assert.Equal(t, api.PathEvents, mock.requests[0].path)
	mock.mu.Unlock()

	// the body must carry Count=1, both timestamps set, the reporting
	// controller name, and the subject linkage fields.
	post := findRequest(t, mock, http.MethodPost, api.PathEvents)
	var posted api.Event
	require.NoError(t, json.Unmarshal(post.body, &posted))
	require.NotNil(t, posted.Count)
	assert.Equal(t, uint(1), *posted.Count, "each emit posts Count=1; the api server increments the stored count on conflict")
	require.NotNil(t, posted.EventTime)
	require.NotNil(t, posted.LastObservedTime)
	require.NotNil(t, posted.ReportingController)
	assert.Equal(t, "test-controller", *posted.ReportingController)
	require.NotNil(t, posted.ObjectType)
	require.NotNil(t, posted.ObjectID)
	assert.Equal(t, "threeport.io/v0.KubernetesWorkloadInstance", *posted.ObjectType)
	assert.Equal(t, uint(42), *posted.ObjectID)
}

// TestHandleEventOverride_UsesErrWithEventWhenPresent covers the
// error-substitution path: when the caller returns an ErrWithEvent,
// HandleEventOverride records the carried event instead of the
// generic fallback the wrapper would emit.
func TestHandleEventOverride_UsesErrWithEventWhenPresent(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// build a specific-reason event wrapped in ErrWithEvent; this
	// stands in for a v0 handler that returned a failure with an
	// override event attached.
	specific := api.Event{
		Reason: util.Ptr("SSHConnectFailed"),
		Note:   util.Ptr("dial tcp: refused"),
		Type:   util.Ptr(TypeWarning),
	}
	errWith := &tp_errors.ErrWithEvent{Message: "ssh failed", Event: specific}

	// call HandleEventOverride with the generic fallback plus the
	// wrapped err; the specific event should win.
	logger := logr.Discard()
	generic := &api.Event{
		Reason: util.Ptr("FailedCreate"),
		Note:   util.Ptr("wrapper"),
		Type:   util.Ptr(TypeWarning),
	}
	rec.HandleEventOverride(generic, 42, "threeport.io/v0.MachineRuntimeInstance", errWith, &logger)

	// the POST body should carry the ErrWithEvent's Reason, not the
	// generic wrapper's FailedCreate.
	post := findRequest(t, mock, http.MethodPost, api.PathEvents)
	var posted api.Event
	require.NoError(t, json.Unmarshal(post.body, &posted))
	require.NotNil(t, posted.Reason)
	assert.Equal(t, "SSHConnectFailed", *posted.Reason, "override event carried by ErrWithEvent takes precedence")
}

// TestHandleEventOverride_UnwrapsWrappedErrWithEvent covers the
// errors.As unwrap contract: an ErrWithEvent wrapped by fmt.Errorf
// (e.g. a caller that added context before returning) still routes
// to the substitution path.
func TestHandleEventOverride_UnwrapsWrappedErrWithEvent(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// wrap the ErrWithEvent one layer deep to model a caller that
	// added context; errors.As should still find the sentinel.
	specific := api.Event{
		Reason: util.Ptr("CreateResourceError"),
		Note:   util.Ptr("api call rejected"),
		Type:   util.Ptr(TypeWarning),
	}
	inner := &tp_errors.ErrWithEvent{Message: "boom", Event: specific}
	wrapped := errors.Join(errors.New("outer"), inner)

	// drive the override with the wrapped err; expect the specific
	// reason to land in the POST body.
	logger := logr.Discard()
	generic := &api.Event{
		Reason: util.Ptr("FailedCreate"),
		Note:   util.Ptr("wrapper"),
		Type:   util.Ptr(TypeWarning),
	}
	rec.HandleEventOverride(generic, 7, "threeport.io/v0.KubernetesWorkloadInstance", wrapped, &logger)

	post := findRequest(t, mock, http.MethodPost, api.PathEvents)
	var posted api.Event
	require.NoError(t, json.Unmarshal(post.body, &posted))
	require.NotNil(t, posted.Reason)
	assert.Equal(t, "CreateResourceError", *posted.Reason, "errors.As unwraps ErrWithEvent through fmt.Errorf/errors.Join layers")
}

// TestHandleEventOverride_UsesFallbackWhenNoErrWithEvent covers the
// no-override path: a plain error routes the generic wrapper event
// through, unchanged.
func TestHandleEventOverride_UsesFallbackWhenNoErrWithEvent(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// plain error, no ErrWithEvent underneath: recorder falls back to
	// the caller-supplied fallback event (the generic FailedCreate).
	logger := logr.Discard()
	generic := &api.Event{
		Reason: util.Ptr("FailedCreate"),
		Note:   util.Ptr("wrapper"),
		Type:   util.Ptr(TypeWarning),
	}
	rec.HandleEventOverride(generic, 42, "threeport.io/v0.KubernetesWorkloadInstance", errors.New("plain"), &logger)

	post := findRequest(t, mock, http.MethodPost, api.PathEvents)
	var posted api.Event
	require.NoError(t, json.Unmarshal(post.body, &posted))
	require.NotNil(t, posted.Reason)
	assert.Equal(t, "FailedCreate", *posted.Reason, "no override sentinel; fallback event is stored as-is")
}
