package machineworkload

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	logr "github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	wlstatus "github.com/threeport/threeport/internal/kubernetes-workload/status"
	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// dialSSH opens an ssh.Client against addr with insecure host key
// verification (test only) and password auth. Local to the workload test
// package, since the runtime reconciler tests build their client through
// machine.GetClient, so this dialer is workload only and lives here
// rather than in the shared machinetest package.
func dialSSH(t *testing.T, addr, user, password string) *ssh.Client {
	t.Helper()
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, cfg)
	require.NoError(t, err)
	return client
}

// TestBuildScript covers the script-assembly behavior: set -e prefix, optional
// cd, export of KEY=VALUE entries with value quoting, and trailing newline
// when the user script doesn't end with one.
func TestBuildScript(t *testing.T) {
	cases := []struct {
		name       string
		script     string
		workingDir string
		env        []string
		wantHas    []string
		wantNotHas []string
	}{
		{
			name:    "minimal",
			script:  "echo hi",
			wantHas: []string{"set -e\n", "echo hi\n"},
		},
		{
			name:       "with working dir",
			script:     "ls\n",
			workingDir: "/var/log",
			wantHas:    []string{"cd '/var/log'\n", "ls\n"},
		},
		{
			name:    "exports env entries with value quoting",
			script:  "true",
			env:     []string{"KEY=plain", "QUOTED=a b c"},
			wantHas: []string{"export KEY='plain'\n", "export QUOTED='a b c'\n"},
		},
		{
			name:    "env with single quote in value is shell-safe",
			script:  "true",
			env:     []string{"K=val'ue"},
			wantHas: []string{`export K='val'\''ue'`},
		},
		{
			name:       "empty env entries are skipped",
			script:     "true",
			env:        []string{"", "OK=1"},
			wantHas:    []string{"export OK='1'\n"},
			wantNotHas: []string{"export =", "export ''"},
		},
		{
			name:    "env without = exports bare name",
			script:  "true",
			env:     []string{"NOEQUALS"},
			wantHas: []string{"export NOEQUALS\n"},
		},
		{
			name:    "appends trailing newline when script has none",
			script:  "echo done",
			wantHas: []string{"echo done\n"},
		},
		{
			name:       "preserves existing trailing newline without doubling",
			script:     "echo done\n",
			wantHas:    []string{"echo done\n"},
			wantNotHas: []string{"echo done\n\n"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildScript(c.script, c.workingDir, c.env)
			for _, want := range c.wantHas {
				assert.Contains(t, got, want, "buildScript output should contain %q", want)
			}
			for _, notWant := range c.wantNotHas {
				assert.NotContains(t, got, notWant, "buildScript output should NOT contain %q", notWant)
			}
		})
	}
}

// TestShellQuote covers single-quote wrapping and embedded-quote escaping.
func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":      "'plain'",
		"":           "''",
		"a b c":      "'a b c'",
		"don't":      `'don'\''t'`,
		"''":         `''\'''\'''`,
		"$VAR `cmd`": "'$VAR `cmd`'",
	}
	for in, want := range cases {
		assert.Equal(t, want, shellQuote(in), "shellQuote(%q)", in)
	}
}

// TestTruncateMessage covers the under/over-threshold behavior and the
// truncation marker.
func TestTruncateMessage(t *testing.T) {
	short := strings.Repeat("a", 10)
	assert.Equal(t, short, truncateMessage(short), "short message should pass through unchanged")

	overLimit := strings.Repeat("b", maxEventMessageChars+50)
	got := truncateMessage(overLimit)
	assert.Less(t, len(got), len(overLimit), "overlong message should be shorter after truncation")
	assert.True(t, strings.HasSuffix(got, "...[truncated]"), "truncation marker should be appended")

	// boundary: exactly maxEventMessageChars stays untouched
	exact := strings.Repeat("c", maxEventMessageChars)
	assert.Equal(t, exact, truncateMessage(exact), "exactly-at-limit message should pass through unchanged")
}

// TestSanitizeScriptOutput covers ANSI escape stripping and carriage-return
// progress-redraw collapse.
func TestSanitizeScriptOutput(t *testing.T) {
	assert.Equal(t, "", sanitizeScriptOutput(""))

	withAnsi := "before \x1b[31mred text\x1b[0m after"
	assert.Equal(t, "before red text after", sanitizeScriptOutput(withAnsi))

	progressLine := "10%\r50%\r100% done\nnext line"
	got := sanitizeScriptOutput(progressLine)
	assert.Contains(t, got, "100% done", "the final state of a carriage-return-redrawn line should survive")
	assert.NotContains(t, got, "10%", "intermediate progress states should be collapsed")
	assert.NotContains(t, got, "50%", "intermediate progress states should be collapsed")
	assert.Contains(t, got, "next line")
}

// TestRunRemoteScript_ConnectionError covers the NewSession-failed branch
// using a closed SSH client.
func TestRunRemoteScript_ConnectionError(t *testing.T) {
	// stand up a server briefly, dial it, then close the connection to
	// force NewSession to fail
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	client := dialSSH(t, addr, "u", "p")
	require.NoError(t, client.Close(), "manual close to invalidate the client before runRemoteScript")

	stdout, stderr, exitCode, timedOut, err := runRemoteScript(
		client,
		"true",
		"sh",
		"",
		nil,
		nil,
	)
	assert.Error(t, err, "runRemoteScript should surface the NewSession failure as a connection error")
	assert.Equal(t, -1, exitCode)
	assert.False(t, timedOut)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

// TestRunRemoteScript_HappyPath confirms exit 0 returns no error and the
// expected exit code.
func TestRunRemoteScript_HappyPath(t *testing.T) {
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 0})
	defer stop()

	client := dialSSH(t, addr, "u", "p")
	defer client.Close()

	_, _, exitCode, timedOut, err := runRemoteScript(client, "echo ok\n", "sh", "", nil, nil)
	require.NoError(t, err, "happy-path script must return no transport error")
	assert.Equal(t, 0, exitCode)
	assert.False(t, timedOut)
}

// TestRunRemoteScript_ScriptFailed confirms a non-zero exit code is returned
// without a transport error.
func TestRunRemoteScript_ScriptFailed(t *testing.T) {
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{ExitCode: 7})
	defer stop()

	client := dialSSH(t, addr, "u", "p")
	defer client.Close()

	_, _, exitCode, timedOut, err := runRemoteScript(client, "false\n", "sh", "", nil, nil)
	require.NoError(t, err, "non-zero exit is not a transport error")
	assert.Equal(t, 7, exitCode)
	assert.False(t, timedOut)
}

// TestRunRemoteScript_Timeout confirms a long-running script gets killed and
// timedOut is reported.
func TestRunRemoteScript_Timeout(t *testing.T) {
	signer := machinetest.NewSigner(t)
	addr, stop := machinetest.StartSSHServer(t, signer, "u", "p", machinetest.SSHOpts{HoldSession: 2 * time.Second})
	defer stop()

	client := dialSSH(t, addr, "u", "p")
	defer client.Close()

	one := 1
	start := time.Now()
	_, _, exitCode, timedOut, err := runRemoteScript(client, "sleep 30\n", "sh", "", nil, &one)
	elapsed := time.Since(start)

	assert.True(t, timedOut, "expected timedOut=true when session exceeds timeout")
	assert.Equal(t, -1, exitCode)
	assert.Less(t, elapsed, 5*time.Second, "runRemoteScript should return shortly after the 1s deadline, not wait the full 30s")
	// err is allowed to be nil or non-nil depending on whether the server
	// returned an exit status before being killed; the timedOut flag is the
	// authoritative signal here
	_ = err
}

// ===== reconciler-level tests =====

// fixture wires up an httptest API stub, in-process SSH server, MWD/MWI
// fixtures, and a Reconciler. The returned helpers let each test customize
// per-endpoint behavior and stop the SSH server when done.
type fixture struct {
	t         *testing.T
	api       *machinetest.APIStub
	recorder  *machinetest.FakeRecorder
	r         *controller.Reconciler
	mri       *v0.MachineRuntimeInstance
	mwd       *v0.MachineWorkloadDefinition
	mwi       *v0.MachineWorkloadInstance
	patches   [][]byte
	patchesMu sync.Mutex
	stopSSH   func()
}

// newFixture spins up the SSH server with opts and registers stub handlers
// for the three endpoints the reconciler hits: GET MWD, GET MRI, and
// PATCH MWI. Tests can mutate f.mwd/f.mri/f.mwi before invoking the
// reconciler to drive specific branches.
func newFixture(t *testing.T, opts machinetest.SSHOpts) *fixture {
	t.Helper()
	key := machinetest.NewEncryptionKey(t)
	signer := machinetest.NewSigner(t)
	addr, stopSSH := machinetest.StartSSHServer(t, signer, "u", "p", opts)

	mri := machinetest.MRIFromAddr(t, 100, "mri-fixture", addr, "u", "p", key)
	mwd := &v0.MachineWorkloadDefinition{
		Common:       v0.Common{ID: util.Ptr(uint(200))},
		Definition:   v0.Definition{Name: util.Ptr("mwd-fixture")},
		CreateScript: util.Ptr("echo create"),
		UpdateScript: util.Ptr("echo update"),
		DeleteScript: util.Ptr("echo delete"),
	}
	mwi := &v0.MachineWorkloadInstance{
		Common:                      v0.Common{ID: util.Ptr(uint(300))},
		Instance:                    v0.Instance{Name: util.Ptr("mwi-fixture")},
		MachineRuntimeInstanceID:    util.Ptr(uint(100)),
		MachineWorkloadDefinitionID: util.Ptr(uint(200)),
	}

	f := &fixture{
		t:        t,
		api:      machinetest.NewAPIStub(t),
		recorder: machinetest.NewFakeRecorder(),
		mri:      mri,
		mwd:      mwd,
		mwi:      mwi,
		stopSSH:  stopSSH,
	}
	t.Cleanup(stopSSH)

	// GET /v0/machine-workload-definitions/200
	f.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", v0.PathMachineWorkloadDefinitions, *mwd.ID),
		func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*f.mwd})
		},
	)

	// GET /v0/machine-runtime-instances/100
	f.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", v0.PathMachineRuntimeInstances, *mri.ID),
		func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{*f.mri})
		},
	)

	// PATCH /v0/machine-workload-instances/300
	f.api.Mux.HandleFunc(
		fmt.Sprintf("%s/%d", v0.PathMachineWorkloadInstances, *mwi.ID),
		func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPatch, r.Method)
			body, _ := io.ReadAll(r.Body)
			f.patchesMu.Lock()
			f.patches = append(f.patches, body)
			f.patchesMu.Unlock()
			var updated v0.MachineWorkloadInstance
			require.NoError(t, json.Unmarshal(body, &updated))
			updated.ID = util.Ptr(uint(*f.mwi.ID))
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{updated})
		},
	)

	f.r = &controller.Reconciler{
		APIClient:      f.api.Client,
		APIServer:      f.api.Addr,
		EncryptionKey:  key,
		EventsRecorder: f.recorder,
	}
	return f
}

// patchedStatuses returns the Status value of every PATCH body received, in
// order. Tests assert the final status the reconciler persisted.
func (f *fixture) patchedStatuses() []string {
	f.patchesMu.Lock()
	defer f.patchesMu.Unlock()
	out := make([]string, 0, len(f.patches))
	for _, body := range f.patches {
		var mwi v0.MachineWorkloadInstance
		require.NoError(f.t, json.Unmarshal(body, &mwi))
		if mwi.Status != nil {
			out = append(out, *mwi.Status)
		} else {
			out = append(out, "")
		}
	}
	return out
}

// TestMachineWorkloadInstanceCreated_HappyPath confirms the Created
// reconciler runs the create script, persists Reconciled=true with a
// Healthy status, and records a ScriptSucceeded event.
func TestMachineWorkloadInstanceCreated_HappyPath(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 0})
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceCreated(f.r, f.mwi, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Equal(t, []string{string(wlstatus.WorkloadInstanceStatusHealthy)}, f.patchedStatuses())
	assert.Equal(t, []string{"ScriptSucceeded"}, f.recorder.GetReasons())
}

// TestMachineWorkloadInstanceCreated_RuntimeNotReconciled covers the early
// requeue path when the MRI hasn't finished its own Created reconcile yet.
// No script runs, no PATCH happens.
func TestMachineWorkloadInstanceCreated_RuntimeNotReconciled(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 0})
	f.mri.Reconciled = util.Ptr(false)
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceCreated(f.r, f.mwi, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(30), delay, "should requeue after 30s while runtime is unreconciled")
	assert.Empty(t, f.patchedStatuses(), "should not have PATCHed any status")
	assert.Empty(t, f.recorder.GetReasons(), "should not have recorded any events")
}

// TestMachineWorkloadInstanceCreated_ScriptFails covers the non-zero exit
// path: status persisted as Unhealthy, ScriptFailed event recorded, and
// the reconciler returns a non-nil error so the controller requeues.
func TestMachineWorkloadInstanceCreated_ScriptFails(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 1})
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceCreated(f.r, f.mwi, &log)
	require.Error(t, err)
	assert.Equal(t, int64(30), delay)
	assert.Equal(t, []string{string(wlstatus.WorkloadInstanceStatusUnhealthy)}, f.patchedStatuses())
	assert.Equal(t, []string{"ScriptFailed"}, f.recorder.GetReasons())
}

// TestMachineWorkloadInstanceCreated_GetDefinitionFails covers the
// pre-script-failure branch where the MWD lookup itself fails: no SSH, no
// PATCH, no events.
func TestMachineWorkloadInstanceCreated_GetDefinitionFails(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 0})
	// override the MWD handler to return 500
	f.api.Mux.HandleFunc(
		fmt.Sprintf("%s/", v0.PathMachineWorkloadDefinitions),
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
	)
	// point the MWI at an ID we haven't registered a handler for so the
	// catch-all 500 handler picks it up
	f.mwi.MachineWorkloadDefinitionID = util.Ptr(uint(999))
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceCreated(f.r, f.mwi, &log)
	require.Error(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Empty(t, f.patchedStatuses())
	assert.Empty(t, f.recorder.GetReasons())
}

// TestMachineWorkloadInstanceUpdated_HappyPath confirms Updated runs the
// update script with healthy result, mirroring Created.
func TestMachineWorkloadInstanceUpdated_HappyPath(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 0})
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceUpdated(f.r, f.mwi, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Equal(t, []string{string(wlstatus.WorkloadInstanceStatusHealthy)}, f.patchedStatuses())
	assert.Equal(t, []string{"ScriptSucceeded"}, f.recorder.GetReasons())
}

// TestMachineWorkloadInstanceUpdated_NoUpdateScript covers the early-return
// path when the definition has no UpdateScript: returns (0, nil), no SSH,
// no PATCH, no events.
func TestMachineWorkloadInstanceUpdated_NoUpdateScript(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 0})
	f.mwd.UpdateScript = nil
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceUpdated(f.r, f.mwi, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Empty(t, f.patchedStatuses())
	assert.Empty(t, f.recorder.GetReasons())
}

// TestMachineWorkloadInstanceDeleted_HappyPath confirms Deleted runs the
// delete script and returns (0, nil). Deleted does not PATCH the MWI - the
// generated reconciler removes the row.
func TestMachineWorkloadInstanceDeleted_HappyPath(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 0})
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceDeleted(f.r, f.mwi, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
	assert.Empty(t, f.patchedStatuses(), "Deleted should not PATCH the MWI; the generated reconciler handles removal")
	assert.Equal(t, []string{"ScriptSucceeded"}, f.recorder.GetReasons())
}

// TestMachineWorkloadInstanceDeleted_ScriptFails covers the non-zero exit
// path for delete: returns (30, err) so the reconciler retries.
func TestMachineWorkloadInstanceDeleted_ScriptFails(t *testing.T) {
	f := newFixture(t, machinetest.SSHOpts{ExitCode: 1})
	log := logr.Discard()

	delay, err := v0MachineWorkloadInstanceDeleted(f.r, f.mwi, &log)
	require.Error(t, err)
	assert.Equal(t, int64(30), delay)
	assert.Equal(t, []string{"ScriptFailed"}, f.recorder.GetReasons())
}
