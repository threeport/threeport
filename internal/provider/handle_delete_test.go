package provider

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

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

// TestHandleInfraDelete_CreatedButNoInventory_RefusesDelete covers a delete
// of an instance whose creation was confirmed but whose inventory is empty.
func TestHandleInfraDelete_CreatedButNoInventory_RefusesDelete(t *testing.T) {
	// cases for each form of empty inventory
	cases := []struct {
		name      string
		inventory *datatypes.JSON
	}{
		{name: "nil inventory", inventory: nil},
		{name: "zero-length inventory", inventory: jsonPtr("")},
		{name: "json null inventory", inventory: jsonPtr("null")},
		{name: "empty object inventory", inventory: jsonPtr("{}")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// set up a confirmed create with an empty inventory and no delete ack
			fl := newFakeLifecycle(&ReconciliationSnapshot{
				DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Minute)),
				CreationAcknowledged: util.Ptr(deleteTestBase.Add(-time.Hour)),
				CreationConfirmed:    util.Ptr(deleteTestBase.Add(-30 * time.Minute)),
				ResourceInventory:    tc.inventory,
			})

			// run delete
			requeue, err := HandleInfraDelete(fl, newTestLogger())

			// refuse with the empty-inventory error
			require.ErrorIs(t, err, ErrEmptyInventory)
			assert.Equal(t, int64(0), requeue)
			// skip ack, build, destroy, and confirm
			assert.Equal(t, 0, fl.callCount("AckDeletion"))
			assert.Equal(t, 0, fl.callCount("BuildInfra"))
			assert.Equal(t, 0, fl.callCount("ConfirmDeletion"))
			assert.Equal(t, 0, fl.callCount("ClearInventory"))
			assert.Equal(t, int64(0), inFlightCount())
		})
	}
}

// TestHandleInfraDelete_CreatedNilInventory_StaleAck_RefusesRelaunch covers an
// acknowledged delete whose nil inventory is unknown, not cleared, so the
// relaunch after a stale ack is refused instead of confirmed.
func TestHandleInfraDelete_CreatedNilInventory_StaleAck_RefusesRelaunch(t *testing.T) {
	// install test config and frozen clock
	restoreCfg := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreCfg)
	restoreClk := setLifecycleClock(newFakeClock(deleteTestBase))
	t.Cleanup(restoreClk)

	// set up a ten-minute-old delete ack and a nil inventory
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-10 * time.Minute)),
		CreationConfirmed:    util.Ptr(deleteTestBase.Add(-30 * time.Minute)),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// refuse rather than confirm
	require.ErrorIs(t, err, ErrEmptyInventory)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 0, fl.callCount("ConfirmDeletion"))
	assert.Equal(t, 0, fl.callCount("OnDeleteConfirmed"))
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
}

// TestHandleInfraDelete_CreatedNilInventory_FreshAck_NotConfirmed covers an
// acknowledged delete with a nil inventory and a fresh ack: it waits and is
// never confirmed.
func TestHandleInfraDelete_CreatedNilInventory_FreshAck_NotConfirmed(t *testing.T) {
	// install test config and frozen clock
	restoreCfg := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreCfg)
	restoreClk := setLifecycleClock(newFakeClock(deleteTestBase))
	t.Cleanup(restoreClk)

	// set up a one-minute-old delete ack and a nil inventory
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed:    util.Ptr(deleteTestBase.Add(-30 * time.Minute)),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// requeue without confirming
	require.NoError(t, err)
	assert.Equal(t, int64(60), requeue)
	assert.Equal(t, 0, fl.callCount("ConfirmDeletion"))
	assert.Equal(t, 0, fl.callCount("OnDeleteConfirmed"))
}

// TestHandleInfraDelete_CreatedAckedClearedSentinel_Confirms covers the
// completed destroy: a confirmed create whose inventory holds the {} sentinel
// after an acknowledged delete.
func TestHandleInfraDelete_CreatedAckedClearedSentinel_Confirms(t *testing.T) {
	// set up an acknowledged delete of a confirmed create with inventory "{}"
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed:    util.Ptr(deleteTestBase.Add(-2 * time.Hour)),
		ResourceInventory:    jsonPtr("{}"),
	})

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// confirm deletion
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fl.callCount("OnDeleteConfirmed"))
	assert.Equal(t, 1, fl.callCount("ConfirmDeletion"))
}

// TestHandleInfraDelete_CreatedWithInventory_Launches covers a confirmed
// create with recorded inventory proceeding to destroy.
func TestHandleInfraDelete_CreatedWithInventory_Launches(t *testing.T) {
	// install a one-slot semaphore
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// block destroy so the goroutine stays observable
	fi := newFakeInfra()
	fi.setDestroy(infraBlock, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed: util.Ptr(deleteTestBase.Add(-time.Hour)),
		ResourceInventory: validStackState(),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy against the restored state
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	assert.Equal(t, 1, fi.setStackStateCallCount())

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
}

// TestHandleInfraDelete_AbandonInfrastructure_Proceeds covers the operator
// override: a confirmed create with an empty inventory is deleted when
// abandonment was requested.
func TestHandleInfraDelete_AbandonInfrastructure_Proceeds(t *testing.T) {
	// install a one-slot semaphore
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// block destroy so the goroutine stays observable
	fi := newFakeInfra()
	fi.setDestroy(infraBlock, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:     util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed:     util.Ptr(deleteTestBase.Add(-time.Hour)),
		AbandonInfrastructure: true,
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy despite the empty inventory
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	assert.Equal(t, 1, fl.callCount("AckDeletion"))
	// skip state restore since there is none
	assert.Equal(t, 0, fi.setStackStateCallCount())

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
	// clear inventory so the next reconcile confirms
	assert.Equal(t, 1, fl.callCount("ClearInventory"))
}

// TestHandleInfraDelete_ClearInventoryRetried covers a failing ClearInventory
// being retried up to the configured attempts.
func TestHandleInfraDelete_ClearInventoryRetried(t *testing.T) {
	// install a one-slot semaphore and three persist attempts
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	cfg.PersistRetries = 3
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// fail every ClearInventory call
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
	})
	fl.setErr("ClearInventory", errors.New("injected: clear failure"))

	// run delete
	_, err := HandleInfraDelete(fl, newTestLogger())
	require.NoError(t, err)
	drainDeleteOps(t)

	// attempt the clear three times
	assert.Equal(t, 3, fl.callCount("ClearInventory"))
}

// TestHandleInfraDelete_NothingRecorded_CloudHasResources_Refuses covers a
// delete with no inventory and no confirmed create, which looks never-created
// but whose cluster the cloud still reports.
func TestHandleInfraDelete_NothingRecorded_CloudHasResources_Refuses(t *testing.T) {
	// install test config and frozen clock
	restoreCfg := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreCfg)
	restoreClk := setLifecycleClock(newFakeClock(deleteTestBase))
	t.Cleanup(restoreClk)

	// cases for each way the create history can look
	cases := []struct {
		name string
		snap *ReconciliationSnapshot
	}{
		{
			name: "no create history, as after a replace wiped it",
			snap: &ReconciliationSnapshot{
				DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
			},
		},
		{
			name: "interrupted create with a stale ack",
			snap: &ReconciliationSnapshot{
				DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Minute)),
				CreationAcknowledged: util.Ptr(deleteTestBase.Add(-time.Hour)),
			},
		},
		{
			name: "failed create",
			snap: &ReconciliationSnapshot{
				DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Minute)),
				CreationAcknowledged: util.Ptr(deleteTestBase.Add(-time.Hour)),
				CreationFailed:       true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fi := newFakeVerifierInfra(true, nil)
			fl := newFakeLifecycle(tc.snap)
			fl.setInfra(fi)

			// run delete
			requeue, err := HandleInfraDelete(fl, newTestLogger())

			// refuse before acking or destroying
			require.ErrorIs(t, err, ErrCloudResourcesExist)
			assert.Equal(t, int64(0), requeue)
			assert.Equal(t, 1, fi.existsCallCount())
			assert.Equal(t, 0, fl.callCount("AckDeletion"))
			assert.Equal(t, 0, fi.destroyCallCount())
			assert.Equal(t, 0, fl.callCount("ClearInventory"))
			assert.Equal(t, int64(0), inFlightCount())
		})
	}
}

// TestHandleInfraDelete_NothingRecorded_CloudEmpty_Launches covers a delete
// with nothing recorded where the cloud confirms there is nothing to destroy.
func TestHandleInfraDelete_NothingRecorded_CloudEmpty_Launches(t *testing.T) {
	// install a one-slot semaphore
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// block destroy so the goroutine stays observable
	fi := newFakeVerifierInfra(false, nil)
	fi.setDestroy(infraBlock, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy after one cloud check, building infra only once
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	assert.Equal(t, 1, fi.existsCallCount())
	assert.Equal(t, 1, fl.callCount("AckDeletion"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
}

// TestHandleInfraDelete_NothingRecorded_CloudLookupFails_Refuses covers a cloud
// lookup that cannot be made: the delete is not treated as safe.
func TestHandleInfraDelete_NothingRecorded_CloudLookupFails_Refuses(t *testing.T) {
	fi := newFakeVerifierInfra(false, errors.New("injected: permission denied"))
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// return the lookup failure without acking or destroying
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrCloudResourcesExist)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
	assert.Equal(t, 0, fi.destroyCallCount())
}

// TestHandleInfraDelete_NothingRecorded_Abandon_SkipsCloudCheck covers the
// override passing a delete the cloud would refuse, without asking it.
func TestHandleInfraDelete_NothingRecorded_Abandon_SkipsCloudCheck(t *testing.T) {
	// install a one-slot semaphore
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// block destroy so the goroutine stays observable
	fi := newFakeVerifierInfra(true, nil)
	fi.setDestroy(infraBlock, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:     util.Ptr(deleteTestBase.Add(-time.Minute)),
		AbandonInfrastructure: true,
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy without a cloud check
	require.NoError(t, err)
	assert.Equal(t, int64(300), requeue)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	assert.Equal(t, 0, fi.existsCallCount())

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
}

// TestHandleInfraDelete_CreatedNoInventory_CloudEmpty_StillRefuses covers a
// confirmed create with no inventory: an empty cluster lookup does not clear it,
// because the rest of the stack (network, router) cannot be checked or removed
// without state.
func TestHandleInfraDelete_CreatedNoInventory_CloudEmpty_StillRefuses(t *testing.T) {
	fi := newFakeVerifierInfra(false, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed: util.Ptr(deleteTestBase.Add(-time.Hour)),
	})
	fl.setInfra(fi)

	// run delete
	_, err := HandleInfraDelete(fl, newTestLogger())

	// refuse on the lost inventory, without a cloud check
	require.ErrorIs(t, err, ErrEmptyInventory)
	assert.Equal(t, 0, fi.existsCallCount())
	assert.Equal(t, 0, fl.callCount("AckDeletion"))
}

// TestHandleInfraDelete_CreatedWithInventory_SkipsPreLaunchCloudCheck covers a
// recorded inventory proceeding to destroy without a cloud check.
func TestHandleInfraDelete_CreatedWithInventory_SkipsPreLaunchCloudCheck(t *testing.T) {
	// install a one-slot semaphore
	cfg := testLifecycleConfig()
	cfg.SemaphoreCapacity = 1
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)

	// block destroy so the goroutine stays observable
	fi := newFakeVerifierInfra(true, nil)
	fi.setDestroy(infraBlock, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed: util.Ptr(deleteTestBase.Add(-time.Hour)),
		ResourceInventory: validStackState(),
	})
	fl.setInfra(fi)

	// run delete
	_, err := HandleInfraDelete(fl, newTestLogger())

	// launch destroy against recorded state without a cloud check
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	assert.Equal(t, 0, fi.existsCallCount())

	// release destroy and drain in-flight work
	fi.releaseDestroy()
	drainDeleteOps(t)
}

// TestHandleInfraDelete_AckedCleared_CloudStillHasCluster_RefusesConfirm covers
// a destroy that cleared the inventory while the cloud still reports the
// cluster: the delete is not confirmed.
func TestHandleInfraDelete_AckedCleared_CloudStillHasCluster_RefusesConfirm(t *testing.T) {
	fi := newFakeVerifierInfra(true, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed:    util.Ptr(deleteTestBase.Add(-2 * time.Hour)),
		ResourceInventory:    jsonPtr("{}"),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// refuse to confirm
	require.ErrorIs(t, err, ErrCloudResourcesExist)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 0, fl.callCount("OnDeleteConfirmed"))
	assert.Equal(t, 0, fl.callCount("ConfirmDeletion"))
}

// TestHandleInfraDelete_AckedCleared_CloudEmpty_Confirms covers the normal
// completed delete when the provider can be asked: the cloud agrees.
func TestHandleInfraDelete_AckedCleared_CloudEmpty_Confirms(t *testing.T) {
	fi := newFakeVerifierInfra(false, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:    util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged: util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed:    util.Ptr(deleteTestBase.Add(-2 * time.Hour)),
		ResourceInventory:    jsonPtr("{}"),
	})
	fl.setInfra(fi)

	// run delete
	requeue, err := HandleInfraDelete(fl, newTestLogger())

	// confirm after one cloud check
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fi.existsCallCount())
	assert.Equal(t, 1, fl.callCount("ConfirmDeletion"))
}

// TestHandleInfraDelete_AckedCleared_Abandon_ConfirmsWithoutCloudCheck covers
// the override confirming a delete the cloud would refuse, without asking it.
func TestHandleInfraDelete_AckedCleared_Abandon_ConfirmsWithoutCloudCheck(t *testing.T) {
	fi := newFakeVerifierInfra(true, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled:     util.Ptr(deleteTestBase.Add(-time.Hour)),
		DeletionAcknowledged:  util.Ptr(deleteTestBase.Add(-time.Minute)),
		CreationConfirmed:     util.Ptr(deleteTestBase.Add(-2 * time.Hour)),
		ResourceInventory:     jsonPtr("{}"),
		AbandonInfrastructure: true,
	})
	fl.setInfra(fi)

	// run delete
	_, err := HandleInfraDelete(fl, newTestLogger())

	// confirm without a cloud check
	require.NoError(t, err)
	assert.Equal(t, 0, fi.existsCallCount())
	assert.Equal(t, 1, fl.callCount("ConfirmDeletion"))
}
