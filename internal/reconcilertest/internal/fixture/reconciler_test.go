package fixture

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	api "github.com/threeport/threeport/internal/reconcilertest/pkg/api/v0"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	tpapi_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	tpapi "github.com/threeport/threeport/pkg/api/v0"
	tpclient_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Tests in this file run generated reconcilers against an in-process NATS
// server and API stub. A skipped operation never records a spy call, so a
// wait for idle watches the lock key rather than the spy.

const (
	fixtureObjectID = 42
	fixtureStream   = "fixtureStream"
	fixtureBucket   = "fixtureLock"

	instanceSubject    = "reconcilerTestInstance.notify"
	instanceReconciler = "ReconcilerTestInstanceReconciler"

	volatileSubject    = "reconcilerTestVolatileInstance.notify"
	volatileReconciler = "ReconcilerTestVolatileInstanceReconciler"
)

// harness is an in-process NATS server, API stub, and pair of reconcilers a
// test publishes into.
type harness struct {
	t   *testing.T
	js  nats.JetStreamContext
	spy *Spy
	api *machinetest.APIStub
	// instanceRecorder captures events from the fetching-object reconciler
	instanceRecorder *machinetest.FakeRecorder

	// The mutex covering obj and volatile
	objMu    sync.Mutex
	obj      *api.ReconcilerTestInstance
	volatile *api.ReconcilerTestVolatileInstance

	// The HTTP status the fetching object's GET handler returns
	apiStatus atomic.Int64

	// patchCount counts PATCH requests against the fetching object, i.e.
	// the "mark reconciled" update issued after the operation switch
	patchCount atomic.Int64
	// lastPatch is the body of the latest PATCH against the fetching object
	lastPatch []byte

	// failures is the reconcilers' record of when operations began failing.
	// Its clock is the harness's, which a test advances by hand to pass the
	// retry bound without waiting out an hour.
	failures *controller.FailureTracker
	clockMu  sync.Mutex
	clock    time.Time
}

// now is the harness's clock.
func (h *harness) now() time.Time {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	return h.clock
}

// advanceClock moves the harness's clock forward.
func (h *harness) advanceClock(d time.Duration) {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	h.clock = h.clock.Add(d)
}

// startNatsServer starts an in-process JetStream server on a free loopback port.
func startNatsServer(t *testing.T) *natsserver.Server {
	t.Helper()

	// Port -1 is nats-server RANDOM_PORT; NoSigs leaves process signals alone
	server, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)

	// wait until the client port is accepting
	go server.Start()
	require.True(t, server.ReadyForConnections(10*time.Second), "nats server did not start")

	return server
}

// newHarness starts NATS, the API stub, and both reconcilers, and registers cleanup.
func newHarness(t *testing.T) *harness {
	t.Helper()

	// start nats
	server := startNatsServer(t)
	// server shutdown runs last; PullMessage calls os.Exit on a missing stream
	t.Cleanup(server.Shutdown)

	// connect and open jetstream
	conn, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)

	js, err := conn.JetStream()
	require.NoError(t, err)

	// add a stream covering both reconciler subjects
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     fixtureStream,
		Subjects: []string{instanceSubject, volatileSubject},
	})
	require.NoError(t, err)

	// create the lock bucket
	keyValue, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: fixtureBucket})
	require.NoError(t, err)

	// build the fixture objects
	h := &harness{
		t:                t,
		js:               js,
		spy:              NewSpy(),
		api:              machinetest.NewAPIStub(t),
		instanceRecorder: machinetest.NewFakeRecorder(),
		obj: &api.ReconcilerTestInstance{
			Common:   tpapi.Common{ID: util.Ptr(uint(fixtureObjectID))},
			Instance: tpapi.Instance{Name: util.Ptr("fixture")},
		},
		volatile: &api.ReconcilerTestVolatileInstance{
			Common:   tpapi.Common{ID: util.Ptr(uint(fixtureObjectID))},
			Instance: tpapi.Instance{Name: util.Ptr("volatile-fixture")},
		},
	}
	h.clock = time.Now()
	h.failures = &controller.FailureTracker{Now: h.now}
	h.apiStatus.Store(http.StatusOK)
	// install the spy
	t.Cleanup(InstallSpy(h.spy))

	// serve the latest-object fetch and the patch after a non-delete
	h.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", api.PathReconcilerTestInstances, fixtureObjectID),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPatch {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				h.objMu.Lock()
				h.lastPatch = body
				h.objMu.Unlock()
				h.patchCount.Add(1)
			}
			status := int(h.apiStatus.Load())
			if status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			h.objMu.Lock()
			defer h.objMu.Unlock()
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*h.obj})
		},
	)

	// serve the post-success patch; this reconciler never fetches
	h.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", api.PathReconcilerTestVolatileInstances, fixtureObjectID),
		func(w http.ResponseWriter, r *http.Request) {
			h.objMu.Lock()
			defer h.objMu.Unlock()
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*h.volatile})
		},
	)

	// start both reconcilers
	h.startReconciler(conn, js, keyValue, instanceReconciler, instanceSubject, ReconcilerTestInstanceReconciler)
	h.startReconciler(conn, js, keyValue, volatileReconciler, volatileSubject, ReconcilerTestVolatileInstanceReconciler)

	// close first so a parked Fetch returns
	t.Cleanup(conn.Close)

	return h
}

// startReconciler runs reconcile in a goroutine against a durable pull subscription.
func (h *harness) startReconciler(
	conn *nats.Conn,
	js nats.JetStreamContext,
	keyValue nats.KeyValue,
	name string,
	subject string,
	reconcile func(*controller.Reconciler),
) {
	h.t.Helper()

	// bind a durable consumer to the fixture stream
	sub, err := js.PullSubscribe(subject, name+"Consumer", nats.BindStream(fixtureStream))
	require.NoError(h.t, err)

	ready := &atomic.Bool{}
	ready.Store(true)
	log := logr.Discard()
	shutdown := make(chan bool, 1)
	var shutdownWait sync.WaitGroup

	// run the reconcile loop
	go reconcile(&controller.Reconciler{
		Name:             name,
		APIServer:        h.api.Addr,
		APIClient:        h.api.Client,
		JetStreamContext: js,
		Sub:              sub,
		KeyValue:         keyValue,
		ControllerID:     uuid.New(),
		Ready:            ready,
		Log:              &log,
		Shutdown:         shutdown,
		ShutdownWait:     &shutdownWait,
		EventsRecorder:   recorderFor(h, name),
		Failures:         h.failures,
	})

	// signal shutdown and wait for the loop to return
	h.t.Cleanup(func() {
		shutdown <- true
		shutdownWait.Wait()
	})
}

// recorderFor returns the shared instance recorder for the fetching-object
// reconciler and a fresh recorder for the persist-false reconciler.
func recorderFor(h *harness, name string) *machinetest.FakeRecorder {
	if name == instanceReconciler {
		return h.instanceRecorder
	}
	return machinetest.NewFakeRecorder()
}

// scheduleDeletion sets DeletionScheduled on both fixture objects. A delete
// against the API stamps that field before it publishes.
func (h *harness) scheduleDeletion() {
	h.objMu.Lock()
	defer h.objMu.Unlock()
	h.obj.DeletionScheduled = util.Ptr(time.Now().UTC())
	h.volatile.DeletionScheduled = util.Ptr(time.Now().UTC())
}

// publish sends operation for the fetching object and waits until that reconciler is idle.
func (h *harness) publish(operation notifications.NotificationOperation) {
	h.t.Helper()
	h.publishCreatedAt(operation, time.Now().Unix())
}

// publishCreatedAt is publish for a notification that carries the given
// creation time, in Unix seconds - how a notification that has been failing for
// a while looks to the reconciler.
func (h *harness) publishCreatedAt(operation notifications.NotificationOperation, creationTime int64) {
	h.t.Helper()

	// build the notification payload
	h.objMu.Lock()
	payload, err := h.obj.NotificationPayload(operation, false, creationTime)
	h.objMu.Unlock()
	require.NoError(h.t, err)

	// snapshot the delivery count so settle can tell this message was
	// actually pulled off the queue, not just that no lock happens to be held yet
	before := h.delivered(instanceReconciler)

	// publish it
	_, err = h.js.Publish(instanceSubject, *payload)
	require.NoError(h.t, err)

	// wait until the loop is idle
	h.settle(instanceReconciler, before)
}

// publishVolatile sends operation for the persist-false object and waits until that reconciler is idle.
func (h *harness) publishVolatile(operation notifications.NotificationOperation) {
	h.t.Helper()

	// build the notification payload
	h.objMu.Lock()
	payload, err := h.volatile.NotificationPayload(operation, false, time.Now().Unix())
	h.objMu.Unlock()
	require.NoError(h.t, err)

	// snapshot the delivery count so settle can tell this message was
	// actually pulled off the queue, not just that no lock happens to be held yet
	before := h.delivered(volatileReconciler)

	// publish it
	_, err = h.js.Publish(volatileSubject, *payload)
	require.NoError(h.t, err)

	// wait until the loop is idle
	h.settle(volatileReconciler, before)
}

// settle waits until reconcilerName has pulled a message beyond the
// before count and holds no lock on the fixture object. The lock alone
// cannot prove the loop is done: it is absent both before the message is
// delivered and after a skipped create/update releases it (with no I/O in
// between, so a poll can miss it being held at all). Requiring the
// delivery count to advance first rules out the "not started yet" case,
// so "no lock" afterward can only mean this pass finished.
func (h *harness) settle(reconcilerName string, before uint64) {
	h.t.Helper()

	deadline := time.Now().Add(15 * time.Second)

	// wait until this message has actually been pulled off the queue
	for h.delivered(reconcilerName) <= before {
		if !time.Now().Before(deadline) {
			h.t.Fatalf("%s never pulled the published message within 15s", reconcilerName)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// poll until the lock is gone
	for time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		if h.locked(reconcilerName) {
			continue
		}
		// lock gone: wait once more so a just-starting pass can appear
		time.Sleep(75 * time.Millisecond)
		if !h.locked(reconcilerName) {
			return
		}
	}
	h.t.Fatalf("%s did not finish within 15s", reconcilerName)
}

// delivered returns the number of times reconcilerName's consumer has ever
// had a message delivered to it, a monotonically increasing count that
// advances on every fetch, including redeliveries.
func (h *harness) delivered(reconcilerName string) uint64 {
	h.t.Helper()

	info, err := h.js.ConsumerInfo(fixtureStream, reconcilerName+"Consumer")
	require.NoError(h.t, err)
	return info.Delivered.Consumer
}

// outstanding returns the number of messages on reconcilerName's consumer that
// have not been acknowledged: waiting to be delivered, delivered and unanswered,
// or negatively acknowledged and waiting out a redelivery delay. It returns -1
// when the consumer cannot be read. It is polled from goroutines that can still
// be running as the test ends and the connection closes, so it reports that
// rather than failing the test from a goroutine that does not own it.
func (h *harness) outstanding(reconcilerName string) int {
	info, err := h.js.ConsumerInfo(fixtureStream, reconcilerName+"Consumer")
	if err != nil {
		return -1
	}
	return info.NumAckPending + int(info.NumPending)
}

// locked reports whether reconcilerName currently holds the fixture object's lock.
func (h *harness) locked(reconcilerName string) bool {
	h.t.Helper()

	// read the lock key for this reconciler and object
	keyValue, err := h.js.KeyValue(fixtureBucket)
	require.NoError(h.t, err)

	entry, err := keyValue.Get(fmt.Sprintf("%s.%d", reconcilerName, fixtureObjectID))
	return err == nil && entry != nil
}

// TestReconcilerDispatchesEachOperation covers create, update, and delete each
// reaching its handler.
func TestReconcilerDispatchesEachOperation(t *testing.T) {
	for _, tc := range []struct {
		operation notifications.NotificationOperation
		want      string
	}{
		{notifications.NotificationOperationCreated, "create"},
		{notifications.NotificationOperationUpdated, "update"},
		{notifications.NotificationOperationDeleted, "delete"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			h := newHarness(t)
			h.publish(tc.operation)
			assert.Equal(t, []string{tc.want}, h.spy.Calls())
		})
	}
}

// TestUpdateSkippedWhenDeletionScheduled covers an update that must not run
// after DeletionScheduled is set, and a delete that still must.
func TestUpdateSkippedWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	// fail the update handler if it runs, so a missed skip would requeue forever
	h.spy.SetResult("update", Result{RequeueDelay: 1, Err: fmt.Errorf("update handler failed")})
	h.scheduleDeletion()

	// update must not dispatch
	h.publish(notifications.NotificationOperationUpdated)

	assert.Empty(t, h.spy.Calls(), "update ran on an object already scheduled for deletion")
	assert.Zero(t, h.patchCount.Load(), "skipped update still marked the object reconciled")
	assert.NotContains(t, h.instanceRecorder.GetReasons(), event.ReasonUpdateSuccessful,
		"skipped update still recorded a successful-reconciliation event")

	// delete still must
	h.publish(notifications.NotificationOperationDeleted)

	assert.Contains(t, h.spy.Calls(), "delete", "delete never reached its handler")
}

// TestCreateSkippedWhenDeletionScheduled covers a create that must not run
// after DeletionScheduled is set.
func TestCreateSkippedWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	h.scheduleDeletion()

	// create must not dispatch
	h.publish(notifications.NotificationOperationCreated)

	assert.Empty(t, h.spy.Calls(), "create ran on an object already scheduled for deletion")
	assert.Zero(t, h.patchCount.Load(), "skipped create still marked the object reconciled")
	assert.NotContains(t, h.instanceRecorder.GetReasons(), event.ReasonCreateSuccessful,
		"skipped create still recorded a successful-reconciliation event")
}

// TestDeleteRunsWhenDeletionScheduled covers a delete that still runs when
// DeletionScheduled is set. The API stamps that field before it publishes delete.
func TestDeleteRunsWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	h.scheduleDeletion()

	h.publish(notifications.NotificationOperationDeleted)

	assert.Equal(t, []string{"delete"}, h.spy.Calls())
}

// TestUpdateDoesNotRestampReconciled covers an update whose fetched object is already reconciled.
// The handler still runs, and the reconciler must not patch Reconciled again.
func TestUpdateDoesNotRestampReconciled(t *testing.T) {
	h := newHarness(t)

	// the fetched object is already reconciled
	h.objMu.Lock()
	h.obj.Reconciled = util.Ptr(true)
	h.objMu.Unlock()

	// the update still reaches the handler
	h.publish(notifications.NotificationOperationUpdated)

	assert.Equal(t, []string{"update"}, h.spy.Calls())
	assert.Zero(t, h.patchCount.Load(), "already reconciled object was patched again")
}

// TestUpdateStampsReconciledWhenNotTrue covers the first stamp.
// An unset or false Reconciled still gets one patch that sets it true.
func TestUpdateStampsReconciledWhenNotTrue(t *testing.T) {
	cases := []struct {
		name       string
		reconciled *bool
	}{
		{name: "nil"},
		{name: "false", reconciled: util.Ptr(false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)

			// the fetched object is not yet reconciled
			h.objMu.Lock()
			h.obj.Reconciled = tc.reconciled
			h.objMu.Unlock()

			// the update reaches the handler and stamps Reconciled
			h.publish(notifications.NotificationOperationUpdated)

			assert.Equal(t, []string{"update"}, h.spy.Calls())
			assert.Equal(t, int64(1), h.patchCount.Load())
			h.objMu.Lock()
			body := append([]byte(nil), h.lastPatch...)
			before := *h.obj
			h.objMu.Unlock()
			var patched api.ReconcilerTestInstance
			require.NoError(t, json.Unmarshal(body, &patched))
			require.NotNil(t, patched.Reconciled)
			assert.True(t, *patched.Reconciled)
			after := before
			after.Reconciled = patched.Reconciled
			notify, changed := tpapi_lib.ObjectUpdateNotifiable(before, after)
			assert.False(t, notify, "the stamp the reconciler wrote must not schedule another reconcile")
			assert.Empty(t, changed)

			// a body that also changes another field still must not notify
			withStatus := after
			withStatus.Status = util.Ptr("changed")
			notify, changed = tpapi_lib.ObjectUpdateNotifiable(before, withStatus)
			assert.False(t, notify, "Reconciled becoming true must not schedule a reconcile")
			assert.Equal(t, []string{"Status"}, changed)
		})
	}
}

// TestHaltsWhenObjectNoLongerExists covers an update whose object GET fails
// before the handler runs.
func TestHaltsWhenObjectNoLongerExists(t *testing.T) {
	h := newHarness(t)
	// answer GET with not found
	h.apiStatus.Store(http.StatusNotFound)

	h.publish(notifications.NotificationOperationUpdated)

	assert.Empty(t, h.spy.Calls(), "dispatched an operation for an object the API says is gone")
}

// TestVolatileObjectDispatchesWithoutFetch covers an update on the persist-false
// object dispatching while the fetching object's GET fails.
func TestVolatileObjectDispatchesWithoutFetch(t *testing.T) {
	h := newHarness(t)
	h.apiStatus.Store(http.StatusNotFound)

	// fetching object's GET fails
	h.publish(notifications.NotificationOperationUpdated)
	require.Empty(t, h.spy.Calls(), "the fetching object should have halted on 404")

	// persist-false object still dispatches
	h.publishVolatile(notifications.NotificationOperationUpdated)

	assert.Equal(t, []string{"volatile-update"}, h.spy.Calls())
}

// TestCreateInProgressRecordedWhenHandlerFails covers CreateInProgress being
// recorded before the custom create handler returns an error.
func TestCreateInProgressRecordedWhenHandlerFails(t *testing.T) {
	h := newHarness(t)
	h.spy.SetResult("create", Result{Err: fmt.Errorf("create handler failed")})

	h.publish(notifications.NotificationOperationCreated)

	assert.Equal(t, []string{"create"}, h.spy.Calls())
	assert.Contains(t, h.instanceRecorder.GetReasons(), event.ReasonCreateInProgress)
	assert.NotContains(t, h.instanceRecorder.GetReasons(), event.ReasonCreateSuccessful)
}

// TestDeleteInProgressRecordedOnceWhenDeleteConflicts covers a blocked or
// already-underway delete recording DeleteInProgress once, not twice.
func TestDeleteInProgressRecordedOnceWhenDeleteConflicts(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"in progress", tpclient_lib.ErrDeleteInProgress},
		{"blocked", tpclient_lib.ErrDeleteBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.spy.SetResult("delete", Result{Err: tc.err})

			h.publish(notifications.NotificationOperationDeleted)

			assert.Equal(t, []string{"delete"}, h.spy.Calls())
			count := 0
			for _, reason := range h.instanceRecorder.GetReasons() {
				if reason == event.ReasonDeleteInProgress {
					count++
				}
			}
			assert.Equal(t, 1, count)
			assert.NotContains(t, h.instanceRecorder.GetReasons(), event.ReasonDeleteFailed)
		})
	}
}

// failUntilGivingUp publishes an operation that fails, lets the failure go on
// for longer than the retry bound, and waits for the reconciler to stop
// retrying it. It is the harness's equivalent of an operation failing for an
// hour.
func failUntilGivingUp(t *testing.T, h *harness, operation notifications.NotificationOperation) {
	t.Helper()

	h.publish(operation)
	require.Len(t, h.spy.Calls(), 1)
	assertDidNotGiveUp(t, h)

	h.advanceClock(controller.DefaultMaxReconcileDuration + time.Minute)

	// the next attempt is the redelivery, a second or so later
	require.Eventually(t, func() bool { return h.outstanding(instanceReconciler) == 0 },
		10*time.Second, 50*time.Millisecond, "the notification must be acknowledged, not requeued")
}

// TestFailingOperationGivesUpAfterRetryBound covers an operation that has been
// failing for longer than the retry bound being dropped rather than requeued.
// Without the bound a failure that cannot resolve itself - a permission that is
// never granted - is redelivered for as long as the object exists.
func TestFailingOperationGivesUpAfterRetryBound(t *testing.T) {
	for _, op := range []struct {
		name         string
		operation    notifications.NotificationOperation
		handler      string
		failedReason string
	}{
		{"create", notifications.NotificationOperationCreated, "create", event.ReasonCreateFailed},
		{"update", notifications.NotificationOperationUpdated, "update", event.ReasonUpdateFailed},
		{"delete", notifications.NotificationOperationDeleted, "delete", event.ReasonDeleteFailed},
	} {
		t.Run(op.name, func(t *testing.T) {
			h := newHarness(t)
			h.spy.SetResult(op.handler, Result{Err: fmt.Errorf("%s handler failed", op.handler)})

			failUntilGivingUp(t, h, op.operation)

			assert.False(t, h.locked(instanceReconciler), "giving up must release the lock")
			assertGaveUp(t, h, op.failedReason)
		})
	}
}

// TestStaleNotificationIsRetriedAfterItsFirstFailure covers a notification that
// waited in the queue for longer than the retry bound - a controller that was
// down, a backlog - being retried when it first fails. The bound is on time
// spent failing, not on the notification's age, and an operation that has
// failed once has not been failing for an hour.
func TestStaleNotificationIsRetriedAfterItsFirstFailure(t *testing.T) {
	h := newHarness(t)
	h.spy.SetResult("update", Result{Err: fmt.Errorf("update handler failed")})

	h.publishCreatedAt(
		notifications.NotificationOperationUpdated,
		time.Now().Add(-controller.DefaultMaxReconcileDuration-time.Hour).Unix(),
	)

	assert.Equal(t, []string{"update"}, h.spy.Calls())
	assert.Never(t, func() bool { return h.outstanding(instanceReconciler) == 0 },
		time.Second, 50*time.Millisecond, "the first failure of an old notification must be requeued")
	assertDidNotGiveUp(t, h)
}

// TestSuccessEndsTheFailureWindow covers a success starting a later failure on
// a new window. Failures an hour apart with a success between them are not one
// hour of failing.
func TestSuccessEndsTheFailureWindow(t *testing.T) {
	h := newHarness(t)

	h.spy.SetResult("update", Result{Err: fmt.Errorf("update handler failed")})
	h.publish(notifications.NotificationOperationUpdated)
	assertDidNotGiveUp(t, h)

	h.advanceClock(controller.DefaultMaxReconcileDuration - time.Minute)
	h.spy.SetResult("update", Result{})
	require.Eventually(t, func() bool { return h.outstanding(instanceReconciler) == 0 },
		10*time.Second, 50*time.Millisecond, "the retry succeeds")

	h.advanceClock(2 * time.Minute)
	h.spy.SetResult("update", Result{Err: fmt.Errorf("update handler failed again")})
	h.publish(notifications.NotificationOperationUpdated)

	assertDidNotGiveUp(t, h)
	assert.Never(t, func() bool { return h.outstanding(instanceReconciler) == 0 },
		time.Second, 50*time.Millisecond, "a failure after a success is a new episode, so it is requeued")
}

// TestFailingOperationIsRequeuedWithinRetryBound is the other side of the
// bound: a recent failure is retried, so the bound cannot pass by dropping every
// failure.
func TestFailingOperationIsRequeuedWithinRetryBound(t *testing.T) {
	h := newHarness(t)
	h.spy.SetResult("update", Result{Err: fmt.Errorf("update handler failed")})

	h.publish(notifications.NotificationOperationUpdated)

	require.Eventually(t, func() bool { return len(h.spy.Calls()) >= 2 }, 10*time.Second, 100*time.Millisecond,
		"a failure inside the bound is redelivered")
}

// gaveUpEvent returns the final event the instance reconciler recorded for a
// failure it stopped retrying, or nil if it recorded none.
func gaveUpEvent(h *harness) *tpapi.Event {
	var final *tpapi.Event
	for _, recorded := range h.instanceRecorder.GetEvents() {
		if recorded.Event != nil && recorded.Event.Note != nil && strings.Contains(*recorded.Event.Note, "giving up") {
			final = recorded.Event
		}
	}
	return final
}

// assertDidNotGiveUp checks that no final event has been recorded.
func assertDidNotGiveUp(t *testing.T, h *harness) {
	t.Helper()
	assert.Nil(t, gaveUpEvent(h), "the reconciler stopped retrying too soon")
}

// assertGaveUp checks that the final event for a failure the reconciler has
// stopped retrying was recorded, and says so.
func assertGaveUp(t *testing.T, h *harness, reason string) {
	t.Helper()

	final := gaveUpEvent(h)
	require.NotNil(t, final, "giving up must leave an event that says retries have stopped")
	assert.Equal(t, reason, *final.Reason)
	assert.Equal(t, event.TypeWarning, *final.Type)
}

// TestGiveUpEventRecordedWhenErrorCarriesAnEvent covers the final event
// surviving an error that carries an event of its own. The events recorded on
// each earlier attempt defer to that event, so a final one routed the same way
// would be replaced by it - an exhausted failure recording the same event as
// every retry, with nothing to say retries had stopped.
func TestGiveUpEventRecordedWhenErrorCarriesAnEvent(t *testing.T) {
	h := newHarness(t)
	h.spy.SetResult("update", Result{Err: &tp_errors.ErrWithEvent{
		Message: "ssh connection failed",
		Event: tpapi.Event{
			Reason: util.Ptr("SSHConnectFailed"),
			Note:   util.Ptr("could not connect"),
			Type:   util.Ptr(event.TypeWarning),
		},
	}})

	failUntilGivingUp(t, h, notifications.NotificationOperationUpdated)

	assertGaveUp(t, h, event.ReasonUpdateFailed)
	for _, reason := range h.instanceRecorder.GetReasons() {
		assert.NotEqual(t, "SSHConnectFailed", reason, "the final event is not the error's own")
	}
}
