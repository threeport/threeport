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

// recordedRequest is one captured HTTP request from the mock API.
type recordedRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

// mockEventAPI is an httptest handler that records inbound requests for
// assertions.
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

// newRecorderForTest returns an EventRecorder pointed at an httptest server
// that records inbound requests. The cleanup function closes the server.
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

// findRequest returns the first recorded request matching method and path
// prefix, or fails the test.
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

// TestRecordEvent_AlwaysPostsRawRowWithCountOne covers a single POST of an
// event row with Count 1 and stamped controller, times, and object fields.
func TestRecordEvent_AlwaysPostsRawRowWithCountOne(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// record the event
	err := rec.RecordEvent(baseEvent(), 42, "threeport.io/v0.KubernetesWorkloadInstance")
	require.NoError(t, err)

	// check a single POST and no preceding GET
	mock.mu.Lock()
	require.Len(t, mock.requests, 1, "recorder should not issue a preceding GET; the api server upserts on insert")
	assert.Equal(t, http.MethodPost, mock.requests[0].method)
	assert.Equal(t, api.PathEvents, mock.requests[0].path)
	mock.mu.Unlock()

	// check Count 1, timestamps, controller, and object fields
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

// TestHandleEventOverride_UsesErrWithEventWhenPresent covers recording the
// event carried on the error instead of the fallback event.
func TestHandleEventOverride_UsesErrWithEventWhenPresent(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// set the override event on the error
	specific := api.Event{
		Reason: util.Ptr("SSHConnectFailed"),
		Note:   util.Ptr("dial tcp: refused"),
		Type:   util.Ptr(TypeWarning),
	}
	errWith := &tp_errors.ErrWithEvent{Message: "ssh failed", Event: specific}

	// run the override handler
	logger := logr.Discard()
	generic := &api.Event{
		Reason: util.Ptr("FailedCreate"),
		Note:   util.Ptr("wrapper"),
		Type:   util.Ptr(TypeWarning),
	}
	rec.HandleEventOverride(generic, 42, "threeport.io/v0.MachineRuntimeInstance", errWith, &logger)

	// check the posted reason is the override
	post := findRequest(t, mock, http.MethodPost, api.PathEvents)
	var posted api.Event
	require.NoError(t, json.Unmarshal(post.body, &posted))
	require.NotNil(t, posted.Reason)
	assert.Equal(t, "SSHConnectFailed", *posted.Reason, "override event carried by ErrWithEvent takes precedence")
}

// TestHandleEventOverride_UnwrapsWrappedErrWithEvent covers an override
// event nested under errors.Join still supplying the recorded reason.
func TestHandleEventOverride_UnwrapsWrappedErrWithEvent(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// set the override event on a joined error
	specific := api.Event{
		Reason: util.Ptr("CreateResourceError"),
		Note:   util.Ptr("api call rejected"),
		Type:   util.Ptr(TypeWarning),
	}
	inner := &tp_errors.ErrWithEvent{Message: "boom", Event: specific}
	wrapped := errors.Join(errors.New("outer"), inner)

	// run the override handler
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

// TestHandleEventOverride_UsesFallbackWhenNoErrWithEvent covers recording
// the fallback event when the error carries no override.
func TestHandleEventOverride_UsesFallbackWhenNoErrWithEvent(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t)
	defer cleanup()

	// run the override handler with a plain error
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
