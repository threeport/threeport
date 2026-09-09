package provider

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// createTestConfig returns the production 240s stale-ack threshold, a
// one-hour refresh so the ack refresher does not tick, a semaphore of
// size 1, one persist attempt, and a 1ms delay.
func createTestConfig() LifecycleConfig {
	return LifecycleConfig{
		StaleAckThreshold: 240 * time.Second,
		RefreshInterval:   time.Hour,
		SemaphoreCapacity: 1,
		PersistRetries:    1,
		PersistRetryDelay: time.Millisecond,
	}
}

// waitForCreateCond fatals if cond is still false after 10 seconds.
func waitForCreateCond(t *testing.T, desc string, cond func() bool) {
	// register as a test helper
	t.Helper()
	// bound the wait at 10 seconds
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// return once the condition holds
		if cond() {
			return
		}
		// wait 1ms between polls
		time.Sleep(time.Millisecond)
	}
	// fail the test with the caller's wait description
	t.Fatalf("timed out waiting for %s", desc)
}

// TestHandleInfraCreate_AlreadyConfirmed_EarlyReturn covers a create
// whose creation is already confirmed.
func TestHandleInfraCreate_AlreadyConfirmed_EarlyReturn(t *testing.T) {
	// set a confirmed creation timestamp
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationConfirmed: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert a clean return with no requeue
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	// assert later create steps were not reached
	assert.Equal(t, 1, fl.callCount("GetReconciliation"))
	assert.Equal(t, 0, fl.callCount("IsCreateComplete"))
	assert.Equal(t, 0, fl.callCount("AckCreation"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, 0, fl.callCount("ConfirmCreation"))
}

// TestHandleInfraCreate_AckedComplete_ConfirmsInOrder covers an
// acknowledged create that has already finished.
func TestHandleInfraCreate_AckedComplete_ConfirmsInOrder(t *testing.T) {
	// acknowledge creation, leave it not failed, and mark it complete
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	fl.setCreateComplete(true)
	fi := newFakeInfra()
	fl.setInfra(fi)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert confirmation ran with no requeue
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fl.callCount("IsCreateComplete"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))
	assert.Equal(t, 1, fl.callCount("OnCreateConfirmed"))
	assert.Equal(t, 1, fl.callCount("ConfirmCreation"))

	// assert no re-ack and no deploy
	assert.Equal(t, 0, fl.callCount("AckCreation"))
	assert.Equal(t, 0, fi.deployCallCount())
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraCreate_OnCreateConfirmedError_Propagates covers a
// post-creation failure that stops confirmation.
func TestHandleInfraCreate_OnCreateConfirmedError_Propagates(t *testing.T) {
	// fail post-creation work on an otherwise complete create
	errPostCreate := errors.New("post-creation work failed")
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	fl.setCreateComplete(true)
	fl.setErr("OnCreateConfirmed", errPostCreate)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert the post-creation error is wrapped and confirmation is skipped
	require.Error(t, err)
	assert.ErrorIs(t, err, errPostCreate)
	assert.Contains(t, err.Error(), "failed to run post-creation work")
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fl.callCount("OnCreateConfirmed"))
	assert.Equal(t, 0, fl.callCount("ConfirmCreation"))
}

// TestHandleInfraCreate_AckedIncomplete_FreshAck_Requeue120 covers
// an in-progress create whose acknowledgement is still fresh.
func TestHandleInfraCreate_AckedIncomplete_FreshAck_Requeue120(t *testing.T) {
	// install create-test lifecycle config
	restoreConfig := setLifecycleConfig(createTestConfig())
	t.Cleanup(restoreConfig)

	// freeze the clock at the acknowledgement time
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	restoreClock := setLifecycleClock(newFakeClock(base))
	t.Cleanup(restoreClock)

	// acknowledge creation at the same instant, still incomplete
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(base),
	})

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert a 120s requeue and no relaunch
	require.NoError(t, err)
	assert.Equal(t, int64(120), requeue)
	assert.Equal(t, 1, fl.callCount("IsCreateComplete"))
	assert.Equal(t, 0, fl.callCount("AckCreation"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraCreate_StaleAck_Relaunches covers an in-progress create
// whose acknowledgement is older than the stale threshold.
func TestHandleInfraCreate_StaleAck_Relaunches(t *testing.T) {
	// install create-test lifecycle config
	restoreConfig := setLifecycleConfig(createTestConfig())
	t.Cleanup(restoreConfig)

	// freeze the clock 241s after acknowledgement, past the 240s threshold
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	restoreClock := setLifecycleClock(newFakeClock(base.Add(241 * time.Second)))
	t.Cleanup(restoreClock)

	// block deploy so the test can observe the relaunch
	fi := newFakeInfra()
	fi.setDeploy(infraBlock, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationAcknowledged: util.Ptr(base),
	})
	fl.setInfra(fi)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert a 120s requeue and a fresh acknowledgement
	require.NoError(t, err)
	assert.Equal(t, int64(120), requeue)
	assert.Equal(t, 1, fl.callCount("AckCreation"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))

	// wait for the deploy goroutine to start
	waitForCreateCond(t, "deploy launch", func() bool {
		return fi.deployCallCount() == 1
	})
	// unblock deploy and wait for in-flight work to drain
	fi.releaseDeploy()
	waitForCreateCond(t, "in-flight drain", func() bool {
		return inFlightCount() == 0
	})
}

// TestHandleInfraCreate_NewRequest_AcksBuildsLaunches covers a create
// with no acknowledgement yet.
func TestHandleInfraCreate_NewRequest_AcksBuildsLaunches(t *testing.T) {
	// install create-test lifecycle config
	restoreConfig := setLifecycleConfig(createTestConfig())
	t.Cleanup(restoreConfig)

	// block deploy so the test can observe the launch
	fi := newFakeInfra()
	fi.setDeploy(infraBlock, nil)
	fl := newFakeLifecycle()
	fl.setInfra(fi)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert acknowledgement, infra build, and a 120s requeue
	require.NoError(t, err)
	assert.Equal(t, int64(120), requeue)
	assert.Equal(t, 1, fl.callCount("AckCreation"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))

	// assert two fetches: initial plus pre-launch, success path not yet run
	assert.Equal(t, 2, fl.callCount("GetReconciliation"))

	// wait for the deploy goroutine to start
	waitForCreateCond(t, "deploy launch", func() bool {
		return fi.deployCallCount() == 1
	})
	// unblock deploy and wait for in-flight work to drain
	fi.releaseDeploy()
	waitForCreateCond(t, "in-flight drain", func() bool {
		return inFlightCount() == 0
	})
}

// TestHandleInfraCreate_AckCreationError covers a failed creation
// acknowledgement.
func TestHandleInfraCreate_AckCreationError(t *testing.T) {
	// fail acknowledgement on a new create
	errAck := errors.New("ack write failed")
	fl := newFakeLifecycle()
	fl.setErr("AckCreation", errAck)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert the ack error is wrapped and deploy is not launched
	require.Error(t, err)
	assert.ErrorIs(t, err, errAck)
	assert.Contains(t, err.Error(), "failed to acknowledge creation")
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraCreate_DeletionScheduledBeforeLaunch_Aborts covers a
// deletion scheduled after acknowledgement and before deploy.
func TestHandleInfraCreate_DeletionScheduledBeforeLaunch_Aborts(t *testing.T) {
	// serve a new-request snapshot, then one with deletion scheduled
	fi := newFakeInfra()
	fl := newFakeLifecycle(
		&ReconciliationSnapshot{},
		&ReconciliationSnapshot{
			DeletionScheduled: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
	)
	fl.setInfra(fi)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert ack and build ran, then create aborted without deploy
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fl.callCount("AckCreation"))
	assert.Equal(t, 1, fl.callCount("BuildInfra"))
	assert.Equal(t, 2, fl.callCount("GetReconciliation"))
	assert.Equal(t, 0, fi.deployCallCount())
	assert.Equal(t, int64(0), inFlightCount())
}

// TestHandleInfraCreate_DeletionScheduledDuringInfra_SuppressesNotification
// covers a deletion scheduled while deploy is running.
func TestHandleInfraCreate_DeletionScheduledDuringInfra_SuppressesNotification(t *testing.T) {
	// install create-test lifecycle config
	restoreConfig := setLifecycleConfig(createTestConfig())
	t.Cleanup(restoreConfig)

	// block deploy so the test can observe the launch
	fi := newFakeInfra()
	fi.setDeploy(infraBlock, nil)

	// serve new-request, pre-launch clear, then success-path deletion snapshots
	fl := newFakeLifecycle(
		&ReconciliationSnapshot{},
		&ReconciliationSnapshot{},
		&ReconciliationSnapshot{
			DeletionScheduled: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
	)
	fl.setInfra(fi)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert a 120s requeue after launch
	require.NoError(t, err)
	assert.Equal(t, int64(120), requeue)

	// wait for the deploy goroutine to start
	waitForCreateCond(t, "deploy launch", func() bool {
		return fi.deployCallCount() == 1
	})
	// let deploy finish so the success path runs, then drain
	fi.releaseDeploy()
	waitForCreateCond(t, "in-flight drain", func() bool {
		return inFlightCount() == 0
	})

	// assert outputs were saved and the create notification was skipped
	assert.Equal(t, 1, fl.callCount("SaveCreateOutputs"))
	assert.Equal(t, validStackState(), fl.createOutputs())
	assert.Equal(t, 0, fl.callCount("PublishCreateNotification"))
}

// TestHandleInfraCreate_GetReconciliationError_FirstFetch covers a
// reconciliation fetch that fails on the first call.
func TestHandleInfraCreate_GetReconciliationError_FirstFetch(t *testing.T) {
	// fail the first reconciliation fetch
	errFetch := errors.New("api unavailable")
	fl := newFakeLifecycle()
	fl.setErr("GetReconciliation", errFetch)

	// run the create handler
	requeue, err := HandleInfraCreate(fl, newTestLogger())

	// assert the fetch error is wrapped and later steps were not reached
	require.Error(t, err)
	assert.ErrorIs(t, err, errFetch)
	assert.Contains(t, err.Error(), "failed to get reconciliation state")
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 0, fl.callCount("AckCreation"))
	assert.Equal(t, 0, fl.callCount("BuildInfra"))
}
