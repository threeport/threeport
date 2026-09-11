package provider

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// orderRecordingInfra implements RefreshableProvider.
var _ RefreshableProvider = (*orderRecordingInfra)(nil)

// configureSemaphoreTest sets semaphore capacity for the test and
// drains in-flight operations before restoring the previous config.
func configureSemaphoreTest(t *testing.T, capacity int) {
	t.Helper()
	// set lifecycle config with hour refresh and millisecond persist delay
	restore := setLifecycleConfig(LifecycleConfig{
		StaleAckThreshold: 240 * time.Second,
		RefreshInterval:   time.Hour,
		SemaphoreCapacity: capacity,
		PersistRetries:    3,
		PersistRetryDelay: time.Millisecond,
	})
	// restore the previous lifecycle config on cleanup
	t.Cleanup(restore)
	// drain in-flight operations first (Cleanup is last-in first-out)
	t.Cleanup(func() { waitForSemaphoreDrain(t) })
}

// waitForSemaphoreDrain waits until no operation is in flight and no
// semaphore slot is held.
func waitForSemaphoreDrain(t *testing.T) {
	t.Helper()
	// poll until no in-flight operations and no held slots
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if inFlightCount() == 0 && len(currentSemaphore()) == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	// fail if operations have not drained within 10 seconds
	t.Errorf(
		"lifecycle goroutines did not drain: inFlight=%d, heldSlots=%d",
		inFlightCount(), len(currentSemaphore()),
	)
}

// orderRecordingInfra is a refreshable fake that records the order of
// restore, refresh, and deploy calls.
type orderRecordingInfra struct {
	*fakeRefreshableInfra

	omu   sync.Mutex
	order []string
}

// newOrderRecordingInfra returns an order-recording fake with a fresh
// refreshable infra.
func newOrderRecordingInfra() *orderRecordingInfra {
	return &orderRecordingInfra{fakeRefreshableInfra: newFakeRefreshableInfra()}
}

// record appends a method name to the recorded call order.
func (o *orderRecordingInfra) record(name string) {
	o.omu.Lock()
	defer o.omu.Unlock()
	o.order = append(o.order, name)
}

// callOrder returns a copy of the recorded method names.
func (o *orderRecordingInfra) callOrder() []string {
	o.omu.Lock()
	defer o.omu.Unlock()
	// copy the recorded names so the caller does not share the slice
	out := make([]string, len(o.order))
	copy(out, o.order)
	return out
}

// SetStackState records the call and delegates to the embedded fake.
func (o *orderRecordingInfra) SetStackState(state *datatypes.JSON) error {
	// record SetStackState
	o.record("SetStackState")
	// restore stack state
	return o.fakeRefreshableInfra.SetStackState(state)
}

// RefreshStack records the call and delegates to the embedded fake.
func (o *orderRecordingInfra) RefreshStack() error {
	// record RefreshStack
	o.record("RefreshStack")
	// refresh stack state
	return o.fakeRefreshableInfra.RefreshStack()
}

// DeployInfra records the call and deploys on the embedded fake.
func (o *orderRecordingInfra) DeployInfra() error {
	// record DeployInfra
	o.record("DeployInfra")
	// deploy infrastructure
	return o.fakeRefreshableInfra.DeployInfra()
}

// TestSemaphoreBackpressure_Requeue30 covers a full worker pool
// returning 30 instead of launching.
func TestSemaphoreBackpressure_Requeue30(t *testing.T) {
	// set semaphore capacity to 2
	configureSemaphoreTest(t, 2)
	log := newTestLogger()

	// block five deploys so acquired slots stay held
	var fis [5]*fakeInfra
	var fls [5]*fakeLifecycle
	for i := range fis {
		fis[i] = newFakeInfra()
		fis[i].setDeploy(infraBlock, nil)
		fls[i] = newFakeLifecycle()
		fls[i].setInfra(fis[i])
	}
	// release blocked deploys after the test
	t.Cleanup(func() {
		for _, fi := range fis {
			fi.releaseDeploy()
		}
	})

	// launch five creates against the capacity-2 pool
	var requeues [5]int64
	for i := range fls {
		requeue, err := HandleInfraCreate(fls[i], log)
		require.NoError(t, err)
		requeues[i] = requeue
	}

	// first two acquire a slot and requeue 120, the rest requeue 30
	require.Equal(t, [5]int64{120, 120, 30, 30, 30}, requeues)

	// wait until the two launched creates have entered deploy
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if fis[0].deployCallCount() == 1 && fis[1].deployCallCount() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	require.Equal(t, 1, fis[0].deployCallCount())
	require.Equal(t, 1, fis[1].deployCallCount())

	// check the three back-pressured creates did not deploy
	for i := 2; i < 5; i++ {
		require.Equal(t, 0, fis[i].deployCallCount())
	}

	// release the held slots and wait for drain
	fis[0].releaseDeploy()
	fis[1].releaseDeploy()
	waitForSemaphoreDrain(t)

	// a later create acquires a slot and requeues 120
	fl := newFakeLifecycle()
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)
}

// TestSemaphoreReleaseOnPanic covers a create panic releasing its semaphore
// slot.
func TestSemaphoreReleaseOnPanic(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// panic on deploy
	fi := newFakeInfra()
	fi.setDeploy(infraPanic, nil)
	fl := newFakeLifecycle()
	fl.setInfra(fi)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for the panicked create to drain
	waitForSemaphoreDrain(t)

	// check deploy ran and creation was marked failed
	require.Equal(t, 1, fi.deployCallCount())
	require.Equal(t, 1, fl.callCount("SetCreationFailed"))

	// a later create on the one-slot pool acquires the released slot
	fl2 := newFakeLifecycle()
	requeue, err = HandleInfraCreate(fl2, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)
}

// TestSemaphoreReleaseOnPanic_Delete covers a delete panic releasing its
// semaphore slot without marking creation failed.
func TestSemaphoreReleaseOnPanic_Delete(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// panic on destroy
	fi := newFakeInfra()
	fi.setDestroy(infraPanic, nil)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(time.Now().UTC()),
	})
	fl.setInfra(fi)

	// launch delete
	requeue, err := HandleInfraDelete(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(300), requeue)

	// wait for the panicked delete to drain
	waitForSemaphoreDrain(t)

	// check destroy ran and no failure or state was persisted
	require.Equal(t, 1, fi.destroyCallCount())
	require.Equal(t, 0, fl.callCount("SetCreationFailed"))
	require.Equal(t, 0, fl.callCount("SaveState"))

	// a later delete on the one-slot pool acquires the released slot
	fl2 := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(time.Now().UTC()),
	})
	requeue, err = HandleInfraDelete(fl2, log)
	require.NoError(t, err)
	require.Equal(t, int64(300), requeue)
}

// TestExecuteInfraCreate_RestoreThenRefreshThenDeploy covers create restoring
// inventory, refreshing, then deploying in that order.
func TestExecuteInfraCreate_RestoreThenRefreshThenDeploy(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// give the create existing inventory on a refreshable fake
	inventory := validStackState()
	oi := newOrderRecordingInfra()
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		ResourceInventory: inventory,
	})
	fl.setInfra(oi)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for create to finish
	waitForSemaphoreDrain(t)

	// check restore, refresh, then deploy ran in that order
	require.Equal(
		t,
		[]string{"SetStackState", "RefreshStack", "DeployInfra"},
		oi.callOrder(),
	)
	require.Equal(t, 1, oi.refreshCallCount())
	require.NotNil(t, oi.lastRestoredState())
	require.JSONEq(t, string(*inventory), string(*oi.lastRestoredState()))

	// check create outputs were saved and the create notification published
	require.Equal(t, 1, fl.callCount("SaveCreateOutputs"))
	require.Equal(t, 1, fl.callCount("PublishCreateNotification"))
}

// TestExecuteInfraCreate_NonStreamable_NoWatcher covers a non-streamable
// create saving outputs without streaming state.
func TestExecuteInfraCreate_NonStreamable_NoWatcher(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// use a non-streamable fake with no existing inventory
	fi := newFakeInfra()
	fl := newFakeLifecycle()
	fl.setInfra(fi)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for create to finish
	waitForSemaphoreDrain(t)

	// check deploy ran and outputs were saved and published
	require.Equal(t, 1, fi.deployCallCount())
	require.Equal(t, 1, fl.callCount("SaveCreateOutputs"))
	require.NotNil(t, fl.createOutputs())
	require.JSONEq(t, string(*validStackState()), string(*fl.createOutputs()))
	require.Equal(t, 1, fl.callCount("PublishCreateNotification"))

	// check stack state was not restored and state was not streamed
	require.Equal(t, 0, fi.setStackStateCallCount())
	require.Equal(t, 0, fl.callCount("SaveState"))
}

// TestExecuteInfraCreate_DeployError_CapturesStateAndPersistsFailure covers a
// failed deploy saving stack state and marking creation failed.
func TestExecuteInfraCreate_DeployError_CapturesStateAndPersistsFailure(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// fail deploy with a given error
	errDeploy := errors.New("deploy exploded")
	fi := newFakeInfra()
	fi.setDeploy(infraError, errDeploy)
	fl := newFakeLifecycle()
	fl.setInfra(fi)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for create to finish
	waitForSemaphoreDrain(t)

	// check stack state was captured once
	require.Equal(t, 1, fi.getStackStateCallCount())
	saved := fl.savedStateHistory()
	require.Len(t, saved, 1)
	require.JSONEq(t, string(*validStackState()), string(*saved[0]))

	// check creation was marked failed and outputs were not saved
	require.Equal(t, 1, fl.callCount("SetCreationFailed"))
	require.Equal(t, 0, fl.callCount("SaveCreateOutputs"))
	require.Equal(t, 0, fl.callCount("PublishCreateNotification"))
}

// TestExecuteInfraCreate_VerifyStateFails_PersistsFailure covers a post-deploy
// empty resource list marking creation failed.
func TestExecuteInfraCreate_VerifyStateFails_PersistsFailure(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// return stack state with an empty resource list
	fi := newFakeInfra()
	fi.setGetStackState(jsonPtr(`{"deployment":{"resources":[]}}`), nil)
	fl := newFakeLifecycle()
	fl.setInfra(fi)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for create to finish
	waitForSemaphoreDrain(t)

	// check deploy ran and creation was marked failed
	require.Equal(t, 1, fi.deployCallCount())
	require.Equal(t, 1, fi.getStackStateCallCount())
	require.Equal(t, 1, fl.callCount("SetCreationFailed"))

	// check outputs were not saved and no create notification was published
	require.Equal(t, 0, fl.callCount("SaveCreateOutputs"))
	require.Equal(t, 0, fl.callCount("PublishCreateNotification"))
}

// TestExecuteInfraDelete_InvalidExistingStateJSON_SkipsRestore covers a
// truncated inventory skipping restore and still destroying.
func TestExecuteInfraDelete_InvalidExistingStateJSON_SkipsRestore(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// schedule deletion with truncated inventory JSON
	fi := newFakeInfra()
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(time.Now().UTC()),
		ResourceInventory: jsonPtr(`{"deployment":{"resources":[`),
	})
	fl.setInfra(fi)

	// launch delete
	requeue, err := HandleInfraDelete(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(300), requeue)

	// wait for delete to finish
	waitForSemaphoreDrain(t)

	// check restore was skipped and destroy still ran
	require.Equal(t, 0, fi.setStackStateCallCount())
	require.Equal(t, 1, fi.destroyCallCount())

	// check inventory was cleared and the delete notification published
	require.Equal(t, 1, fl.callCount("ClearInventory"))
	require.Equal(t, 1, fl.callCount("PublishDeleteNotification"))
}

// TestExecuteInfraDelete_DestroyError_CapturesRemainingState covers a failed
// destroy saving remaining stack state without clearing inventory.
func TestExecuteInfraDelete_DestroyError_CapturesRemainingState(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// fail destroy with a given error
	errDestroy := errors.New("destroy exploded")
	fi := newFakeInfra()
	fi.setDestroy(infraError, errDestroy)
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(time.Now().UTC()),
	})
	fl.setInfra(fi)

	// launch delete
	requeue, err := HandleInfraDelete(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(300), requeue)

	// wait for delete to finish
	waitForSemaphoreDrain(t)

	// check remaining stack state was captured once
	require.Equal(t, 1, fi.destroyCallCount())
	require.Equal(t, 1, fi.getStackStateCallCount())
	saved := fl.savedStateHistory()
	require.Len(t, saved, 1)
	require.JSONEq(t, string(*validStackState()), string(*saved[0]))

	// check inventory was not cleared and no delete notification was published
	require.Equal(t, 0, fl.callCount("ClearInventory"))
	require.Equal(t, 0, fl.callCount("PublishDeleteNotification"))
}

// TestExecuteInfraCreate_RestoreError_PersistsFailureWithoutDeploying
// covers a failed restore marking creation failed without deploying.
func TestExecuteInfraCreate_RestoreError_PersistsFailureWithoutDeploying(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// fail restore of existing inventory
	fi := newFakeInfra()
	fi.setSetStackStateErr(errors.New("state blob is corrupt"))
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		ResourceInventory: validStackState(),
	})
	fl.setInfra(fi)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for create to finish
	waitForSemaphoreDrain(t)

	// check restore ran once and deploy did not
	require.Equal(t, 1, fi.setStackStateCallCount(), "the restore is attempted once")
	require.Equal(t, 0, fi.deployCallCount(), "a failed restore must not deploy")
	require.Equal(t, 1, fl.callCount("SetCreationFailed"))
	require.Equal(t, 0, fl.callCount("SaveCreateOutputs"))
	require.Equal(t, 0, fl.callCount("PublishCreateNotification"))
}

// TestExecuteInfraCreate_RefreshError_PersistsFailureWithoutDeploying
// covers a failed refresh marking creation failed without deploying.
func TestExecuteInfraCreate_RefreshError_PersistsFailureWithoutDeploying(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// fail refresh after restoring existing inventory
	ri := newFakeRefreshableInfra()
	ri.setRefreshErr(errors.New("refresh could not reach the cloud provider"))
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		ResourceInventory: validStackState(),
	})
	fl.setInfra(ri)

	// launch create
	requeue, err := HandleInfraCreate(fl, log)
	require.NoError(t, err)
	require.Equal(t, int64(120), requeue)

	// wait for create to finish
	waitForSemaphoreDrain(t)

	// check refresh ran once and deploy did not
	require.Equal(t, 1, ri.refreshCallCount())
	require.Equal(t, 0, ri.deployCallCount(), "a failed refresh must not deploy on create")
	require.Equal(t, 1, fl.callCount("SetCreationFailed"))
	require.Equal(t, 0, fl.callCount("SaveCreateOutputs"))
}

// TestExecuteInfraDelete_RefreshError_StillDestroys covers a failed refresh
// still destroying.
func TestExecuteInfraDelete_RefreshError_StillDestroys(t *testing.T) {
	// set semaphore capacity to 1
	configureSemaphoreTest(t, 1)
	log := newTestLogger()

	// fail refresh after restoring existing inventory
	ri := newFakeRefreshableInfra()
	ri.setRefreshErr(errors.New("refresh could not reach the cloud provider"))
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Hour)),
		ResourceInventory: validStackState(),
	})
	fl.setInfra(ri)

	// launch delete
	_, err := HandleInfraDelete(fl, log)
	require.NoError(t, err)

	// wait for delete to finish
	waitForSemaphoreDrain(t)

	// check refresh ran once and destroy still ran
	require.Equal(t, 1, ri.refreshCallCount())
	require.Equal(t, 1, ri.destroyCallCount(), "a failed refresh must not block the destroy")
}
