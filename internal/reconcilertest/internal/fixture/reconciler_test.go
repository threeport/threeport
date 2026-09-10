package fixture

import (
	"fmt"
	"net/http"
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
	tpapi "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
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
	h.apiStatus.Store(http.StatusOK)
	// install the spy
	t.Cleanup(InstallSpy(h.spy))

	// serve the latest-object fetch and the patch after a non-delete
	h.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", api.PathReconcilerTestInstances, fixtureObjectID),
		func(w http.ResponseWriter, r *http.Request) {
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

	// build the notification payload
	h.objMu.Lock()
	payload, err := h.obj.NotificationPayload(operation, false, time.Now().Unix())
	h.objMu.Unlock()
	require.NoError(h.t, err)

	// publish it
	_, err = h.js.Publish(instanceSubject, *payload)
	require.NoError(h.t, err)

	// wait until the loop is idle
	h.settle(instanceReconciler)
}

// publishVolatile sends operation for the persist-false object and waits until that reconciler is idle.
func (h *harness) publishVolatile(operation notifications.NotificationOperation) {
	h.t.Helper()

	// build the notification payload
	h.objMu.Lock()
	payload, err := h.volatile.NotificationPayload(operation, false, time.Now().Unix())
	h.objMu.Unlock()
	require.NoError(h.t, err)

	// publish it
	_, err = h.js.Publish(volatileSubject, *payload)
	require.NoError(h.t, err)

	// wait until the loop is idle
	h.settle(volatileReconciler)
}

// settle waits until reconcilerName holds no lock on the fixture object.
// A skipped create or update never records a spy call, so the lock is the
// completion signal.
func (h *harness) settle(reconcilerName string) {
	h.t.Helper()

	// poll until the lock is gone
	deadline := time.Now().Add(15 * time.Second)
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
}

// TestDeleteRunsWhenDeletionScheduled covers a delete that still runs when
// DeletionScheduled is set. The API stamps that field before it publishes delete.
func TestDeleteRunsWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	h.scheduleDeletion()

	h.publish(notifications.NotificationOperationDeleted)

	assert.Equal(t, []string{"delete"}, h.spy.Calls())
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
