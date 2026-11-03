package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestMachineWorkloadInstanceConfig_Replace_PreservesControllerState covers
// what a replace does to the fields the reconciler owns.
//
// The PUT a replace sends writes every field on the object, so one left off
// the payload is cleared rather than left alone. Editing an unrelated field -
// the Env map here - would otherwise wipe the instance's Status and
// reconciliation progress, which no caller of replace asked to change.
func TestMachineWorkloadInstanceConfig_Replace_PreservesControllerState(t *testing.T) {
	existingID, definitionID, runtimeID := uint(1), uint(2), uint(3)
	name := "test-machine-workload"
	existing := api_v0.MachineWorkloadInstance{
		Common:                      api_v0.Common{ID: &existingID},
		Instance:                    api_v0.Instance{Name: &name},
		Reconciliation:              api_v0.Reconciliation{Reconciled: util.Ptr(true)},
		MachineWorkloadDefinitionID: &definitionID,
		MachineRuntimeInstanceID:    &runtimeID,
		Status:                      util.Ptr("Running"),
	}

	var sent api_v0.MachineWorkloadInstance
	createdAt := time.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondWithObject(t, w, existing)

		case http.MethodPut:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			// deliberately echoing what was sent, rather than the values the
			// test wants to see: a handler that fills them in regardless is
			// how this defect went unnoticed in the first place
			replaced := sent
			replaced.CreatedAt = &createdAt
			respondWithObject(t, w, replaced)

		default:
			http.Error(w, "unexpected "+r.Method, http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	config := &MachineWorkloadInstanceConfig{
		MachineWorkloadInstance: MachineWorkloadInstanceValues{
			Name:                      &name,
			MachineWorkloadDefinition: &MachineWorkloadDefinitionValues{Name: util.Ptr("test-definition")},
			MachineRuntimeInstance:    &MachineRuntimeInstanceValues{Name: util.Ptr("test-runtime")},
			Env:                       []string{"EDITED=true"},
		},
	}

	_, err := config.Replace(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), name)
	require.NoError(t, err)

	assertReplacePreservesControllerState(t, existing.Reconciled, sent.Reconciled, existing.Status, sent.Status)
}

// assertReplacePreservesControllerState is the shared assertion proposed in
// threeport#565, so every type's replace test states this the same way.
func assertReplacePreservesControllerState(
	t *testing.T,
	wantReconciled, gotReconciled *bool,
	wantStatus, gotStatus *string,
) {
	t.Helper()
	assert.Equal(t, wantReconciled, gotReconciled, "Reconciled must be carried over, not cleared")
	assert.Equal(t, wantStatus, gotStatus, "Status must be carried over, not cleared")
}
