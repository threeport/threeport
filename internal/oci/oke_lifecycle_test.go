package oci

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestOkeLifecycle_GetReconciliation_AbandonInfrastructure covers the operator
// override reaching the shared delete state machine. A snapshot that drops it
// leaves a held delete refused no matter what the operator sets.
func TestOkeLifecycle_GetReconciliation_AbandonInfrastructure(t *testing.T) {
	cases := []struct {
		name    string
		abandon *bool
		want    bool
	}{
		{name: "unset is not abandoned", abandon: nil, want: false},
		{name: "false is not abandoned", abandon: util.Ptr(false), want: false},
		{name: "true is abandoned", abandon: util.Ptr(true), want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instanceID := uint(7)
			instance := v0.OciOkeKubernetesRuntimeInstance{
				Common:                v0.Common{ID: &instanceID},
				AbandonInfrastructure: tc.abandon,
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
					Data:   []apiserver_lib.Object{instance},
				})
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			lifecycle := &okeLifecycle{
				r: &controller.Reconciler{
					APIClient: &http.Client{},
					APIServer: strings.TrimPrefix(srv.URL, "http://"),
				},
				instanceID: instanceID,
			}

			snap, err := lifecycle.GetReconciliation()
			require.NoError(t, err)
			assert.Equal(t, tc.want, snap.AbandonInfrastructure)
		})
	}
}

// recordingRecorder captures the events passed to RecordEvent.
type recordingRecorder struct {
	events    []*v0.Event
	objectIDs []uint
	types     []string
}

// RecordEvent records the call.
func (r *recordingRecorder) RecordEvent(event *v0.Event, objectID uint, fullyQualifiedType string) error {
	r.events = append(r.events, event)
	r.objectIDs = append(r.objectIDs, objectID)
	r.types = append(r.types, fullyQualifiedType)
	return nil
}

// HandleEventOverride is unused by the lifecycle.
func (r *recordingRecorder) HandleEventOverride(*v0.Event, uint, string, error, *logr.Logger) {}

// TestOkeLifecycle_RecordWarning covers the Warning event reaching the
// instance's events, bound to the right object type and ID.
func TestOkeLifecycle_RecordWarning(t *testing.T) {
	recorder := &recordingRecorder{}
	lifecycle := &okeLifecycle{
		r:          &controller.Reconciler{EventsRecorder: recorder},
		instanceID: 7,
	}

	err := lifecycle.RecordWarning("InfrastructureAbandoned", "left running")

	require.NoError(t, err)
	require.Len(t, recorder.events, 1)
	assert.Equal(t, "Warning", *recorder.events[0].Type)
	assert.Equal(t, "InfrastructureAbandoned", *recorder.events[0].Reason)
	assert.Equal(t, "left running", *recorder.events[0].Note)
	assert.Equal(t, []uint{7}, recorder.objectIDs)
	assert.Equal(t, []string{new(v0.OciOkeKubernetesRuntimeInstance).GetFullyQualifiedType()}, recorder.types)
}

// TestOkeLifecycle_RecordWarning_NoRecorder covers a reconciler built
// without an events recorder: an error, not a panic.
func TestOkeLifecycle_RecordWarning_NoRecorder(t *testing.T) {
	lifecycle := &okeLifecycle{r: &controller.Reconciler{}, instanceID: 7}

	assert.Error(t, lifecycle.RecordWarning("Reason", "note"))
}
