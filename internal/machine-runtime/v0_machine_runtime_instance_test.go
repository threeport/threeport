package machineruntime

import (
	"context"
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

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
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
	mri.HostKey = util.Ptr(machinetest.HostKeyFromSigner(signer))

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

	// check success with no requeue
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)

	// check the handler records no event
	assert.Empty(t, recorder.GetReasons(), "reconciler emits no Normal event on the success path; the wrapper covers the outcome and reachability is a log line")
}

// TestMachineRuntimeInstanceCreated_NoHostname_RequeuesWithoutDialing covers
// the deferred-dial path: an instance whose hostname is not yet populated
// requeues with the unpopulated delay, returns no error, dials no SSH server,
// records no event, and persists no update, so Reconciled stays unset until
// the machine is reachable. Both a nil and an empty-string hostname take this
// path.
func TestMachineRuntimeInstanceCreated_NoHostname_RequeuesWithoutDialing(t *testing.T) {
	overrideUnpopulatedRequeueDelay(t, 3)
	// fail loudly if the deferred-dial guard ever lets a reconcile reach the
	// SSH dial: the connect would build a reconcile context from this factory
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
			// arrange an instance whose hostname is unpopulated so the
			// reconcile must take the deferred-dial path
			mri := &v0.MachineRuntimeInstance{
				Common:      v0.Common{ID: util.Ptr(uint(101))},
				Instance:    v0.Instance{Name: util.Ptr("mri-unpopulated")},
				SSHPassword: util.Ptr("ignored"),
				Hostname:    tc.hostname,
			}

			// count PATCHes so a persisted update would register as nonzero
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

			// run the Created hook against the unpopulated instance
			delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
			// verify the deferred-dial path returns cleanly rather than failing
			require.NoError(t, err, "an unpopulated instance must requeue without erroring")
			// verify it requeues on the unpopulated delay, not the ssh-retry delay
			assert.Equal(t, int64(3), delay, "an unpopulated instance requeues with the unpopulated delay")
			// verify no event fires before the machine is reachable
			assert.Empty(t, recorder.GetReasons(), "no event may be recorded before the machine is reachable")
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
	mri.HostKey = util.Ptr(machinetest.HostKeyFromSigner(wrongSigner))

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

// overrideRetryDelay shrinks the package retry delay for one test and
// restores it on cleanup. Tests in this package run sequentially, so
// mutating the package var is safe.
func overrideRetryDelay(t *testing.T, seconds int64) {
	t.Helper()
	prev := sshRetryDelaySeconds
	sshRetryDelaySeconds = seconds
	t.Cleanup(func() { sshRetryDelaySeconds = prev })
}

// overrideUnpopulatedRequeueDelay sets the unpopulated-instance requeue
// delay for one test and restores it on cleanup.
func overrideUnpopulatedRequeueDelay(t *testing.T, seconds int64) {
	t.Helper()
	prev := unpopulatedRequeueDelaySeconds
	unpopulatedRequeueDelaySeconds = seconds
	t.Cleanup(func() { unpopulatedRequeueDelaySeconds = prev })
}

// overrideSSHTimeout shrinks the package SSH operation timeout for one test
// and restores it on cleanup.
func overrideSSHTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := sshOperationTimeout
	sshOperationTimeout = d
	t.Cleanup(func() { sshOperationTimeout = prev })
}

// overrideReconcileContext swaps the package reconcile-context factory for
// one test and restores it on cleanup.
func overrideReconcileContext(t *testing.T, fn func() (context.Context, context.CancelFunc)) {
	t.Helper()
	prev := newReconcileContext
	newReconcileContext = fn
	t.Cleanup(func() { newReconcileContext = prev })
}

// registerPatchCounter registers a PATCH handler for the MRI with the given
// id that counts calls and replies with a valid envelope, so tests can
// assert exactly how many updates the reconciler persisted.
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

// TestMachineRuntimeInstanceCreated_IdempotentOnDoubleCall proves a
// re-reconcile of an MRI whose host key is already persisted does not PATCH
// again. Leg one runs in capture mode (HostKey nil) and persists the key
// with exactly one PATCH; leg two carries the server's real host key, so
// GetClient runs in verification mode, the capture branch is skipped, and
// no second PATCH lands.
func TestMachineRuntimeInstanceCreated_IdempotentOnDoubleCall(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	api := machinetest.NewAPIStub(t)
	patchCount := registerPatchCounter(t, api, 21)
	log := logr.Discard()

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
	assert.Equal(t, []string{"HostKeyCaptured", "SSHReachable"}, firstRecorder.GetReasons())
	require.Equal(t, int64(1), atomic.LoadInt64(patchCount), "first reconcile persists the captured host key with one PATCH")

	second := machinetest.NewMRIWithInfra(t, 21, "mri-idem", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: machinetest.HostKeyFromSigner(signer),
	})
	secondRecorder := machinetest.NewFakeRecorder()
	r.EventsRecorder = secondRecorder

	delay, err = v0MachineRuntimeInstanceCreated(r, second, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Equal(t, []string{"SSHReachable"}, secondRecorder.GetReasons())
	assert.Equal(t, int64(1), atomic.LoadInt64(patchCount), "second reconcile must not re-PATCH the host key")
}

// TestMachineRuntimeInstanceCreated_SSHPingFails_Retries drives a connect
// that succeeds and a ping that fails (non-zero exit), asserting the
// configurable retry delay is returned, SSHPingFailed is recorded, and no
// update is persisted.
func TestMachineRuntimeInstanceCreated_SSHPingFails_Retries(t *testing.T) {
	overrideRetryDelay(t, 7)
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 1})
	defer stop()

	mri := machinetest.NewMRIWithInfra(t, 31, "mri-pingfail", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: machinetest.HostKeyFromSigner(signer),
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

	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	require.Error(t, err)
	assert.Equal(t, int64(7), delay, "ping failures requeue with the configurable delay")
	assert.Equal(t, []string{"SSHPingFailed"}, recorder.GetReasons())
	assert.Equal(t, int64(0), atomic.LoadInt64(patchCount), "no update may be persisted on ping failure")
}

// TestMachineRuntimeInstanceCreated_HostKeyPatchFails_Retries closes the
// API stub before the reconcile so the host-key PATCH hits a refused
// connection. A transport-level failure must requeue (non-zero delay,
// non-nil error) so the failed persist is retried rather than the object
// being silently marked reconciled.
func TestMachineRuntimeInstanceCreated_HostKeyPatchFails_Retries(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	mri := machinetest.MRIFromAddr(t, 41, "mri-patchfail", addr, "u", "p", key)

	api := machinetest.NewAPIStub(t)
	api.Server.Close()

	recorder := machinetest.NewFakeRecorder()
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	require.Error(t, err)
	assert.Equal(t, int64(30), delay, "transport failures on the host-key PATCH requeue after 30s")
	assert.Empty(t, recorder.GetReasons(), "no events may be recorded when the capture PATCH fails")
}

// TestMachineRuntimeInstanceCreated_HostKeyPatchHTTP500_TerminalError is
// the sibling of the transport-failure case: an HTTP 500 on the host-key
// PATCH is not a network error, so the delay is 0, but the error must
// still be non-nil so the dispatch requeues instead of marking the object
// reconciled.
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

	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	require.Error(t, err)
	assert.Equal(t, int64(0), delay, "an http 500 is not a network error, so no requeue delay")
	assert.Empty(t, recorder.GetReasons())
}

// TestMachineRuntimeInstanceCreated_EventRecordingFailure_Continues sets
// the recorder to fail every call and asserts a happy-path reconcile still
// succeeds; event persistence must never block reconciliation.
func TestMachineRuntimeInstanceCreated_EventRecordingFailure_Continues(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	mri := machinetest.NewMRIWithInfra(t, 71, "mri-eventfail", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: machinetest.HostKeyFromSigner(signer),
	})

	api := machinetest.NewAPIStub(t)
	recorder := machinetest.NewFakeRecorder()
	recorder.RecordErr = errors.New("event store down")
	log := logr.Discard()
	r := &controller.Reconciler{
		APIClient:      api.Client,
		APIServer:      api.Addr,
		EncryptionKey:  key,
		EventsRecorder: recorder,
	}

	delay, err := v0MachineRuntimeInstanceCreated(r, mri, &log)
	require.NoError(t, err, "event persistence failures must not block reconciliation")
	assert.Equal(t, int64(0), delay)
	assert.Equal(t, []string{"SSHReachable"}, recorder.GetReasons())
}

// TestMachineRuntimeInstanceCreated_ContextCancellation_AbortsSSH injects
// an already-canceled reconcile context and asserts the handler returns
// promptly with the configurable retry delay, and that the abandoned
// connect's client is closed behind it so no connection or goroutine is
// left hanging.
func TestMachineRuntimeInstanceCreated_ContextCancellation_AbortsSSH(t *testing.T) {
	overrideRetryDelay(t, 5)
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

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(5), delay)
	assert.Less(t, elapsed, 5*time.Second, "an already-canceled context must abort the reconcile promptly")
	assert.Equal(t, []string{"SSHConnectFailed"}, recorder.GetReasons())

	// stopping the server waits for every accepted connection to finish
	// serving, which only happens once the abandoned connect's client has
	// been closed; a leaked client would hang the stop and fail the run
	stop()
	assert.Equal(t, int64(0), openConns.Load(), "abandoned ssh connection must be closed")
}

// TestMachineRuntimeInstanceCreated_SSHOperationTimeout_ReturnsErrorWithDelay
// points the reconcile at a server that holds the ping session open far
// past the operation timeout and asserts the handler returns within the
// timeout window with the configurable retry delay, instead of hanging for
// the full hold.
func TestMachineRuntimeInstanceCreated_SSHOperationTimeout_ReturnsErrorWithDelay(t *testing.T) {
	overrideRetryDelay(t, 9)
	overrideSSHTimeout(t, 100*time.Millisecond)

	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{HoldSession: 30 * time.Second})
	defer stop()

	mri := machinetest.NewMRIWithInfra(t, 91, "mri-timeout", addr, "u", "p", key, machinetest.MRIInfraOpts{
		HostKey: machinetest.HostKeyFromSigner(signer),
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

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int64(9), delay)
	assert.Less(t, elapsed, 5*time.Second, "timeout must fire well before the held session would release")
	assert.Equal(t, []string{"SSHPingFailed"}, recorder.GetReasons())
}

// TestMachineRuntimeInstanceCreated_SSHConnectTimeout_ReturnsErrorWithDelay
// is the connect-phase sibling of the ping timeout test: the server holds
// each accepted connection before any protocol bytes, so the dial blocks
// as if the host were not responding, and the handler must return within
// the operation timeout with the configurable retry delay.
func TestMachineRuntimeInstanceCreated_SSHConnectTimeout_ReturnsErrorWithDelay(t *testing.T) {
	overrideRetryDelay(t, 11)
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

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int64(11), delay)
	assert.Less(t, elapsed, 5*time.Second, "timeout must fire well before the held handshake would release")
	assert.Equal(t, []string{"SSHConnectFailed"}, recorder.GetReasons())
}

// TestMachineRuntimeInstanceDeleted_ReclaimsProviderResources covers the
// Deleted hook: a provider-provisioned MRI that still holds a resource
// inventory records a warning so the operator reclaims the live provider
// resources, a provisioned MRI with no inventory is a clean no-op, and an
// imported machine is a clean no-op.
func TestMachineRuntimeInstanceDeleted_ReclaimsProviderResources(t *testing.T) {
	key := machinetest.NewEncryptionKey(t)
	api := machinetest.NewAPIStub(t)
	log := logr.Discard()

	t.Run("provisioned with inventory warns to reclaim resources", func(t *testing.T) {
		// arrange a provider-provisioned mri that still holds a resource inventory
		mri := machinetest.NewMRIWithInfra(t, 61, "mri-del-provisioned", "127.0.0.1:22", "u", "p", key, machinetest.MRIInfraOpts{
			MachineRuntimeDefinitionID: 7,
			Region:                     "us-central1",
			ResourceInventory:          `{"vmId":"i-123"}`,
		})
		recorder := machinetest.NewFakeRecorder()
		r := &controller.Reconciler{
			APIClient:      api.Client,
			APIServer:      api.Addr,
			EncryptionKey:  key,
			EventsRecorder: recorder,
		}

		// run the Deleted hook against the provisioned instance
		delay, err := v0MachineRuntimeInstanceDeleted(r, mri, &log)
		require.NoError(t, err)
		// verify delete finishes instead of blocking on the live provider resources
		assert.Equal(t, int64(0), delay, "delete must complete rather than block on live resources")
		// verify the operator is warned to reclaim the still-live resources
		assert.Equal(t, []string{"ProviderResourcesNotReclaimed"}, recorder.GetReasons())
	})

	t.Run("provisioned without inventory is a no-op", func(t *testing.T) {
		// arrange a provider-provisioned mri that holds no resource inventory
		mri := machinetest.NewMRIWithInfra(t, 63, "mri-del-no-inventory", "127.0.0.1:22", "u", "p", key, machinetest.MRIInfraOpts{
			MachineRuntimeDefinitionID: 7,
			Region:                     "us-central1",
		})
		recorder := machinetest.NewFakeRecorder()
		r := &controller.Reconciler{
			APIClient:      api.Client,
			APIServer:      api.Addr,
			EncryptionKey:  key,
			EventsRecorder: recorder,
		}

		// run the Deleted hook against the inventory-free instance
		delay, err := v0MachineRuntimeInstanceDeleted(r, mri, &log)
		require.NoError(t, err)
		assert.Equal(t, int64(0), delay)
		// verify no warning fires when there are no recorded resources to reclaim
		assert.Empty(t, recorder.GetReasons(), "a provisioned machine with no recorded resources has nothing to reclaim")
	})

	t.Run("imported machine is a no-op", func(t *testing.T) {
		mri := machinetest.MRIFromAddr(t, 62, "mri-del-imported", "127.0.0.1:22", "u", "p", key)
		recorder := machinetest.NewFakeRecorder()
		r := &controller.Reconciler{
			APIClient:      api.Client,
			APIServer:      api.Addr,
			EncryptionKey:  key,
			EventsRecorder: recorder,
		}

		delay, err := v0MachineRuntimeInstanceDeleted(r, mri, &log)
		require.NoError(t, err)
		assert.Equal(t, int64(0), delay)
		assert.Empty(t, recorder.GetReasons(), "imported machines have nothing to deprovision")
	})
}

// TestMachineRuntimeInstanceCreated_ConcurrentReconciles_NoRace runs many
// Created reconciles for distinct MRIs concurrently against one SSH
// server. Every reconcile must succeed and every recorder must hold
// exactly its own SSHReachable event, proving no shared state bleeds
// between concurrent reconciles. Run under -race.
func TestMachineRuntimeInstanceCreated_ConcurrentReconciles_NoRace(t *testing.T) {
	const n = 50
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	api := machinetest.NewAPIStub(t)
	log := logr.Discard()
	hostKey := machinetest.HostKeyFromSigner(signer)

	// build all inputs on the test goroutine; the require-based helpers
	// are not safe to call from spawned goroutines
	mris := make([]*v0.MachineRuntimeInstance, n)
	recorders := make([]*machinetest.FakeRecorder, n)
	for i := 0; i < n; i++ {
		mris[i] = machinetest.NewMRIWithInfra(t, uint(1000+i), fmt.Sprintf("mri-conc-%d", i), addr, "u", "p", key, machinetest.MRIInfraOpts{
			HostKey: hostKey,
		})
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

	for i := 0; i < n; i++ {
		require.NoError(t, errs[i], "reconcile %d", i)
		assert.Equal(t, int64(0), delays[i], "reconcile %d", i)
		assert.Equal(t, []string{"SSHReachable"}, recorders[i].GetReasons(), "reconcile %d", i)
	}
}

// TestMachineRuntimeInstanceCreated_ManyConcurrent_NoConnLeak runs a wide
// burst of concurrent reconciles against a connection-counting SSH server
// and asserts every SSH connection is closed and goroutines settle back to
// the pre-burst baseline after the burst drains. This proves the SSH path
// closes connections and leaks no goroutines under width; it makes no
// claim about provider-level concurrency limits. Run under -race.
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
	hostKey := machinetest.HostKeyFromSigner(signer)

	mris := make([]*v0.MachineRuntimeInstance, n)
	recorders := make([]*machinetest.FakeRecorder, n)
	for i := 0; i < n; i++ {
		mris[i] = machinetest.NewMRIWithInfra(t, uint(2000+i), fmt.Sprintf("mri-leak-%d", i), addr, "u", "p", key, machinetest.MRIInfraOpts{
			HostKey: hostKey,
		})
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

	// every ssh client must be closed after the burst drains
	require.Eventually(t, func() bool {
		return openConns.Load() == 0
	}, 10*time.Second, 20*time.Millisecond, "open ssh connections must drain to zero")

	// goroutines must settle back near the pre-burst baseline
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseline+10
	}, 10*time.Second, 20*time.Millisecond, "goroutines must return to baseline after the burst")
}
