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
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	fixtureObjectID = 42
	fixtureStream   = "fixtureStream"
	fixtureBucket   = "fixtureLock"

	instanceSubject    = "reconcilerTestInstance.notify"
	instanceReconciler = "ReconcilerTestInstanceReconciler"

	volatileSubject    = "reconcilerTestVolatileInstance.notify"
	volatileReconciler = "ReconcilerTestVolatileInstanceReconciler"
)

// harness runs the generated reconciler against an embedded JetStream server
// and an httptest API stub. Tests publish notifications through it and read
// back which operation handler the dispatch loop chose.
type harness struct {
	t   *testing.T
	js  nats.JetStreamContext
	spy *Spy
	api *machinetest.APIStub

	objMu    sync.Mutex
	obj      *api.ReconcilerTestInstance
	volatile *api.ReconcilerTestVolatileInstance

	// apiStatus is the status the object-fetch endpoint answers with. A 404
	// drives the halt path, where the reconciler stops without requeuing
	// because the object is gone.
	apiStatus atomic.Int64
}

// startNatsServer runs an in-process NATS server with JetStream enabled on a
// free port, backed by a scratch store directory.
func startNatsServer(t *testing.T) *natsserver.Server {
	t.Helper()

	server, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)

	go server.Start()
	require.True(t, server.ReadyForConnections(10*time.Second), "nats server did not start")

	return server
}

// newHarness wires the embedded server, the API stub, and both fixture
// reconcilers together, then runs their loops until the test ends.
//
// Teardown order matters. A loop parks in a 20 second Fetch, so the connection
// closes first to release it; each loop then sees the shutdown signal and
// returns before the server goes down. Stopping the server while its stream
// still exists would make Fetch report a missing stream, and PullMessage
// treats that as fatal and calls os.Exit.
func newHarness(t *testing.T) *harness {
	t.Helper()

	server := startNatsServer(t)
	t.Cleanup(server.Shutdown)

	conn, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)

	js, err := conn.JetStream()
	require.NoError(t, err)

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     fixtureStream,
		Subjects: []string{instanceSubject, volatileSubject},
	})
	require.NoError(t, err)

	keyValue, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: fixtureBucket})
	require.NoError(t, err)

	h := &harness{
		t:   t,
		js:  js,
		spy: NewSpy(),
		api: machinetest.NewAPIStub(t),
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
	t.Cleanup(InstallSpy(h.spy))

	// the reconciler fetches the latest object before dispatching, and
	// patches it after a non-delete operation succeeds
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

	// the volatile object's reconciler never fetches, but it still patches
	// Reconciled after a successful operation
	h.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", api.PathReconcilerTestVolatileInstances, fixtureObjectID),
		func(w http.ResponseWriter, r *http.Request) {
			h.objMu.Lock()
			defer h.objMu.Unlock()
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*h.volatile})
		},
	)

	h.startReconciler(conn, js, keyValue, instanceReconciler, instanceSubject, ReconcilerTestInstanceReconciler)
	h.startReconciler(conn, js, keyValue, volatileReconciler, volatileSubject, ReconcilerTestVolatileInstanceReconciler)

	t.Cleanup(conn.Close)

	return h
}

// startReconciler subscribes one reconciler to its subject and runs its loop
// until the test ends.
func (h *harness) startReconciler(
	conn *nats.Conn,
	js nats.JetStreamContext,
	keyValue nats.KeyValue,
	name string,
	subject string,
	reconcile func(*controller.Reconciler),
) {
	h.t.Helper()

	sub, err := js.PullSubscribe(subject, name+"Consumer", nats.BindStream(fixtureStream))
	require.NoError(h.t, err)

	ready := &atomic.Bool{}
	ready.Store(true)
	log := logr.Discard()
	shutdown := make(chan bool, 1)
	var shutdownWait sync.WaitGroup

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
		EventsRecorder:   machinetest.NewFakeRecorder(),
	})

	h.t.Cleanup(func() {
		shutdown <- true
		shutdownWait.Wait()
	})
}

// scheduleDeletion stamps DeletionScheduled on the object the API stub serves.
// A DELETE against the real API stamps the same field before publishing the
// delete notification.
func (h *harness) scheduleDeletion() {
	h.objMu.Lock()
	defer h.objMu.Unlock()
	h.obj.DeletionScheduled = util.Ptr(time.Now().UTC())
	h.volatile.DeletionScheduled = util.Ptr(time.Now().UTC())
}

// publish sends a notification for one operation on the ReconcilerTestInstance
// object and waits for the loop to finish with it, so the assertion that
// follows reads a settled spy.
func (h *harness) publish(operation notifications.NotificationOperation) {
	h.t.Helper()

	h.objMu.Lock()
	payload, err := h.obj.NotificationPayload(operation, false, time.Now().Unix())
	h.objMu.Unlock()
	require.NoError(h.t, err)

	_, err = h.js.Publish(instanceSubject, *payload)
	require.NoError(h.t, err)

	h.settle(instanceReconciler)
}

// publishVolatile sends a notification for one operation on the
// ReconcilerTestVolatileInstance object, whose reconciler skips the
// latest-object fetch.
func (h *harness) publishVolatile(operation notifications.NotificationOperation) {
	h.t.Helper()

	h.objMu.Lock()
	payload, err := h.volatile.NotificationPayload(operation, false, time.Now().Unix())
	h.objMu.Unlock()
	require.NoError(h.t, err)

	_, err = h.js.Publish(volatileSubject, *payload)
	require.NoError(h.t, err)

	h.settle(volatileReconciler)
}

// settle waits until the named reconcile loop has released its lock on the
// object, which it does at the end of every pass whichever branch it took.
func (h *harness) settle(reconcilerName string) {
	h.t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		if h.locked(reconcilerName) {
			continue
		}
		// two consecutive unlocked reads, so a pass that has not yet taken
		// the lock is not mistaken for one that has finished
		time.Sleep(75 * time.Millisecond)
		if !h.locked(reconcilerName) {
			return
		}
	}
	h.t.Fatalf("%s did not finish within 15s", reconcilerName)
}

// locked reports whether the named reconciler currently holds the object lock.
func (h *harness) locked(reconcilerName string) bool {
	h.t.Helper()

	keyValue, err := h.js.KeyValue(fixtureBucket)
	require.NoError(h.t, err)

	entry, err := keyValue.Get(fmt.Sprintf("%s.%d", reconcilerName, fixtureObjectID))
	return err == nil && entry != nil
}

// TestReconcilerDispatchesEachOperation asserts the generated switch routes
// each notification operation to its own handler. Everything below assumes
// this mapping holds.
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

// TestUpdateSkippedWhenDeletionScheduled covers the deadlock this fixture
// exists for.
//
// A failing update handler leaves its notification in JetStream redelivery.
// Without a guard on the update branch, the loop keeps re-running the update
// for as long as it keeps failing, and the delete branch never gets a turn on
// the single reconciler worker. Deletion then blocks indefinitely, and because
// the redelivery is a durable NAK rather than in-process retry state, a
// controller restart does not clear it.
func TestUpdateSkippedWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	h.spy.SetResult("update", Result{RequeueDelay: 1, Err: fmt.Errorf("update handler failed")})
	h.scheduleDeletion()

	h.publish(notifications.NotificationOperationUpdated)

	assert.Empty(t, h.spy.Calls(), "update ran on an object already scheduled for deletion")

	h.publish(notifications.NotificationOperationDeleted)

	assert.Contains(t, h.spy.Calls(), "delete", "delete never reached its handler")
}

// TestCreateSkippedWhenDeletionScheduled asserts the create branch skips its
// handler once deletion is scheduled. Provisioning for an object on its way
// out leaves resources behind that nothing owns.
func TestCreateSkippedWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	h.scheduleDeletion()

	h.publish(notifications.NotificationOperationCreated)

	assert.Empty(t, h.spy.Calls(), "create ran on an object already scheduled for deletion")
}

// TestDeleteRunsWhenDeletionScheduled asserts the delete branch carries no
// such guard. Deletion is always scheduled before the delete operation runs,
// so a guard here would skip the only handler that tears the object down, and
// it would never leave the database.
func TestDeleteRunsWhenDeletionScheduled(t *testing.T) {
	h := newHarness(t)
	h.scheduleDeletion()

	h.publish(notifications.NotificationOperationDeleted)

	assert.Equal(t, []string{"delete"}, h.spy.Calls())
}

// TestHaltsWhenObjectNoLongerExists asserts the reconciler stops without
// requeuing when the latest-object fetch answers 404. The object is gone, so
// retrying would keep an unacked message cycling for an ID nothing can
// reconcile.
func TestHaltsWhenObjectNoLongerExists(t *testing.T) {
	h := newHarness(t)
	h.apiStatus.Store(http.StatusNotFound)

	h.publish(notifications.NotificationOperationUpdated)

	assert.Empty(t, h.spy.Calls(), "dispatched an operation for an object the API says is gone")
}

// TestVolatileObjectDispatchesWithoutFetch asserts an object carrying a
// persist:"false" field reaches its handler even when the API cannot serve it.
//
// The generator omits the latest-object fetch for such an object, because the
// unpersisted value arrives only in the notification and re-reading the record
// would lose it. Holding both objects against the same 404 shows the two
// emission shapes apart: the fetching one halts, this one proceeds.
func TestVolatileObjectDispatchesWithoutFetch(t *testing.T) {
	h := newHarness(t)
	h.apiStatus.Store(http.StatusNotFound)

	h.publish(notifications.NotificationOperationUpdated)
	require.Empty(t, h.spy.Calls(), "the fetching object should have halted on 404")

	h.publishVolatile(notifications.NotificationOperationUpdated)

	assert.Equal(t, []string{"volatile-update"}, h.spy.Calls())
}
