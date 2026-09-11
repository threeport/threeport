package machineruntime

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	logr "github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

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
	mri.HostKey = util.Ptr(hostKeyBase64(signer))

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

// hostKeyBase64 returns the base64-encoded marshalled public key matching
// buildHostKeyCallback's verification-mode encoding.
func hostKeyBase64(signer interface{ PublicKey() ssh.PublicKey }) string {
	return base64.StdEncoding.EncodeToString(signer.PublicKey().Marshal())
}
