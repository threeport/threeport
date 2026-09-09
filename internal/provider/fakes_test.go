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

// compile-time interface satisfaction checks for all fakes.
var (
	_ InfraLifecycleProvider = (*fakeLifecycle)(nil)
	_ InfraProvider          = (*fakeInfra)(nil)
	_ StreamableProvider     = (*fakeStreamableInfra)(nil)
	_ RefreshableProvider    = (*fakeRefreshableInfra)(nil)
	_ Clock                  = (*fakeClock)(nil)
)

// errFakeInfra is the fallback error returned by infra fakes when a method
// is set to fail but no specific error was injected.
var errFakeInfra = errors.New("fakeInfra: injected failure")

// baselineGoroutines holds the goroutine count captured before any test
// runs, printed with a leak so the process-wide delta is in the output.
var baselineGoroutines int

// goroutineDrainTimeout bounds the post-suite wait for background
// goroutines to exit. The lifecycle spawns refresh-ack and state-stream
// goroutines that stop on a channel close, so they exit promptly; the
// window only covers scheduling.
const goroutineDrainTimeout = 10 * time.Second

// TestMain runs the suite, then fails when a lifecycle goroutine is still
// running. Runtime and testing leftovers are ignored.
func TestMain(m *testing.M) {
	baselineGoroutines = runtime.NumGoroutine()

	code := m.Run()

	// only report leaks on an otherwise green run: a failed test may have
	// returned early and left its own goroutines behind, and that error is
	// the one worth reading
	if code == 0 {
		if leaked := lifecycleGoroutinesRemaining(goroutineDrainTimeout); leaked > 0 {
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

	os.Exit(code)
}

// lifecycleGoroutineNames are the functions this package starts in the
// background. The post-suite check matches these in stacks, not the process count.
var lifecycleGoroutineNames = []string{
	"provider.refreshAck",
	"provider.streamState",
	"provider.executeInfraCreate",
	"provider.executeInfraDelete",
	"provider.launchInfraCreate",
	"provider.launchInfraDelete",
}

// lifecycleGoroutineStacks returns the current goroutine dump, used when
// the leak check fails so the leftover function names are in the output.
func lifecycleGoroutineStacks() string {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// countLifecycleGoroutines returns how many running goroutines are in a
// lifecycle function this package started.
func countLifecycleGoroutines() int {
	count := 0
	for _, g := range strings.Split(lifecycleGoroutineStacks(), "\n\n") {
		for _, name := range lifecycleGoroutineNames {
			if strings.Contains(g, name) {
				count++
				break
			}
		}
	}
	return count
}

// lifecycleGoroutinesRemaining polls until no lifecycle goroutine remains
// and returns 0, or returns how many remain once the timeout expires.
func lifecycleGoroutinesRemaining(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		left := countLifecycleGoroutines()
		if left == 0 {
			return 0
		}
		if time.Now().After(deadline) {
			return left
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// newTestLogger returns a pointer to a discard logger matching the
// *logr.Logger parameter shape the lifecycle handlers take.
func newTestLogger() *logr.Logger {
	l := logr.Discard()
	return &l
}

// jsonPtr returns a pointer to the given string as a JSON value, for
// building resource inventories inline.
func jsonPtr(s string) *datatypes.JSON {
	j := datatypes.JSON(s)
	return &j
}

// validStackState returns a state JSON in deployment format with one
// resource, sufficient to pass post-create state verification.
func validStackState() *datatypes.JSON {
	return jsonPtr(`{"deployment":{"resources":[{"urn":"urn:fake:resource"}]}}`)
}

// fakeClock implements Clock with a fixed, advanceable time.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock returns a clock frozen at the given time.
func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

// Now returns the clock's current frozen time.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by the given duration.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// infraMode selects the behavior of a fakeInfra deploy or destroy call.
type infraMode int

const (
	// infraSucceed makes the call return nil immediately.
	infraSucceed infraMode = iota

	// infraError makes the call return the injected error, or
	// errFakeInfra when none was injected.
	infraError

	// infraPanic makes the call panic, exercising the recover path in
	// the launch goroutines.
	infraPanic

	// infraBlock makes the call block until released, so a test can hold
	// a semaphore slot open and observe backpressure.
	infraBlock
)

// fakeInfra implements InfraProvider with per-method programmable
// behavior and call counters. Safe for concurrent use.
type fakeInfra struct {
	mu sync.Mutex

	deployMode infraMode
	deployErr  error

	destroyMode infraMode
	destroyErr  error

	deployRelease   chan struct{}
	deployReleased  bool
	destroyRelease  chan struct{}
	destroyReleased bool

	deployCalls   int
	destroyCalls  int
	setStateCalls int
	getStateCalls int

	restoredStates []*datatypes.JSON
	setStateErr    error

	stackState  *datatypes.JSON
	getStateErr error
}

// newFakeInfra returns an infra fake whose deploy and destroy succeed
// immediately and whose stack state defaults to a valid one-resource
// deployment so the create success path completes.
func newFakeInfra() *fakeInfra {
	return &fakeInfra{
		deployRelease:  make(chan struct{}),
		destroyRelease: make(chan struct{}),
		stackState:     validStackState(),
	}
}

// DeployInfra runs the programmed deploy behavior.
func (f *fakeInfra) DeployInfra() error {
	f.mu.Lock()
	f.deployCalls++
	mode := f.deployMode
	err := f.deployErr
	release := f.deployRelease
	f.mu.Unlock()

	return runInfraMode(mode, err, release, "fakeInfra: deploy panic")
}

// DestroyInfra runs the programmed destroy behavior.
func (f *fakeInfra) DestroyInfra() error {
	f.mu.Lock()
	f.destroyCalls++
	mode := f.destroyMode
	err := f.destroyErr
	release := f.destroyRelease
	f.mu.Unlock()

	return runInfraMode(mode, err, release, "fakeInfra: destroy panic")
}

// runInfraMode executes the shared mode dispatch for deploy and destroy.
func runInfraMode(
	mode infraMode,
	err error,
	release chan struct{},
	panicMsg string,
) error {
	switch mode {
	case infraError:
		if err != nil {
			return err
		}
		return errFakeInfra
	case infraPanic:
		panic(panicMsg)
	case infraBlock:
		<-release
		return nil
	}
	return nil
}

// SetStackState records the restored state and returns the injected error.
func (f *fakeInfra) SetStackState(state *datatypes.JSON) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setStateCalls++
	f.restoredStates = append(f.restoredStates, state)
	return f.setStateErr
}

// GetStackState returns the programmed stack state and error.
func (f *fakeInfra) GetStackState() (*datatypes.JSON, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getStateCalls++
	return f.stackState, f.getStateErr
}

// setDeploy programs the deploy mode and, for infraError, the error
// returned.
func (f *fakeInfra) setDeploy(mode infraMode, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deployMode = mode
	f.deployErr = err
}

// setDestroy programs the destroy mode and, for infraError, the error
// returned.
func (f *fakeInfra) setDestroy(mode infraMode, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyMode = mode
	f.destroyErr = err
}

// setGetStackState programs the state and error returned when the
// lifecycle captures stack state.
func (f *fakeInfra) setGetStackState(state *datatypes.JSON, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stackState = state
	f.getStateErr = err
}

// setSetStackStateErr programs the error returned when the lifecycle
// restores stack state.
func (f *fakeInfra) setSetStackStateErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setStateErr = err
}

// releaseDeploy unblocks a deploy call in infraBlock mode. Idempotent.
func (f *fakeInfra) releaseDeploy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.deployReleased {
		close(f.deployRelease)
		f.deployReleased = true
	}
}

// releaseDestroy unblocks a destroy call in infraBlock mode. Idempotent.
func (f *fakeInfra) releaseDestroy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.destroyReleased {
		close(f.destroyRelease)
		f.destroyReleased = true
	}
}

// deployCallCount returns the number of deploy invocations.
func (f *fakeInfra) deployCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deployCalls
}

// destroyCallCount returns the number of destroy invocations.
func (f *fakeInfra) destroyCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.destroyCalls
}

// setStackStateCallCount returns the number of state-restore invocations.
func (f *fakeInfra) setStackStateCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setStateCalls
}

// getStackStateCallCount returns the number of state-capture invocations.
func (f *fakeInfra) getStackStateCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getStateCalls
}

// lastRestoredState returns the state passed to the most recent
// state-restore call, or nil if none occurred.
func (f *fakeInfra) lastRestoredState() *datatypes.JSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.restoredStates) == 0 {
		return nil
	}
	return f.restoredStates[len(f.restoredStates)-1]
}

// fakeStreamableInfra implements StreamableProvider by embedding
// fakeInfra and adding programmable state file path and read behavior.
type fakeStreamableInfra struct {
	*fakeInfra

	smu              sync.Mutex
	stateFilePath    string
	stateFilePathErr error
	readState        *datatypes.JSON
	readStateErr     error
	readCalls        int
}

// newFakeStreamableInfra returns a streamable infra fake reporting the
// given state file path. Pass a path under t.TempDir() so the stream
// watcher has a real directory to watch.
func newFakeStreamableInfra(stateFilePath string) *fakeStreamableInfra {
	return &fakeStreamableInfra{
		fakeInfra:     newFakeInfra(),
		stateFilePath: stateFilePath,
	}
}

// GetStateFilePath returns the programmed path and error.
func (f *fakeStreamableInfra) GetStateFilePath() (string, error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	if f.stateFilePathErr != nil {
		return "", f.stateFilePathErr
	}
	return f.stateFilePath, nil
}

// ReadStateFile returns the programmed state and error.
func (f *fakeStreamableInfra) ReadStateFile() (*datatypes.JSON, error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	f.readCalls++
	return f.readState, f.readStateErr
}

// setStateFilePathErr programs an error for state file path lookups.
func (f *fakeStreamableInfra) setStateFilePathErr(err error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	f.stateFilePathErr = err
}

// setReadState programs the state and error returned by state file reads.
func (f *fakeStreamableInfra) setReadState(state *datatypes.JSON, err error) {
	f.smu.Lock()
	defer f.smu.Unlock()
	f.readState = state
	f.readStateErr = err
}

// readStateCallCount returns the number of state file read invocations.
func (f *fakeStreamableInfra) readStateCallCount() int {
	f.smu.Lock()
	defer f.smu.Unlock()
	return f.readCalls
}

// fakeRefreshableInfra implements RefreshableProvider by embedding
// fakeInfra and adding programmable refresh behavior.
type fakeRefreshableInfra struct {
	*fakeInfra

	rmu          sync.Mutex
	refreshErr   error
	refreshCalls int
}

// newFakeRefreshableInfra returns a refreshable infra fake whose refresh
// succeeds by default.
func newFakeRefreshableInfra() *fakeRefreshableInfra {
	return &fakeRefreshableInfra{
		fakeInfra: newFakeInfra(),
	}
}

// RefreshStack returns the programmed refresh error.
func (f *fakeRefreshableInfra) RefreshStack() error {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	f.refreshCalls++
	return f.refreshErr
}

// setRefreshErr programs the error returned by stack refreshes.
func (f *fakeRefreshableInfra) setRefreshErr(err error) {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	f.refreshErr = err
}

// refreshCallCount returns the number of refresh invocations.
func (f *fakeRefreshableInfra) refreshCallCount() int {
	f.rmu.Lock()
	defer f.rmu.Unlock()
	return f.refreshCalls
}

// fakeLifecycle implements InfraLifecycleProvider with per-method call
// counters, per-method error injection, and a programmable sequence of
// reconciliation snapshots. Safe for concurrent use.
type fakeLifecycle struct {
	mu sync.Mutex

	calls map[string]int
	errs  map[string]error

	snaps     []*ReconciliationSnapshot
	snapIndex int

	infra          InfraProvider
	createComplete bool

	savedStates       []*datatypes.JSON
	createOutputState *datatypes.JSON
}

// newFakeLifecycle returns a lifecycle fake that walks the given
// reconciliation snapshots in order: each call consumes the next
// snapshot, and once exhausted the last snapshot repeats. With no
// snapshots, an empty snapshot (a brand new create request) repeats.
// The built infra defaults to a fresh fakeInfra.
func newFakeLifecycle(snaps ...*ReconciliationSnapshot) *fakeLifecycle {
	return &fakeLifecycle{
		calls: make(map[string]int),
		errs:  make(map[string]error),
		snaps: snaps,
		infra: newFakeInfra(),
	}
}

// recordSimple counts a call and returns its injected error, if any.
func (f *fakeLifecycle) recordSimple(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[method]++
	return f.errs[method]
}

// GetReconciliation counts the call and returns the next snapshot in the
// programmed sequence. An injected error is returned without advancing
// the sequence. Callers must not mutate returned snapshots.
func (f *fakeLifecycle) GetReconciliation() (*ReconciliationSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["GetReconciliation"]++
	if err := f.errs["GetReconciliation"]; err != nil {
		return nil, err
	}
	if len(f.snaps) == 0 {
		return &ReconciliationSnapshot{}, nil
	}
	snap := f.snaps[f.snapIndex]
	if f.snapIndex < len(f.snaps)-1 {
		f.snapIndex++
	}
	return snap, nil
}

// BuildInfra counts the call and returns the configured infra, or the
// injected error.
func (f *fakeLifecycle) BuildInfra() (InfraProvider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["BuildInfra"]++
	if err := f.errs["BuildInfra"]; err != nil {
		return nil, err
	}
	return f.infra, nil
}

// IsCreateComplete counts the call and returns the programmed completion
// flag, or the injected error.
func (f *fakeLifecycle) IsCreateComplete() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["IsCreateComplete"]++
	if err := f.errs["IsCreateComplete"]; err != nil {
		return false, err
	}
	return f.createComplete, nil
}

// OnCreateConfirmed counts the call and returns its injected error.
func (f *fakeLifecycle) OnCreateConfirmed(infra InfraProvider) error {
	return f.recordSimple("OnCreateConfirmed")
}

// SaveCreateOutputs counts the call, records the final state, and
// returns its injected error.
func (f *fakeLifecycle) SaveCreateOutputs(infra InfraProvider, state *datatypes.JSON) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["SaveCreateOutputs"]++
	f.createOutputState = state
	return f.errs["SaveCreateOutputs"]
}

// OnDeleteConfirmed counts the call and returns its injected error.
func (f *fakeLifecycle) OnDeleteConfirmed(infra InfraProvider) error {
	return f.recordSimple("OnDeleteConfirmed")
}

// AckCreation counts the call and returns its injected error.
func (f *fakeLifecycle) AckCreation() error {
	return f.recordSimple("AckCreation")
}

// RefreshCreationAck counts the call and returns its injected error.
func (f *fakeLifecycle) RefreshCreationAck() error {
	return f.recordSimple("RefreshCreationAck")
}

// SetCreationFailed counts the call and returns its injected error.
func (f *fakeLifecycle) SetCreationFailed() error {
	return f.recordSimple("SetCreationFailed")
}

// ConfirmCreation counts the call and returns its injected error.
func (f *fakeLifecycle) ConfirmCreation() error {
	return f.recordSimple("ConfirmCreation")
}

// AckDeletion counts the call and returns its injected error.
func (f *fakeLifecycle) AckDeletion() error {
	return f.recordSimple("AckDeletion")
}

// RefreshDeletionAck counts the call and returns its injected error.
func (f *fakeLifecycle) RefreshDeletionAck() error {
	return f.recordSimple("RefreshDeletionAck")
}

// ConfirmDeletion counts the call and returns its injected error.
func (f *fakeLifecycle) ConfirmDeletion() error {
	return f.recordSimple("ConfirmDeletion")
}

// SaveState counts the call, appends the state to the saved history, and
// returns its injected error.
func (f *fakeLifecycle) SaveState(state *datatypes.JSON) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["SaveState"]++
	f.savedStates = append(f.savedStates, state)
	return f.errs["SaveState"]
}

// ClearInventory counts the call and returns its injected error.
func (f *fakeLifecycle) ClearInventory() error {
	return f.recordSimple("ClearInventory")
}

// PublishCreateNotification counts the call and returns its injected error.
func (f *fakeLifecycle) PublishCreateNotification() error {
	return f.recordSimple("PublishCreateNotification")
}

// PublishDeleteNotification counts the call and returns its injected error.
func (f *fakeLifecycle) PublishDeleteNotification() error {
	return f.recordSimple("PublishDeleteNotification")
}

// setErr injects an error for the named interface method; passing nil
// clears the injection.
func (f *fakeLifecycle) setErr(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.errs, method)
		return
	}
	f.errs[method] = err
}

// setInfra replaces the infra returned when the lifecycle builds one.
func (f *fakeLifecycle) setInfra(infra InfraProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infra = infra
}

// setCreateComplete programs the completion flag for create checks.
func (f *fakeLifecycle) setCreateComplete(complete bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createComplete = complete
}

// pushSnapshot appends a snapshot to the programmed sequence so tests
// can extend the walk mid-flight.
func (f *fakeLifecycle) pushSnapshot(snap *ReconciliationSnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snaps = append(f.snaps, snap)
}

// callCount returns how many times the named interface method was called.
func (f *fakeLifecycle) callCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

// savedStateHistory returns a copy of all states passed to SaveState in
// call order.
func (f *fakeLifecycle) savedStateHistory() []*datatypes.JSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*datatypes.JSON, len(f.savedStates))
	copy(out, f.savedStates)
	return out
}

// createOutputs returns the state passed to the most recent
// SaveCreateOutputs call, or nil if none occurred.
func (f *fakeLifecycle) createOutputs() *datatypes.JSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createOutputState
}
