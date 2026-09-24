package v0

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// recordedRequest captures one inbound HTTP request so tests can
// assert on URL, query, method, and parsed body. The recorder
// dispatches based on path so a single httptest.Server can stand in
// for all three endpoints RecordEvent hits (GET join, POST events,
// PATCH events/:id).
type recordedRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

// mockEventAPI is an httptest fake that:
//   - returns existingEvents on the GET /v0/events-join-attached-object-references query
//   - returns 201 + the inbound body on POST /v0/events
//   - returns 200 + the inbound body on PATCH /v0/events/<id>
//
// All three responses use the apiserver_lib.Response envelope so
// client_lib.GetResponse decodes correctly.
type mockEventAPI struct {
	t              *testing.T
	existingEvents []api.Event
	requests       []recordedRequest
	mu             sync.Mutex
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
	case r.Method == http.MethodGet && r.URL.Path == "/v0/events-join-attached-object-references":
		writeEnvelope(m.t, w, http.StatusOK, eventsToObjects(m.existingEvents))
	case r.Method == http.MethodPost && r.URL.Path == api.PathEvents:
		// echo the inbound payload back with an assigned ID so RecordEvent
		// can read response.Data[0] without erroring on missing fields
		var ev api.Event
		require.NoError(m.t, json.Unmarshal(body, &ev))
		ev.ID = util.Ptr(uint(100))
		writeEnvelope(m.t, w, http.StatusCreated, []apiserver_lib.Object{ev})
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, api.PathEvents+"/"):
		var ev api.Event
		require.NoError(m.t, json.Unmarshal(body, &ev))
		writeEnvelope(m.t, w, http.StatusOK, []apiserver_lib.Object{ev})
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

func eventsToObjects(events []api.Event) []apiserver_lib.Object {
	out := make([]apiserver_lib.Object, len(events))
	for i := range events {
		out[i] = events[i]
	}
	return out
}

// newRecorderForTest stands up the mock API and returns a recorder
// bound to its URL. existingEvents controls what the join query
// returns: zero = the create path, one = the dedup-bump path, many =
// the unexpected-state error path.
func newRecorderForTest(t *testing.T, existingEvents []api.Event) (*EventRecorder, *mockEventAPI, func()) {
	t.Helper()
	mock := &mockEventAPI{t: t, existingEvents: existingEvents}
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
// a path prefix. Fails the test if none match - the test that called
// it expected exactly that interaction.
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

// TestRecordEvent_CreateWhenNoneExists exercises the 0-existing
// branch: the join query returns empty, so RecordEvent must POST a
// new Event with subject fields set and Count=1.
func TestRecordEvent_CreateWhenNoneExists(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t, nil)
	defer cleanup()

	err := rec.RecordEvent(baseEvent(), 42, "threeport.io/v0.KubernetesWorkloadInstance")
	require.NoError(t, err)

	// POST /v0/events should have fired with the subject fields set
	post := findRequest(t, mock, http.MethodPost, api.PathEvents)
	var posted api.Event
	require.NoError(t, json.Unmarshal(post.body, &posted))
	require.NotNil(t, posted.ObjectType)
	require.NotNil(t, posted.ObjectID)
	require.NotNil(t, posted.Count)
	require.NotNil(t, posted.ReportingController)
	assert.Equal(t, "threeport.io/v0.KubernetesWorkloadInstance", *posted.ObjectType)
	assert.Equal(t, uint(42), *posted.ObjectID)
	assert.Equal(t, uint(1), *posted.Count, "Count starts at 1 on first observation")
	assert.Equal(t, "test-controller", *posted.ReportingController)
	assert.NotNil(t, posted.EventTime)
	assert.NotNil(t, posted.LastObservedTime)
}

// TestRecordEvent_QueryEscaping verifies that the join-query
// parameters are URL-escaped. A reason or note containing &, =, or
// space must not split the query string into extra params.
func TestRecordEvent_QueryEscaping(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t, nil)
	defer cleanup()

	ev := baseEvent()
	ev.Reason = util.Ptr("Has Spaces & Ampersand=Sign")
	ev.Note = util.Ptr("Note with = and &")
	require.NoError(t, rec.RecordEvent(ev, 7, "threeport.io/v0.Foo"))

	get := findRequest(t, mock, http.MethodGet, "/v0/events-join-attached-object-references")
	assert.Equal(t, "Has Spaces & Ampersand=Sign", get.query.Get("reason"))
	assert.Equal(t, "Note with = and &", get.query.Get("note"))
	assert.Equal(t, TypeWarning, get.query.Get("type"))
	assert.Equal(t, "7", get.query.Get("objectid"),
		"objectid is rendered as a bare integer, not escaped")
}

// TestRecordEvent_DedupQueryIncludesSubjectType pins the dedup-lookup
// query shape against the events handler's subject filter. The handler
// rejects requests that supply objectid without objecttypename, so
// RecordEvent must split the fully qualified type and send all three
// parts (typename / namespace / version) alongside objectid.
func TestRecordEvent_DedupQueryIncludesSubjectType(t *testing.T) {
	rec, mock, cleanup := newRecorderForTest(t, nil)
	defer cleanup()

	require.NoError(t, rec.RecordEvent(baseEvent(), 42, "threeport.io/v0.KubernetesWorkloadInstance"))

	get := findRequest(t, mock, http.MethodGet, "/v0/events-join-attached-object-references")
	assert.Equal(t, "KubernetesWorkloadInstance", get.query.Get("objecttypename"))
	assert.Equal(t, "threeport.io", get.query.Get("objectnamespace"))
	assert.Equal(t, "v0", get.query.Get("objectversion"))
	assert.Equal(t, "42", get.query.Get("objectid"))
}

// TestRecordEvent_RejectsMalformedQualifiedType returns a clear error
// when the caller hands a string that doesn't parse as a fully
// qualified type. The dedup query would otherwise be silently broken.
func TestRecordEvent_RejectsMalformedQualifiedType(t *testing.T) {
	rec, _, cleanup := newRecorderForTest(t, nil)
	defer cleanup()

	err := rec.RecordEvent(baseEvent(), 42, "KubernetesWorkloadInstance")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fully qualified object type")
}

// TestRecordEvent_BumpsCountWhenOneExists exercises the 1-existing
// branch: an event matching (reason, note, type, objectid) already
// exists, so RecordEvent must PATCH it with Count+1 and clear the
// in-memory projection fields (UpdateEvent rejects them).
func TestRecordEvent_BumpsCountWhenOneExists(t *testing.T) {
	existing := api.Event{
		Common:           api.Common{ID: util.Ptr(uint(99))},
		Reason:           util.Ptr("ScriptFailed"),
		Note:             util.Ptr("script returned exit 1"),
		Type:             util.Ptr(TypeWarning),
		Count:            util.Ptr(uint(3)),
		EventTime:        util.Ptr(nowTime()),
		LastObservedTime: util.Ptr(nowTime()),
	}
	rec, mock, cleanup := newRecorderForTest(t, []api.Event{existing})
	defer cleanup()

	err := rec.RecordEvent(baseEvent(), 42, "threeport.io/v0.KubernetesWorkloadInstance")
	require.NoError(t, err)

	patch := findRequest(t, mock, http.MethodPatch, api.PathEvents+"/")
	assert.Equal(t, api.PathEvents+"/99", patch.path, "PATCH targets the existing event id")

	var patched api.Event
	require.NoError(t, json.Unmarshal(patch.body, &patched))
	require.NotNil(t, patched.Count)
	assert.Equal(t, uint(4), *patched.Count, "Count increments by 1")
	assert.Nil(t, patched.ObjectType, "projection fields cleared so UpdateEvent doesn't reject")
	assert.Nil(t, patched.ObjectID)
	assert.Nil(t, patched.ObjectName)
}

// TestRecordEvent_ErrorsWhenMultipleExist exercises the >1-existing
// branch: the (reason, note, type, objectid) dedup tuple should
// resolve to at most one row. More than one signals a race that
// produced duplicates - return an error so the caller can flag it
// rather than silently bump one and ignore the others.
func TestRecordEvent_ErrorsWhenMultipleExist(t *testing.T) {
	existing := []api.Event{
		{Common: api.Common{ID: util.Ptr(uint(1))}, Count: util.Ptr(uint(1))},
		{Common: api.Common{ID: util.Ptr(uint(2))}, Count: util.Ptr(uint(1))},
	}
	rec, _, cleanup := newRecorderForTest(t, existing)
	defer cleanup()

	err := rec.RecordEvent(baseEvent(), 42, "threeport.io/v0.KubernetesWorkloadInstance")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected number of events")
}

// nowTime is a helper that returns a time.Time value the test
// builder can take an address of via util.Ptr.
func nowTime() time.Time { return time.Now() }
