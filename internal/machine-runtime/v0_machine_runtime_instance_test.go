package machineruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logr "github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestMachineRuntimeInstanceCreated_HappyPath covers a reachable machine
// whose stored host key matches the server.
func TestMachineRuntimeInstanceCreated_HappyPath(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	// store the server's host key so the connect verifies rather than captures
	mri := machinetest.MRIFromAddr(t, 42, "mri-happy", addr, "u", "p", key)
	mri.HostKey = util.Ptr(hostKeyBase64(signer))

	// mock the PATCH the reconciler issues to stamp creation_confirmed and
	// record every request body so the test can assert exactly one fires
	api := machinetest.NewAPIStub(t)
	var (
		patches   [][]byte
		patchesMu sync.Mutex
		patchPath = fmt.Sprintf("%s/%d", v0.PathMachineRuntimeInstances, 42)
	)
	api.Mux.HandleFunc(patchPath, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		body, _ := io.ReadAll(r.Body)
		patchesMu.Lock()
		patches = append(patches, body)
		patchesMu.Unlock()
		var updated v0.MachineRuntimeInstance
		require.NoError(t, json.Unmarshal(body, &updated))
		updated.ID = util.Ptr(uint(42))
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{updated})
	})

	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()

	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// reconcile create
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// check success with no requeue
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)

	// check the handler records no event
	assert.Empty(t, recorder.GetReasons(), "reconciler emits no Normal event on the success path; the wrapper covers the outcome and reachability is a log line")
}

// TestMachineRuntimeInstanceCreated_NoHostname_RequeuesWithoutDialing covers
// the deferred-dial path: an instance whose hostname is not yet populated
// requeues with the unpopulated delay, returns no error, dials no SSH server,
// records nothing beyond the CreateInProgress lifecycle marker, and persists
// no update, so Reconciled stays unset until the machine is reachable. Both a
// nil and an empty-string hostname take this path.
func TestMachineRuntimeInstanceCreated_NoHostname_RequeuesWithoutDialing(t *testing.T) {
	// shrink the unpopulated requeue delay
	overrideUnpopulatedRequeueDelay(t, 3)
	// fail if the reconcile builds an ssh context
	overrideReconcileContext(t, func() (context.Context, context.CancelFunc) {
		t.Fatal("reconcile must not dial ssh when the hostname is unpopulated")
		return context.WithCancel(context.Background())
	})

	key := machinetest.NewEncryptionKey(t)

	cases := []struct {
		name     string
		hostname *string
	}{
		{"nil hostname", nil},
		{"empty hostname", util.Ptr("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// build an mri with an unpopulated hostname
			mri := &v0.MachineRuntimeInstance{
				Common:      v0.Common{ID: util.Ptr(uint(101))},
				Instance:    v0.Instance{Name: util.Ptr("mri-unpopulated")},
				SSHPassword: util.Ptr("ignored"),
				Hostname:    tc.hostname,
			}

			// count persisted updates
			api := machinetest.NewAPIStub(t)
			patchCount := registerPatchCounter(t, api, 101)
			recorder := machinetest.NewFakeRecorder()
			log := logr.Discard()
			r := &controller.Reconciler{
				APIClient:      api.Client,
				APIServer:      api.Addr,
				EncryptionKey:  key,
				EventsRecorder: recorder,
			}

			// run created
			delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
			// assert requeue without error, event, or patch
			require.NoError(t, err, "an unpopulated instance must requeue without erroring")
			assert.Equal(t, int64(3), delay, "an unpopulated instance requeues with the unpopulated delay")
			// verify only the lifecycle marker fires; nothing may claim
			// reachability before the machine has been dialed
			assert.Equal(t, []string{event.ReasonCreateInProgress}, recorder.GetReasons(), "no reachability event may be recorded before the machine is reachable")
			// verify nothing is persisted so Reconciled stays unset until reachable
			assert.Equal(t, int64(0), atomic.LoadInt64(patchCount), "no update may be persisted, so Reconciled stays unset")
		})
	}
}

// TestMachineRuntimeInstanceCreated_HostKeyCaptured covers the first-connect
// path: HostKey is nil, so GetClient captures the server's key, the
// reconciler PATCHes the MRI to persist it, and emits HostKeyCaptured +
// TestMachineRuntimeInstanceCreated_HostKeyCaptured covers first connect
// with no stored host key and persists the captured key.
func TestMachineRuntimeInstanceCreated_HostKeyCaptured(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	mri := machinetest.MRIFromAddr(t, 7, "mri-capture", addr, "u", "p", key)
	mri.HostKey = nil

	api := machinetest.NewAPIStub(t)
	var (
		patches   [][]byte
		patchesMu sync.Mutex
		patchPath = fmt.Sprintf("%s/%d", v0.PathMachineRuntimeInstances, 7)
	)
	api.Mux.HandleFunc(patchPath, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		body, _ := io.ReadAll(r.Body)
		patchesMu.Lock()
		patches = append(patches, body)
		patchesMu.Unlock()
		var updated v0.MachineRuntimeInstance
		require.NoError(t, json.Unmarshal(body, &updated))
		updated.ID = util.Ptr(uint(7))
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{updated})
	})

	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// reconcile create
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// check success with no requeue
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)

	// check the handler records no event
	assert.Empty(t, recorder.GetReasons(), "reconciler no longer emits boot-noise events on the create path")

	// check the captured host key is persisted with Reconciled true
	patchesMu.Lock()
	defer patchesMu.Unlock()
	require.Len(t, patches, 1, "expected exactly one PATCH to persist the captured host key")
	assert.Contains(t, string(patches[0]), "HostKey", "PATCH body should carry the HostKey field")
	assert.Contains(t, string(patches[0]), `"Reconciled":true`, "PATCH should set Reconciled=true so the resulting update notification does not retrigger reconciliation")
}

// TestMachineRuntimeInstanceCreated_NetworkError covers an unreachable SSH
// endpoint and returns a 30s requeue with a connect-failed event.
func TestMachineRuntimeInstanceCreated_NetworkError(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	// point the instance at 127.0.0.1:1 so the dial is refused
	mri := machinetest.MRIFromAddr(t, 9, "mri-unreachable", "127.0.0.1:1", "u", "p", key)

	api := machinetest.NewAPIStub(t)
	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// reconcile create
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// check the connect failure is retried after 30s
	require.Error(t, err)
	assert.Equal(t, int64(30), delay, "network-class errors should be retried after 30s")

	// check the error carries a connect-failed event
	var errWithEvent *tp_errors.ErrWithEvent
	require.ErrorAs(t, err, &errWithEvent, "reconciler should return *tp_errors.ErrWithEvent so the wrapper can substitute the specific reason")
	require.NotNil(t, errWithEvent.Event.Reason)
	assert.Equal(t, "SSHConnectFailed", *errWithEvent.Event.Reason)

	// check the handler records no event
	assert.Empty(t, recorder.GetReasons(), "failure path should not call RecordEvent directly; the wrapper substitutes the event")
}

// TestMachineRuntimeInstanceCreated_HostKeyMismatch covers a stored host
// key that does not match the server.
func TestMachineRuntimeInstanceCreated_HostKeyMismatch(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	serverSigner := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, serverSigner, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	// store a host key that does not match the server
	wrongSigner := machinetest.NewSigner(t)
	mri := machinetest.MRIFromAddr(t, 11, "mri-mismatch", addr, "u", "p", key)
	mri.HostKey = util.Ptr(hostKeyBase64(wrongSigner))

	api := machinetest.NewAPIStub(t)
	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// reconcile create
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// check the mismatch is retried after 30s so a corrected key can succeed
	require.Error(t, err)
	assert.Equal(t, int64(30), delay, "ssh-client errors always retry")

	// check the error carries a connect-failed event
	var errWithEvent *tp_errors.ErrWithEvent
	require.ErrorAs(t, err, &errWithEvent, "reconciler should return *tp_errors.ErrWithEvent so the wrapper can substitute the specific reason")
	require.NotNil(t, errWithEvent.Event.Reason)
	assert.Equal(t, "SSHConnectFailed", *errWithEvent.Event.Reason)

	// check the handler records no event
	assert.Empty(t, recorder.GetReasons(), "failure path should not call RecordEvent directly; the wrapper substitutes the event")
}

// overrideUnpopulatedRequeueDelay sets unpopulatedRequeueDelaySeconds for one test.
func overrideUnpopulatedRequeueDelay(t *testing.T, seconds int64) {
	t.Helper()
	prev := unpopulatedRequeueDelaySeconds
	unpopulatedRequeueDelaySeconds = seconds
	t.Cleanup(func() { unpopulatedRequeueDelaySeconds = prev })
}

// overrideSSHTimeout sets sshOperationTimeout for one test.
func overrideSSHTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := sshOperationTimeout
	sshOperationTimeout = d
	t.Cleanup(func() { sshOperationTimeout = prev })
}

// overrideReconcileContext sets newReconcileContext for one test.
func overrideReconcileContext(t *testing.T, fn func() (context.Context, context.CancelFunc)) {
	t.Helper()
	prev := newReconcileContext
	newReconcileContext = fn
	t.Cleanup(func() { newReconcileContext = prev })
}

// registerPatchCounter counts PATCH calls for one machine runtime instance id.
func registerPatchCounter(t *testing.T, api *machinetest.APIStub, id uint) *int64 {
	t.Helper()
	var count int64
	api.Mux.HandleFunc(fmt.Sprintf("%s/%d", v0.PathMachineRuntimeInstances, id), func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		atomic.AddInt64(&count, 1)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var updated v0.MachineRuntimeInstance
		require.NoError(t, json.Unmarshal(body, &updated))
		updated.ID = util.Ptr(id)
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{updated})
	})
	return &count
}

// registerMachineRuntimeDefinition registers a GET handler for the machine
// runtime definition with the given id that answers with a GCE-provisioned
// definition, so a reconcile that loads the parent to name the married
// provider kind finds one instead of a missing object.
func registerMachineRuntimeDefinition(t *testing.T, api *machinetest.APIStub, id uint) {
	t.Helper()
	api.Mux.HandleFunc(fmt.Sprintf("%s/%d", v0.PathMachineRuntimeDefinitions, id), func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{
			&v0.MachineRuntimeDefinition{
				Common:        v0.Common{ID: util.Ptr(id)},
				Definition:    v0.Definition{Name: util.Ptr("mrd-married")},
				InfraProvider: util.Ptr(v0.MachineRuntimeInfraProviderGCE),
			},
		})
	})
}

// TestMachineRuntimeInstanceCreated_IdempotentOnDoubleCall asserts a
// re-reconcile of an instance whose first-reachability write already landed
// does not write again. Leg one runs in capture mode with creation_confirmed
// unset, so exactly one PATCH persists the captured host key and the
// confirmation stamp together; leg two carries the server's real host key and
// a stamped creation_confirmed, so GetClient runs in verification mode, the
// write guard finds nothing new to record, and no second PATCH lands.
func TestMachineRuntimeInstanceCreated_IdempotentOnDoubleCall(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	api := machinetest.NewAPIStub(t)
	patchCount := registerPatchCounter(t, api, 21)
	log := logr.Discard()

	// leg one: no host key and no confirmation stamp, so the reachable pass
	// has both to record and issues the single combined PATCH
	first := machinetest.MRIFromAddr(t, 21, "mri-idem", addr, "u", "p", key)
	firstRecorder := machinetest.NewFakeRecorder()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: firstRecorder,
	}

	delay, err := v0MachineRuntimeInstanceCreated(r, first, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Empty(t, firstRecorder.GetReasons(), "host key capture is a log line, not a recorded event")
	require.Equal(t, int64(1), atomic.LoadInt64(patchCount), "first reconcile persists the captured host key with one PATCH")

	// leg two: the state leg one persisted, so the write guard has nothing
	// left to record
	second := machinetest.NewMRIWithInfra(t, 21, "mri-idem", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: hostKeyBase64(signer),
	})
	second.CreationConfirmed = util.Ptr(time.Now().UTC())
	secondRecorder := machinetest.NewFakeRecorder()
	r.EventsRecorder = secondRecorder

	delay, err = v0MachineRuntimeInstanceCreated(r, second, &log)
	// assert no second host-key patch
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Empty(t, secondRecorder.GetReasons(), "reachability is a log line, not a recorded event")
	assert.Equal(t, int64(1), atomic.LoadInt64(patchCount), "second reconcile must not re-PATCH the host key")
}

// TestMachineRuntimeInstanceCreated_SSHPingFails_Retries drives a connect
// that succeeds and a ping that fails (non-zero exit), asserting the
// configurable retry delay is returned, the error carries the SSHPingFailed
// event the wrapper substitutes for the generic FailedCreate row, and no
// update is persisted.
func TestMachineRuntimeInstanceCreated_SSHPingFails_Retries(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 1})
	defer stop()

	mri := machinetest.NewMRIWithInfra(t, 31, "mri-pingfail", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: hostKeyBase64(signer),
	})

	api := machinetest.NewAPIStub(t)
	patchCount := registerPatchCounter(t, api, 31)
	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// drive the Created reconciler against a host whose ping fails
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// the first failure on this instance requeues at the base delay, before
	// the backoff has had a chance to double it
	require.Error(t, err)
	assert.Equal(t, int64(7), delay, "ping failures requeue with the configurable delay")
	var errWithEvent *tp_errors.ErrWithEvent
	require.ErrorAs(t, err, &errWithEvent)
	require.NotNil(t, errWithEvent.Event.Reason)
	assert.Equal(t, "SSHPingFailed", *errWithEvent.Event.Reason)
	assert.Empty(t, recorder.GetReasons(), "the wrapper records the event from the returned error")
	assert.Equal(t, int64(0), atomic.LoadInt64(patchCount), "no update may be persisted on ping failure")
}

// TestMachineRuntimeInstanceCreated_HostKeyPatchFails_Retries closes the
// API stub before the reconcile so the single first-reachability PATCH,
// which persists the captured host key and stamps creation_confirmed, hits a
// refused connection. A transport-level failure must requeue (non-zero delay,
// non-nil error) so the failed persist is retried rather than the object
// being silently marked reconciled.
func TestMachineRuntimeInstanceCreated_HostKeyPatchFails_Retries(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	mri := machinetest.MRIFromAddr(t, 41, "mri-patchfail", addr, "u", "p", key)

	api := machinetest.NewAPIStub(t)
	// close the api stub so the host-key patch is refused
	api.Server.Close()

	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// drive the Created reconciler with the API unreachable
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// a refused connection is a network-class error, so the persist retries
	require.Error(t, err)
	assert.Equal(t, int64(30), delay, "transport failures on the first-reachability PATCH requeue after 30s")

	// the only event is the lifecycle marker at the top of the run; a
	// reconcile whose persist failed records nothing about reachability
	assert.Equal(t, []string{event.ReasonCreateInProgress}, recorder.GetReasons(), "a failed persist records nothing beyond the lifecycle marker")
}

// TestMachineRuntimeInstanceCreated_HostKeyPatchHTTP500_TerminalError is
// the sibling of the transport-failure case: an HTTP 500 on the
// first-reachability PATCH is not a network error, so the delay is 0, but the
// error must still be non-nil so the dispatch requeues instead of marking the
// object reconciled.
func TestMachineRuntimeInstanceCreated_HostKeyPatchHTTP500_TerminalError(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	mri := machinetest.MRIFromAddr(t, 51, "mri-patch500", addr, "u", "p", key)

	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(fmt.Sprintf("%s/%d", v0.PathMachineRuntimeInstances, 51), func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusInternalServerError, nil)
	})

	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// drive the Created reconciler against an API that rejects the persist
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)

	// a server-side rejection is terminal for this pass, but still an error
	require.Error(t, err)
	assert.Equal(t, int64(0), delay, "an http 500 is not a network error, so no requeue delay")
	assert.Equal(t, []string{event.ReasonCreateInProgress}, recorder.GetReasons(), "a rejected persist records nothing beyond the lifecycle marker")
}

// TestMachineRuntimeInstanceCreated_EventRecordingFailure_Continues sets
// the recorder to fail every call and asserts a happy-path reconcile still
// succeeds and still persists its first-reachability write; event persistence
// must never block reconciliation.
func TestMachineRuntimeInstanceCreated_EventRecordingFailure_Continues(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	mri := machinetest.NewMRIWithInfra(t, 71, "mri-eventfail", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: hostKeyBase64(signer),
	})

	api := machinetest.NewAPIStub(t)
	patchCount := registerPatchCounter(t, api, 71)
	recorder := machinetest.NewFakeRecorder()
	recorder.RecordErr = errors.New("event store down")
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	// drive the Created reconciler with every RecordEvent call failing
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	assert.Equal(t, int64(1), atomic.LoadInt64(patchCount), "the first-reachability write still persists when event recording fails")
	// assert created still succeeds
	require.NoError(t, err, "a failing recorder must not block reconciliation")
	assert.Equal(t, int64(0), delay)
	assert.Empty(t, recorder.GetReasons(), "the success path does not record an event")
}

// TestMachineRuntimeInstanceCreated_ContextCancellation_AbortsSSH injects
// an already-canceled reconcile context and asserts the handler returns
// promptly with the configurable retry delay, that the failure names the
// cancellation and carries the SSHConnectFailed event, and that the abandoned
// connect's client is closed behind it so no connection or goroutine is
// left hanging.
func TestMachineRuntimeInstanceCreated_ContextCancellation_AbortsSSH(t *testing.T) {
	overrideReconcileContext(t, func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx, cancel
	})

	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	var openConns atomic.Int64
	addr, stopServer := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{
		ExitCode:  0,
		OpenConns: &openConns,
	})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(stopServer) }
	defer stop()

	mri := machinetest.MRIFromAddr(t, 81, "mri-canceled", addr, "u", "p", key)

	api := machinetest.NewAPIStub(t)
	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	start := time.Now()
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	elapsed := time.Since(start)

	// the canceled context aborts the connect instead of waiting it out
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.Canceled.Error())
	var errWithEvent *tp_errors.ErrWithEvent
	require.ErrorAs(t, err, &errWithEvent)
	require.NotNil(t, errWithEvent.Event.Reason)
	assert.Equal(t, "SSHConnectFailed", *errWithEvent.Event.Reason)
	assert.Equal(t, int64(5), delay)
	assert.Less(t, elapsed, 5*time.Second, "an already-canceled context must abort the reconcile promptly")
	assert.Empty(t, recorder.GetReasons())

	// stop waits until accepted connections finish serving
	stop()
	assert.Equal(t, int64(0), openConns.Load(), "abandoned ssh connection must be closed")
}

// TestMachineRuntimeInstanceCreated_SSHOperationTimeout_ReturnsErrorWithDelay
// covers a ping session held past sshOperationTimeout.
func TestMachineRuntimeInstanceCreated_SSHOperationTimeout_ReturnsErrorWithDelay(t *testing.T) {
	overrideSSHTimeout(t, 100*time.Millisecond)

	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{HoldSession: 30 * time.Second})
	defer stop()

	mri := machinetest.NewMRIWithInfra(t, 91, "mri-timeout", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: hostKeyBase64(signer),
	})

	api := machinetest.NewAPIStub(t)
	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	start := time.Now()
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	elapsed := time.Since(start)

	// the operation timeout fires and reports the expired deadline as the cause
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
	var pingEvent *tp_errors.ErrWithEvent
	require.ErrorAs(t, err, &pingEvent)
	require.NotNil(t, pingEvent.Event.Reason)
	assert.Equal(t, "SSHPingFailed", *pingEvent.Event.Reason)
	assert.Equal(t, int64(9), delay)
	assert.Less(t, elapsed, 5*time.Second, "timeout must fire well before the held session would release")
	assert.Empty(t, recorder.GetReasons())
}

// TestMachineRuntimeInstanceCreated_SSHConnectTimeout_ReturnsErrorWithDelay
// covers a handshake held past sshOperationTimeout.
func TestMachineRuntimeInstanceCreated_SSHConnectTimeout_ReturnsErrorWithDelay(t *testing.T) {
	overrideSSHTimeout(t, 100*time.Millisecond)

	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{HoldHandshake: 30 * time.Second})
	defer stop()

	mri := machinetest.MRIFromAddr(t, 95, "mri-connect-timeout", addr, "u", "p", key)

	api := machinetest.NewAPIStub(t)
	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	start := time.Now()
	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	elapsed := time.Since(start)

	// the operation timeout fires and reports the expired deadline as the cause
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error(), "the aborted connect must name the expired deadline in its message")
	assert.Equal(t, int64(0), delay)
	assert.Less(t, elapsed, 5*time.Second, "timeout must fire well before the held handshake would release")

	// the abort surfaces as a connect failure the wrapper can substitute for
	// the generic FailedCreate row
	var errWithEvent *tp_errors.ErrWithEvent
	require.ErrorAs(t, err, &errWithEvent, "reconciler should return *tp_errors.ErrWithEvent so the wrapper can substitute the specific reason")
	require.NotNil(t, errWithEvent.Event.Reason)
	assert.Equal(t, "SSHConnectFailed", *errWithEvent.Event.Reason)
	assert.Equal(t, []string{event.ReasonCreateInProgress}, recorder.GetReasons(), "failure path should not call RecordEvent directly for the failure; the wrapper substitutes it")
}

// TestMachineRuntimeInstanceCreated_ConcurrentReconciles_NoRace covers concurrent
// Created reconciles for distinct instances against one ssh server.
func TestMachineRuntimeInstanceCreated_ConcurrentReconciles_NoRace(t *testing.T) {
	const n = 50
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	api := machinetest.NewAPIStub(t)
	log := logr.Discard()
	hostKey := hostKeyBase64(signer)
	confirmed := time.Now().UTC()

	// build inputs on the test goroutine; require helpers are not goroutine-safe
	mris := make([]*v0.MachineRuntimeInstance, n)
	recorders := make([]*machinetest.FakeRecorder, n)
	for i := 0; i < n; i++ {
		mris[i] = machinetest.NewMRIWithInfra(t, uint(1000+i), fmt.Sprintf("mri-conc-%d", i), addr, "u", "p", key, machinetest.MRIInfraOpts{
			HostKey: hostKey,
		})
		// pre-stamp the confirmation so a reachable pass has nothing new to
		// record, keeping the burst on the ssh path with no api traffic
		mris[i].CreationConfirmed = util.Ptr(confirmed)
		recorders[i] = machinetest.NewFakeRecorder()
	}

	var wg sync.WaitGroup
	delays := make([]int64, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := &controller.Reconciler{
				APIClient:      api.Client,
				APIServer:      api.Addr,
				EncryptionKey:  key,
				EventsRecorder: recorders[i],
			}
			delays[i], errs[i] = v0MachineRuntimeInstanceCreated(r, mris[i], &log)
		}(i)
	}
	wg.Wait()

	// assert each reconcile succeeded without recording an event
	for i := 0; i < n; i++ {
		require.NoError(t, errs[i], "reconcile %d", i)
		assert.Equal(t, int64(0), delays[i], "reconcile %d", i)
		assert.Empty(t, recorders[i].GetReasons(), "reconcile %d", i)
	}
}

// TestMachineRuntimeInstanceCreated_ManyConcurrent_NoConnLeak covers a burst of concurrent Created reconciles and waits for OpenConns to hit 0 and goroutines to return near baseline.
func TestMachineRuntimeInstanceCreated_ManyConcurrent_NoConnLeak(t *testing.T) {
	const n = 200
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	var openConns atomic.Int64
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{
		ExitCode:  0,
		OpenConns: &openConns,
	})
	defer stop()

	api := machinetest.NewAPIStub(t)
	log := logr.Discard()
	hostKey := hostKeyBase64(signer)
	confirmed := time.Now().UTC()

	mris := make([]*v0.MachineRuntimeInstance, n)
	recorders := make([]*machinetest.FakeRecorder, n)
	for i := 0; i < n; i++ {
		mris[i] = machinetest.NewMRIWithInfra(t, uint(2000+i), fmt.Sprintf("mri-leak-%d", i), addr, "u", "p", key, machinetest.MRIInfraOpts{
			HostKey: hostKey,
		})
		// pre-stamp the confirmation so the burst issues no api calls, whose
		// pooled connections and goroutines would blur the leak assertion
		mris[i].CreationConfirmed = util.Ptr(confirmed)
		recorders[i] = machinetest.NewFakeRecorder()
	}

	baseline := runtime.NumGoroutine()

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := &controller.Reconciler{
				APIClient:      api.Client,
				APIServer:      api.Addr,
				EncryptionKey:  key,
				EventsRecorder: recorders[i],
			}
			_, errs[i] = v0MachineRuntimeInstanceCreated(r, mris[i], &log)
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		require.NoError(t, errs[i], "reconcile %d", i)
	}

	// wait for ssh clients to close
	require.Eventually(t, func() bool {
		return openConns.Load() == 0
	}, 10*time.Second, 20*time.Millisecond, "open ssh connections must drain to zero")

	// wait for goroutines to return near the pre-burst baseline
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseline+10
	}, 10*time.Second, 20*time.Millisecond, "goroutines must return to baseline after the burst")
}

// hostKeyBase64 returns the base64-encoded marshalled public key matching
// buildHostKeyCallback's verification-mode encoding.
func hostKeyBase64(signer interface{ PublicKey() ssh.PublicKey }) string {
	return base64.StdEncoding.EncodeToString(signer.PublicKey().Marshal())
}
