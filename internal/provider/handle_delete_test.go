package provider

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// deleteTestBase is the frozen clock origin for ack-age calculations.
var deleteTestBase = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// drainDeleteOps waits until no operation is in flight and the
// semaphore holds no slots.
func drainDeleteOps(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		return inFlightCount() == 0 && len(currentSemaphore()) == 0
	}, 5*time.Second, 5*time.Millisecond, "in-flight infrastructure operations did not drain")
}

// TestHandleInfraDelete_NotScheduled_Error covers a delete
// notification whose snapshot has no deletion scheduled.
func TestHandleInfraDelete_NotScheduled_Error(t *testing.T) {
	// set up an empty reconciliation snapshot
	fl := newFakeLifecycle()

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// reject an unscheduled delete
	require.Error(t, err)
	assert.Contains(t, err.Error(), "received but not scheduled")
	assert.Equal(t, int64(0), requeue)
	// skip ack and build
	assert.Equal(t, 1, fl.callCount("GetReconciliation"))
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
}

// TestHandleInfraDelete_AlreadyConfirmed_EarlyReturn covers a
// snapshot that already has deletion confirmed.
func TestHandleInfraDelete_AlreadyConfirmed_EarlyReturn(t *testing.T) {
	// set up a snapshot that is already confirmed
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionConfirmed: util.Ptr(deleteTestBase.Add(-time.Minute)),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// accept a no-op with a zero requeue
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	// skip ack, build, and a destroy goroutine
	assert.Equal(t, 1, fl.callCount("GetReconciliation"))
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraDelete_CrossReplicaSafety_Requeue60 covers a
// fresh create acknowledgement that is not yet confirmed.
func TestHandleInfraDelete_CrossReplicaSafety_Requeue60(t *testing.T) {
	// install test config and frozen clock
	restoreCfg := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreCfg)
	clk := newFakeClock(deleteTestBase)
	restoreClk := setLifecycleClock(clk)
	t.Cleanup(restoreClk)

	// set up a one-minute-old unconfirmed create ack against a 240s stale threshold
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		CreationAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// requeue 60 seconds without launching
	require.NoError(t, err)
	assert.Equal(t, int64(60), requeue)
	// skip ack, build, and a destroy goroutine
	assert.Equal(t, 1, fl.callCount("GetReconciliation"))
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraDelete_StaleCreateAck_AllowsDelete covers a
// stale create acknowledgement that does not block delete.
func TestHandleInfraDelete_StaleCreateAck_AllowsDelete(t *testing.T) {
	// install a one-slot semaphore and frozen clock
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)
	clk := newFakeClock(deleteTestBase)
	restoreClk := setLifecycleClock(clk)
	t.Cleanup(restoreClk)

	// block destroy so the goroutine stays observable
	fi := newFakeInfra()
	fi.setDestroy(infraBlock, nil)
	// set up a ten-minute-old unconfirmed create ack against a 240s stale threshold
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		CreationAcknowledged: util.Ptr(deleteTestBase.Add(-10 * time.Minute)),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy and requeue 300 seconds
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	// ack, build, and fetch reconciliation twice
	assert.Equal(t, 1, fl.callCount("AckDeletion"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))
	assert.Equal(t, 2, fl.callCount("GetReconciliation"))

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
	// clear inventory and publish the delete notification
	assert.Equal(t, 1, fl.callCount("ClearInventory"))
	assert.Equal(t, 1, fl.callCount("PublishDeleteNotification"))
}

// TestHandleInfraDelete_FreshAckButCreateFailed_StillRequeues60
// covers a fresh create ack that still requeues when CreationFailed is set.
func TestHandleInfraDelete_FreshAckButCreateFailed_StillRequeues60(t *testing.T) {
	// install test config and frozen clock
	restoreCfg := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreCfg)
	clk := newFakeClock(deleteTestBase)
	restoreClk := setLifecycleClock(clk)
	t.Cleanup(restoreClk)

	// set up a one-minute-old create ack with CreationFailed set
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		CreationAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationFailed:       true,
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// requeue 60 seconds without launching
	require.NoError(t, err)
	assert.Equal(t, int64(60), requeue)
	// skip ack, build, and a destroy goroutine
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraDelete_AckedInventoryCleared_Confirms covers an
// acknowledged delete whose inventory is already cleared.
func TestHandleInfraDelete_AckedInventoryCleared_Confirms(t *testing.T) {
	// set up an acknowledged delete with inventory "{}"
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		ResourceInventory:    jsonPtr("{}"),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// confirm deletion without launching
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 2, fl.callCount("GetReconciliation"))
	assert.Equal(t, 1, fl.callCount("RefreshDeletionAck"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))
	assert.Equal(t, 1, fl.callCount("OnDeleteConfirmed"))
	assert.Equal(t, 1, fl.callCount("ConfirmDeletion"))
	// skip a new ack and a destroy goroutine
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraDelete_OnDeleteConfirmedError_Requeue60 covers a
// post-deletion cleanup error that requeues without confirming.
func TestHandleInfraDelete_OnDeleteConfirmedError_Requeue60(t *testing.T) {
	// set up an acknowledged delete with a cleared inventory
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		ResourceInventory:    jsonPtr("{}"),
	})
	// inject a post-deletion cleanup failure
	fl.setErr("OnDeleteConfirmed", errors.New("injected: post-deletion cleanup failure"))

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// requeue 60 seconds without confirming or returning an error
	require.NoError(t, err)
	assert.Equal(t, int64(60), requeue)
	assert.Equal(t, 1, fl.callCount("RefreshDeletionAck"))
	assert.Equal(t, 1, fl.callCount("OnDeleteConfirmed"))
	assert.Equal(t, 0, fl.callCount("ConfirmDeletion"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraDelete_AckedInventoryNotCleared_FreshAck_Requeue60
// covers an in-progress delete whose acknowledgement is still fresh.
func TestHandleInfraDelete_AckedInventoryNotCleared_FreshAck_Requeue60(t *testing.T) {
	// install test config and frozen clock
	restoreCfg := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreCfg)
	clk := newFakeClock(deleteTestBase)
	restoreClk := setLifecycleClock(clk)
	t.Cleanup(restoreClk)

	// set up remaining inventory and a one-minute-old ack against a 240s stale threshold
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		ResourceInventory:    validStackState(),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// requeue 60 seconds without launching
	require.NoError(t, err)
	assert.Equal(t, int64(60), requeue)
	assert.Equal(t, 2, fl.callCount("GetReconciliation"))
	// skip refresh, a new ack, and build
	assert.Equal(t, 0, fl.callCount("RefreshDeletionAck"))
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraDelete_AckedInventoryNotCleared_StaleAck_Relaunches
// covers a stale deletion acknowledgement that launches destroy.
func TestHandleInfraDelete_AckedInventoryNotCleared_StaleAck_Relaunches(t *testing.T) {
	// install a one-slot semaphore and frozen clock
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)
	clk := newFakeClock(deleteTestBase)
	restoreClk := setLifecycleClock(clk)
	t.Cleanup(restoreClk)

	// block destroy so the goroutine stays observable
	fi := newFakeInfra()
	fi.setDestroy(infraBlock, nil)
	// set up remaining inventory and a ten-minute-old ack against a 240s stale threshold
	inventory := validStackState()
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-10 * time.Minute)),
		ResourceInventory:    inventory,
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy and requeue 300 seconds
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	// ack, build, and fetch reconciliation three times
	assert.Equal(t, 1, fl.callCount("AckDeletion"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))
	assert.Equal(t, 3, fl.callCount("GetReconciliation"))

	// restore stack state, already finished once destroy has started
	assert.Equal(t, 1, fi.setStackStateCallCount())
	assert.Equal(t, inventory, fi.lastRestoredState())

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
	// clear inventory and publish the delete notification
	assert.Equal(t, 1, fl.callCount("ClearInventory"))
	assert.Equal(t, 1, fl.callCount("PublishDeleteNotification"))
}

// TestHandleInfraDelete_NewRequest_AcksBuildsLaunches covers a
// first-time delete that acknowledges, builds, and launches destroy.
func TestHandleInfraDelete_NewRequest_AcksBuildsLaunches(t *testing.T) {
	// install a one-slot semaphore
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// block destroy so the goroutine stays observable
	fi := newFakeInfra()
	fi.setDestroy(infraBlock, nil)
	// set up a scheduled delete with no deletion acknowledgement
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy and requeue 300 seconds
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	// ack, build, and fetch reconciliation twice
	assert.Equal(t, 1, fl.callCount("AckDeletion"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))
	assert.Equal(t, 2, fl.callCount("GetReconciliation"))

	// skip state restore when inventory is nil
	assert.Equal(t, 0, fi.setStackStateCallCount())

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
	// clear inventory and publish the delete notification
	assert.Equal(t, 1, fl.callCount("ClearInventory"))
	assert.Equal(t, 1, fl.callCount("PublishDeleteNotification"))
}
