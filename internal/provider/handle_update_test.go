package provider

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestHandleInfraUpdate_CreationUnconfirmed_Requeues covers an update that
// arrives while the create is still running. A zero return would let the
// reconciler mark the instance reconciled before it is.
func TestHandleInfraUpdate_CreationUnconfirmed_Requeues(t *testing.T) {
	// acknowledge a creation that has not been confirmed
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		ResourceInventory:    jsonPtr(`{"deployment":{}}`),
	})

	requeue, err := HandleInfraUpdate(fl, newTestLogger())

	require.NoError(t, err)
	assert.Equal(t, int64(createUnconfirmedRequeueSeconds), requeue)
	// the update leaves the create to its own handler
	assert.Equal(t, 0, fl.callCount("IsCreateComplete"))
	assert.Equal(t, 0, fl.callCount("OnCreateConfirmed"))
	assert.Equal(t, 0, fl.callCount("ConfirmCreation"))
}

// TestHandleInfraUpdate_FailedCreation_StillRequeues covers an instance whose
// create failed. It is not reconciled either.
func TestHandleInfraUpdate_FailedCreation_StillRequeues(t *testing.T) {
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		CreationFailed:       true,
	})

	requeue, err := HandleInfraUpdate(fl, newTestLogger())

	require.NoError(t, err)
	assert.Equal(t, int64(createUnconfirmedRequeueSeconds), requeue)
}

// TestHandleInfraUpdate_CreationConfirmed_NoRequeue covers an update of an
// instance whose create is done.
func TestHandleInfraUpdate_CreationConfirmed_NoRequeue(t *testing.T) {
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationConfirmed: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})

	requeue, err := HandleInfraUpdate(fl, newTestLogger())

	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
}

// TestHandleInfraUpdate_DeletionScheduled_NoRequeue covers an update of an
// instance being deleted before its create was confirmed.
func TestHandleInfraUpdate_DeletionScheduled_NoRequeue(t *testing.T) {
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		DeletionScheduled:    util.Ptr(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)),
	})

	requeue, err := HandleInfraUpdate(fl, newTestLogger())

	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
}

// TestHandleInfraUpdate_GetReconciliationError covers a failed read of the
// instance.
func TestHandleInfraUpdate_GetReconciliationError(t *testing.T) {
	errAPI := errors.New("api unavailable")
	fl := newFakeLifecycle(&ReconciliationSnapshot{})
	fl.setErr("GetReconciliation", errAPI)

	requeue, err := HandleInfraUpdate(fl, newTestLogger())

	require.ErrorIs(t, err, errAPI)
	assert.Equal(t, int64(0), requeue)
}
