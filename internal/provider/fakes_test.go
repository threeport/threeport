package provider

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"
)

// compile-time interface checks for the test fakes
var (
	_ InfraLifecycleProvider = (*fakeLifecycle)(nil)
	_ InfraProvider          = (*fakeInfra)(nil)
	_ StreamableProvider     = (*fakeStreamableInfra)(nil)
	_ RefreshableProvider    = (*fakeRefreshableInfra)(nil)
	_ Clock                  = (*fakeClock)(nil)
)

// errFakeInfra is the error returned in infraError mode when no
// error was injected.
var errFakeInfra = errors.New("fakeInfra: injected failure")

// baselineGoroutines is the process goroutine count captured before tests.
// It is printed in a leak report and is not the leak signal.
var baselineGoroutines int

// goroutineDrainTimeout is how long to wait for leftover lifecycle
// goroutines after the suite.
const goroutineDrainTimeout = 10 * time.Second

// TestMain runs the package tests and fails the suite if a lifecycle
// goroutine is still running afterward. Runtime leftovers are ignored.
func TestMain(m *testing.M) {
	// capture the process goroutine count before tests
	baselineGoroutines = runtime.NumGoroutine()

	// run the package tests
	code := m.Run()

	// check for leftover lifecycle goroutines only when tests passed
	if code == 0 {
		if leaked := lifecycleGoroutinesRemaining(goroutineDrainTimeout); leaked > 0 {
			// report the leak and fail the suite
			fmt.Fprintf(
				os.Stderr,
				"goroutine leak: %d lifecycle goroutines still running (process count %d above baseline %d)\n%s\n",
				leaked,
				runtime.NumGoroutine()-baselineGoroutines,
				baselineGoroutines,
				lifecycleGoroutineStacks(),
			)
			code = 1
		}
	}

	// exit with the suite status
	os.Exit(code)
}

// lifecycleGoroutineNames are the package.function strings matched in
// a runtime.Stack dump to count leftover lifecycle goroutines.
var lifecycleGoroutineNames = []string{
	"provider.refreshAck",
	"provider.streamState",
	"provider.executeInfraCreate",
	"provider.executeInfraDelete",
	"provider.launchInfraCreate",
	"provider.launchInfraDelete",
}

// lifecycleGoroutineStacks returns a runtime.Stack dump of every goroutine.
func lifecycleGoroutineStacks() string {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// countLifecycleGoroutines counts goroutines whose stacks name a lifecycle
// function. Each stack is counted at most once.
func countLifecycleGoroutines() int {
	count := 0
	// split the dump into one stack per goroutine
	for _, g := range strings.Split(lifecycleGoroutineStacks(), "\n\n") {
		// count a stack that names a lifecycle function once
		for _, name := range lifecycleGoroutineNames {
			if strings.Contains(g, name) {
				count++
				break
			}
		}
	}
	return count
}

// lifecycleGoroutinesRemaining waits up to timeout for lifecycle goroutines
// to exit and returns how many are still running.
func lifecycleGoroutinesRemaining(timeout time.Duration) int {
	// set the drain deadline
	deadline := time.Now().Add(timeout)
	for {
		// count remaining lifecycle goroutines
		left := countLifecycleGoroutines()
		// return when none remain
		if left == 0 {
			return 0
		}
		// return leftovers after the deadline
		if time.Now().After(deadline) {
			return left
		}
		// wait before counting again
		time.Sleep(10 * time.Millisecond)
	}
}

// newTestLogger returns a pointer to a discarding logger.
func newTestLogger() *logr.Logger {
	l := logr.Discard()
	return &l
}

// jsonPtr returns a pointer to s as JSON bytes.
func jsonPtr(s string) *datatypes.JSON {
	j := datatypes.JSON(s)
	return &j
}

// validStackState returns populated deployment-format stack JSON that
// passes state verification.
func validStackState() *datatypes.JSON {
	return jsonPtr(`{"deployment":{"resources":[{"urn":"urn:fake:resource"}]}}`)
}

// fakeClock is a Clock whose current time is set by the test.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock returns a Clock frozen at t.
func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

// Now returns the fake clock's current time.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the fake clock forward by d.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// infraMode is how a fake DeployInfra or DestroyInfra call completes.
type infraMode int

const (
	// infraSucceed returns nil from deploy and destroy.
	infraSucceed infraMode = iota

	// infraError returns deployErr, destroyErr, or errFakeInfra.
	infraError

	// infraPanic panics from deploy and destroy.
	infraPanic

	// infraBlock waits until the matching release channel is closed.
	infraBlock
)

// fakeInfra is an in-memory InfraProvider with programmable deploy and
// destroy behavior, safe for concurrent use.
type fakeInfra struct {
	mu sync.Mutex

	// The completion mode for DeployInfra
	deployMode infraMode
	// The error DeployInfra returns in infraError mode
	deployErr error

	// The completion mode for DestroyInfra
	destroyMode infraMode
	// The error DestroyInfra returns in infraError mode
	destroyErr error

	// The channel DeployInfra waits on in infraBlock mode
	deployRelease chan struct{}
	// Whether deployRelease has already been closed
	deployReleased bool
	// The channel DestroyInfra waits on in infraBlock mode
	destroyRelease chan struct{}
	// Whether destroyRelease has already been closed
	destroyReleased bool

	deployCalls   int
	destroyCalls  int
	setStateCalls int
	getStateCalls int

	// The arguments passed to SetStackState, in call order
	restoredStates []*datatypes.JSON
	// The error SetStackState returns
	setStateErr error

	// The value GetStackState returns
	stackState *datatypes.JSON
	// The error GetStackState returns
	getStateErr error
}

// newFakeInfra returns a fake that succeeds deploy and destroy and holds
// a populated stack state.
func newFakeInfra() *fakeInfra {
	return &fakeInfra{
		deployRelease:  make(chan struct{}),
		destroyRelease: make(chan struct{}),
		stackState:     validStackState(),
	}
}

// DeployInfra records the call and completes according to deployMode.
func (f *fakeInfra) DeployInfra() error {
	// record the call and copy mode under the lock
	f.mu.Lock()
	f.deployCalls++
	mode := f.deployMode
	err := f.deployErr
	release := f.deployRelease
	f.mu.Unlock()

	// run the copied mode after unlocking so a block does not hold mu
	return runInfraMode(mode, err, release, "fakeInfra: deploy panic")
}

// DestroyInfra records the call and completes according to destroyMode.
func (f *fakeInfra) DestroyInfra() error {
	// record the call and copy mode under the lock
	f.mu.Lock()
	f.destroyCalls++
	mode := f.destroyMode
	err := f.destroyErr
	release := f.destroyRelease
	f.mu.Unlock()

	// run the copied mode after unlocking so a block does not hold mu
	return runInfraMode(mode, err, release, "fakeInfra: destroy panic")
}

// runInfraMode completes a deploy or destroy call according to mode.
func runInfraMode(
	mode infraMode,
	err error,
	release chan struct{},
	panicMsg string,
) error {
	switch mode {
	case infraError:
		// return the injected error, or the shared fake failure
		if err != nil {
			return err
		}
		return errFakeInfra
	case infraPanic:
		// panic with the caller-supplied message
		panic(panicMsg)
	case infraBlock:
		// block until the release channel is closed
		<-release
		return nil
	}
	// succeed
	return nil
}

// SetStackState records state and returns setStateErr.
func (f *fakeInfra) SetStackState(state *datatypes.JSON) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setStateCalls++
	f.restoredStates = append(f.restoredStates, state)
	return f.setStateErr
}

// GetStackState returns stackState and getStateErr.
func (f *fakeInfra) GetStackState() (*datatypes.JSON, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getStateCalls++
	return f.stackState, f.getStateErr
}

// setDeploy sets how DeployInfra completes.
func (f *fakeInfra) setDeploy(mode infraMode, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deployMode = mode
	f.deployErr = err
}

// setDestroy sets how DestroyInfra completes.
func (f *fakeInfra) setDestroy(mode infraMode, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyMode = mode
	f.destroyErr = err
}

// setGetStackState sets the value and error GetStackState returns.
func (f *fakeInfra) setGetStackState(state *datatypes.JSON, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stackState = state
	f.getStateErr = err
}

// setSetStackStateErr sets the error SetStackState returns.
func (f *fakeInfra) setSetStackStateErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setStateErr = err
}

// releaseDeploy unblocks a blocked DeployInfra. A second call is a no-op.
func (f *fakeInfra) releaseDeploy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	// close the deploy release channel once
	if !f.deployReleased {
		close(f.deployRelease)
		f.deployReleased = true
	}
}

// releaseDestroy unblocks a blocked DestroyInfra. A second call is a no-op.
func (f *fakeInfra) releaseDestroy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	// close the destroy release channel once
	if !f.destroyReleased {
		close(f.destroyRelease)
		f.destroyReleased = true
	}
}

// deployCallCount returns how many times DeployInfra has been called.
func (f *fakeInfra) deployCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deployCalls
}

// destroyCallCount returns how many times DestroyInfra has been called.
func (f *fakeInfra) destroyCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.destroyCalls
}

// setStackStateCallCount returns how many times SetStackState has been called.
func (f *fakeInfra) setStackStateCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setStateCalls
}

// getStackStateCallCount returns how many times GetStackState has been called.
func (f *fakeInfra) getStackStateCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getStateCalls
}

// lastRestoredState returns the most recent argument to SetStackState.
func (f *fakeInfra) lastRestoredState() *datatypes.JSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	// return nil when nothing has been restored
	if len(f.restoredStates) == 0 {
		return nil
	}
	return f.restoredStates[len(f.restoredStates)-1]
}

// fakeStreamableInfra is an in-memory provider with a configurable state
// file path and in-memory state-file reads.
type fakeStreamableInfra struct {
	*fakeInfra

	smu sync.Mutex
	// The path GetStateFilePath returns
	stateFilePath string
	// The error GetStateFilePath returns
	stateFilePathErr error
	// The value ReadStateFile returns
	readState *datatypes.JSON
	// The error ReadStateFile returns
	readStateErr error
	readCalls    int
}

// newFakeStreamableInfra returns a streamable fake that reports
// stateFilePath.
func newFakeStreamableInfra(stateFilePath string) *fakeStreamableInfra {
	return &fakeStreamableInfra{
		fakeInfra:     newFakeInfra(),
		stateFilePath: stateFilePath,
	}
}

// GetStateFilePath returns the configured path or a configured error.
func (f *fakeStreamableInfra) GetStateFilePath() (string, error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	// return a configured error
	if f.stateFilePathErr != nil {
		return "", f.stateFilePathErr
	}
	// return the configured path
	return f.stateFilePath, nil
}

// ReadStateFile records the call and returns readState and readStateErr.
func (f *fakeStreamableInfra) ReadStateFile() (*datatypes.JSON, error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	f.readCalls++
	return f.readState, f.readStateErr
}

// setStateFilePathErr sets the error GetStateFilePath returns.
func (f *fakeStreamableInfra) setStateFilePathErr(err error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	f.stateFilePathErr = err
}

// setReadState sets the value and error ReadStateFile returns.
func (f *fakeStreamableInfra) setReadState(state *datatypes.JSON, err error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	f.readState = state
	f.readStateErr = err
}

// readStateCallCount returns how many times ReadStateFile has been called.
func (f *fakeStreamableInfra) readStateCallCount() int {
	f.smu.Lock()
	defer f.smu.Unlock()
	return f.readCalls
}

// fakeRefreshableInfra is an in-memory provider with a configurable
// RefreshStack result.
type fakeRefreshableInfra struct {
	*fakeInfra

	rmu          sync.Mutex
	refreshErr   error
	refreshCalls int
}

// newFakeRefreshableInfra returns a refreshable fake that succeeds
// RefreshStack.
func newFakeRefreshableInfra() *fakeRefreshableInfra {
	return &fakeRefreshableInfra{
		fakeInfra: newFakeInfra(),
	}
}

// RefreshStack records the call and returns refreshErr.
func (f *fakeRefreshableInfra) RefreshStack() error {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	f.refreshCalls++
	return f.refreshErr
}

// setRefreshErr sets the error RefreshStack returns.
func (f *fakeRefreshableInfra) setRefreshErr(err error) {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	f.refreshErr = err
}

// refreshCallCount returns how many times RefreshStack has been called.
func (f *fakeRefreshableInfra) refreshCallCount() int {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	return f.refreshCalls
}

// fakeLifecycle is an in-memory InfraLifecycleProvider that records calls
// and serves queued reconciliation snapshots, safe for concurrent use.
type fakeLifecycle struct {
	mu sync.Mutex

	calls map[string]int
	errs  map[string]error

	// The GetReconciliation results, served in order
	snaps []*ReconciliationSnapshot
	// The index of the next snapshot, held on the last
	snapIndex int

	// The InfraProvider BuildInfra returns
	infra InfraProvider
	// The value IsCreateComplete returns
	createComplete bool

	// The arguments passed to SaveState, in call order
	savedStates []*datatypes.JSON
	// The last state passed to SaveCreateOutputs
	createOutputState *datatypes.JSON
}

// newFakeLifecycle returns a fake that serves snaps in order.
func newFakeLifecycle(snaps ...*ReconciliationSnapshot) *fakeLifecycle {
	return &fakeLifecycle{
		calls: make(map[string]int),
		errs:  make(map[string]error),
		snaps: snaps,
		infra: newFakeInfra(),
	}
}

// recordSimple increments the call count for method and returns any
// configured error.
func (f *fakeLifecycle) recordSimple(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[method]++
	return f.errs[method]
}

// GetReconciliation returns queued snapshots in order and holds on
// the last. An empty queue yields an empty snapshot. An error does
// not advance, and the snapshot is not copied.
func (f *fakeLifecycle) GetReconciliation() (*ReconciliationSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// record the call
	f.calls["GetReconciliation"]++
	// return a configured error
	if err := f.errs["GetReconciliation"]; err != nil {
		return nil, err
	}
	// return an empty snapshot when none were queued
	if len(f.snaps) == 0 {
		return &ReconciliationSnapshot{}, nil
	}
	// return the current snapshot and advance until the last, which then repeats
	snap := f.snaps[f.snapIndex]
	if f.snapIndex < len(f.snaps)-1 {
		f.snapIndex++
	}
	return snap, nil
}

// BuildInfra returns the configured provider or a configured error.
func (f *fakeLifecycle) BuildInfra() (InfraProvider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// record the call
	f.calls["BuildInfra"]++
	// return a configured error
	if err := f.errs["BuildInfra"]; err != nil {
		return nil, err
	}
	// return the configured provider
	return f.infra, nil
}

// IsCreateComplete returns createComplete or a configured error.
func (f *fakeLifecycle) IsCreateComplete() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// record the call
	f.calls["IsCreateComplete"]++
	// return a configured error
	if err := f.errs["IsCreateComplete"]; err != nil {
		return false, err
	}
	// return the configured completion flag
	return f.createComplete, nil
}

// OnCreateConfirmed records the call and returns any configured error.
func (f *fakeLifecycle) OnCreateConfirmed(infra InfraProvider) error {
	return f.recordSimple("OnCreateConfirmed")
}

// SaveCreateOutputs records the call and stores state as the create output.
func (f *fakeLifecycle) SaveCreateOutputs(infra InfraProvider, state *datatypes.JSON) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// record the call and store the output state
	f.calls["SaveCreateOutputs"]++
	f.createOutputState = state
	return f.errs["SaveCreateOutputs"]
}

// OnDeleteConfirmed records the call and returns any configured error.
func (f *fakeLifecycle) OnDeleteConfirmed(infra InfraProvider) error {
	return f.recordSimple("OnDeleteConfirmed")
}

// AckCreation records the call and returns any configured error.
func (f *fakeLifecycle) AckCreation() error {
	return f.recordSimple("AckCreation")
}

// RefreshCreationAck records the call and returns any configured error.
func (f *fakeLifecycle) RefreshCreationAck() error {
	return f.recordSimple("RefreshCreationAck")
}

// SetCreationFailed records the call and returns any configured error.
func (f *fakeLifecycle) SetCreationFailed() error {
	return f.recordSimple("SetCreationFailed")
}

// ConfirmCreation records the call and returns any configured error.
func (f *fakeLifecycle) ConfirmCreation() error {
	return f.recordSimple("ConfirmCreation")
}

// AckDeletion records the call and returns any configured error.
func (f *fakeLifecycle) AckDeletion() error {
	return f.recordSimple("AckDeletion")
}

// RefreshDeletionAck records the call and returns any configured error.
func (f *fakeLifecycle) RefreshDeletionAck() error {
	return f.recordSimple("RefreshDeletionAck")
}

// ConfirmDeletion records the call and returns any configured error.
func (f *fakeLifecycle) ConfirmDeletion() error {
	return f.recordSimple("ConfirmDeletion")
}

// SaveState appends state to the saved-state history.
func (f *fakeLifecycle) SaveState(state *datatypes.JSON) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// record the call and append the state
	f.calls["SaveState"]++
	f.savedStates = append(f.savedStates, state)
	return f.errs["SaveState"]
}

// ClearInventory records the call and returns any configured error.
func (f *fakeLifecycle) ClearInventory() error {
	return f.recordSimple("ClearInventory")
}

// PublishCreateNotification records the call and returns any configured error.
func (f *fakeLifecycle) PublishCreateNotification() error {
	return f.recordSimple("PublishCreateNotification")
}

// PublishDeleteNotification records the call and returns any configured error.
func (f *fakeLifecycle) PublishDeleteNotification() error {
	return f.recordSimple("PublishDeleteNotification")
}

// setErr sets the error returned by method. A nil err clears it.
func (f *fakeLifecycle) setErr(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// clear the method's error
	if err == nil {
		delete(f.errs, method)
		return
	}
	// store the method's error
	f.errs[method] = err
}

// setInfra sets the provider BuildInfra returns.
func (f *fakeLifecycle) setInfra(infra InfraProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infra = infra
}

// setCreateComplete sets the value IsCreateComplete returns.
func (f *fakeLifecycle) setCreateComplete(complete bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createComplete = complete
}

// pushSnapshot appends snap to the reconciliation queue.
func (f *fakeLifecycle) pushSnapshot(snap *ReconciliationSnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snaps = append(f.snaps, snap)
}

// callCount returns how many times method has been called.
func (f *fakeLifecycle) callCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

// savedStateHistory returns a new slice of the states passed to SaveState.
func (f *fakeLifecycle) savedStateHistory() []*datatypes.JSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	// copy saved states
	out := make([]*datatypes.JSON, len(f.savedStates))
	copy(out, f.savedStates)
	return out
}

// createOutputs returns the last state passed to SaveCreateOutputs.
func (f *fakeLifecycle) createOutputs() *datatypes.JSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createOutputState
}
