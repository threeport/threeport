package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	"github.com/threeport/threeport/internal/machinetest"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestV0LoggingInstanceCreatedEmitsReconciliationStartedBeforeFanOut covers
// ReconciliationStarted being recorded even when the definition fetch fails.
func TestV0LoggingInstanceCreatedEmitsReconciliationStartedBeforeFanOut(t *testing.T) {
	// fail every API call so only the started event and the fetch error remain
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// capture recorded events
	recorder := machinetest.NewFakeRecorder()
	r := &controller.Reconciler{
		APIClient:      server.Client(),
		APIServer:      server.URL,
		EventsRecorder: recorder,
	}

	// logging instance whose definition fetch will fail
	loggingInstance := &v0.LoggingInstance{
		Common:              v0.Common{ID: util.Ptr(uint(42))},
		Instance:            v0.Instance{Name: util.Ptr("test-logging-instance")},
		LoggingDefinitionID: util.Ptr(uint(1)),
	}

	log := logr.Discard()
	// run create reconciliation against the failing API
	_, err := v0LoggingInstanceCreated(r, loggingInstance, &log)

	// the fetch failure still surfaces; the recorder is what this test checks
	assert.Error(t, err, "handler should surface the downstream fetch failure")

	// one ReconciliationStarted event was recorded before the fetch failed
	events := recorder.GetEvents()
	assert.Len(t, events, 1, "one ReconciliationStarted event should be recorded")
	if len(events) == 0 {
		return
	}

	// the event names this logging instance
	got := events[0]
	assert.Equal(t, uint(42), got.ObjectID, "event object ID should match the logging instance")
	assert.Equal(t, "threeport.io/v0.LoggingInstance", got.Type, "event object type should be the fully qualified type")
	assert.NotNil(t, got.Event, "recorded event body should be populated")
	assert.Equal(t, "ReconciliationStarted", *got.Event.Reason)
	assert.Equal(t, "Normal", *got.Event.Type)
	assert.Contains(t, *got.Event.Note, "test-logging-instance")
	assert.Contains(t, *got.Event.Note, "starting reconciliation of logging instance")
}
