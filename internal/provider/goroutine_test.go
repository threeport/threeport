package provider

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// fastRefreshConfig returns a lifecycle config whose refresh interval is
// 5 milliseconds so a test can observe ticks in milliseconds.
func fastRefreshConfig() LifecycleConfig {
	return LifecycleConfig{
		StaleAckThreshold: 240 * time.Second,
		RefreshInterval:   5 * time.Millisecond,
		SemaphoreCapacity: 5,
		PersistRetries:    1,
		PersistRetryDelay: time.Millisecond,
	}
}

// TestRefreshAck_QuitClean covers a quit signal stopping further refresh
// calls after the loop has ticked.
func TestRefreshAck_QuitClean(t *testing.T) {
	// set a short refresh interval
	restore := setLifecycleConfig(fastRefreshConfig())
	t.Cleanup(restore)

	// count refresh calls
	var calls int64
	refresh := func() error {
		atomic.AddInt64(&calls, 1)
		return nil
	}

	// run refreshAck in a goroutine
	quit := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshAck(refresh, quit, newTestLogger())
	}()

	// wait for at least two ticks before quit
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&calls) >= 2
	}, 10*time.Second, time.Millisecond, "refreshAck should tick on the refresh interval")

	// send quit and wait for return
	quit <- true
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("refreshAck did not return after quit signal")
	}

	// assert refresh calls stay settled across many refresh intervals
	settled := atomic.LoadInt64(&calls)
	assert.Never(t, func() bool {
		return atomic.LoadInt64(&calls) != settled
	}, 100*time.Millisecond, 5*time.Millisecond, "refresh calls must stop after refreshAck returns")
}

// TestRefreshAck_RefreshErrorDoesNotBlock covers refresh errors leaving
// the tick loop running.
func TestRefreshAck_RefreshErrorDoesNotBlock(t *testing.T) {
	// set a short refresh interval
	restore := setLifecycleConfig(fastRefreshConfig())
	t.Cleanup(restore)

	// fail the first three refresh calls
	var calls int64
	refresh := func() error {
		if atomic.AddInt64(&calls, 1) <= 3 {
			return errFakeInfra
		}
		return nil
	}

	// run refreshAck in a goroutine
	quit := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshAck(refresh, quit, newTestLogger())
	}()

	// wait for ticks past the failing calls
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&calls) >= 6
	}, 10*time.Second, time.Millisecond, "refreshAck should keep ticking through refresh errors")

	// send quit and wait for return
	quit <- true
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("refreshAck did not return after quit signal")
	}
}

// streamHarness is a test fixture that runs a stream loop against a fake
// streamable provider and records each saved state.
type streamHarness struct {
	t     *testing.T
	infra *fakeStreamableInfra
	// The state file path
	path string

	mu sync.Mutex
	// The states received by a save callback
	saved []*datatypes.JSON

	// The quit signal for the stream loop
	quit chan bool
	// The channel closed when the stream loop returns
	done chan struct{}
}

// newStreamHarness returns a streamHarness with a nested state file path
// under a temp directory.
func newStreamHarness(t *testing.T) *streamHarness {
	t.Helper()
	// derive a nested state file path under a temp dir
	ws := NewPulumiWorkspace("test-instance", "test-project", WithStateDirRoot(t.TempDir()))
	path, err := ws.GetStateFilePath()
	require.NoError(t, err)
	// return a harness pointing at that path
	return &streamHarness{
		t:     t,
		infra: newFakeStreamableInfra(path),
		path:  path,
		quit:  make(chan bool, 1),
		done:  make(chan struct{}),
	}
}

// start launches the stream loop and registers a non-blocking quit
// cleanup so a failed test does not leak the goroutine.
func (h *streamHarness) start() {
	// launch streamState
	go func() {
		defer close(h.done)
		streamState(h.infra, h.saveState, h.quit, newTestLogger())
	}()
	// send quit on cleanup if the test did not
	h.t.Cleanup(func() {
		select {
		case h.quit <- true:
		default:
		}
	})
}

// saveState records each saved state for later assertions.
func (h *streamHarness) saveState(state *datatypes.JSON) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.saved = append(h.saved, state)
	return nil
}

// saveCount returns the number of saved states.
func (h *streamHarness) saveCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.saved)
}

// lastSaved returns the most recent saved state, or nil if none.
func (h *streamHarness) lastSaved() *datatypes.JSON {
	h.mu.Lock()
	defer h.mu.Unlock()
	// return nil when nothing has been saved
	if len(h.saved) == 0 {
		return nil
	}
	// return the most recent saved state
	return h.saved[len(h.saved)-1]
}

// writeStateFile writes the watched state file to fire a watcher event.
// It uses assert so a failure does not FailNow off the test goroutine.
func (h *streamHarness) writeStateFile(content string) {
	// create the state file directory
	if !assert.NoError(h.t, os.MkdirAll(filepath.Dir(h.path), 0755)) {
		return
	}
	// write the state file
	assert.NoError(h.t, os.WriteFile(h.path, []byte(content), 0644))
}

// writeSiblingFile writes content to a non-state file in the same
// directory as the watched state file.
func (h *streamHarness) writeSiblingFile(content string) {
	sibling := filepath.Join(filepath.Dir(h.path), "sibling.json")
	// create the state file directory
	if !assert.NoError(h.t, os.MkdirAll(filepath.Dir(h.path), 0755)) {
		return
	}
	// write a sibling file in the same directory
	assert.NoError(h.t, os.WriteFile(sibling, []byte(content), 0644))
}

// sendQuit signals the stream loop to return.
func (h *streamHarness) sendQuit() {
	h.quit <- true
}

// waitDone fails the test if the stream loop does not return in time.
func (h *streamHarness) waitDone(deadline time.Duration) {
	h.t.Helper()
	// wait for streamState to return
	select {
	case <-h.done:
	case <-time.After(deadline):
		h.t.Fatal("streamState did not return before deadline")
	}
}

// TestStreamState_ValidJSON_SavesImmediately covers a valid state file
// write triggering a save.
func TestStreamState_ValidJSON_SavesImmediately(t *testing.T) {
	h := newStreamHarness(t)
	// seed a valid canned read result
	want := validStackState()
	h.infra.setReadState(want, nil)

	// start streamState
	h.start()

	// wait for the state directory before writing
	stateDir := filepath.Dir(h.path)
	require.Eventually(t, func() bool {
		_, err := os.Stat(stateDir)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "streamState should create the state directory")

	// write until a save lands since the watch is not observable
	require.Eventually(t, func() bool {
		h.writeStateFile(string(*want))
		return h.saveCount() >= 1
	}, 10*time.Second, 50*time.Millisecond, "state file write should trigger a save")

	// assert the saved bytes match the canned read
	got := h.lastSaved()
	require.NotNil(t, got)
	assert.Equal(t, string(*want), string(*got), "saved state must match the bytes read from the state file")
	// assert the write triggered a read
	assert.GreaterOrEqual(t, h.infra.readStateCallCount(), 1, "state file event should trigger a read")

	// send quit and wait for return
	h.sendQuit()
	h.waitDone(5 * time.Second)
}

// TestStreamState_PartialJSON_Skipped covers invalid JSON being skipped
// and a later valid write still saving.
func TestStreamState_PartialJSON_Skipped(t *testing.T) {
	h := newStreamHarness(t)
	// seed a partial JSON canned read
	partial := jsonPtr(`{"deployment":{"resources":[`)
	h.infra.setReadState(partial, nil)

	// start streamState
	h.start()

	// write the partial file until a read is recorded
	require.Eventually(t, func() bool {
		h.writeStateFile(string(*partial))
		return h.infra.readStateCallCount() >= 1
	}, 10*time.Second, 50*time.Millisecond, "state file write should trigger a read")

	// assert no save was recorded
	assert.Never(t, func() bool {
		return h.saveCount() > 0
	}, 300*time.Millisecond, 20*time.Millisecond, "partial JSON must not be saved")

	// seed a valid canned read
	want := validStackState()
	h.infra.setReadState(want, nil)
	// write a valid file to prove the loop survived the skip
	require.Eventually(t, func() bool {
		h.writeStateFile(string(*want))
		return h.saveCount() >= 1
	}, 10*time.Second, 50*time.Millisecond, "valid state after a skipped partial write should save")

	// send quit and wait for return
	h.sendQuit()
	h.waitDone(5 * time.Second)
}

// TestStreamState_QuitOrdering_NoLateWrite covers a state file write
// after quit being ignored.
func TestStreamState_QuitOrdering_NoLateWrite(t *testing.T) {
	h := newStreamHarness(t)
	// seed a valid canned read
	want := validStackState()
	h.infra.setReadState(want, nil)

	// start streamState
	h.start()

	// write until a save lands so quit hits a live watch
	require.Eventually(t, func() bool {
		h.writeStateFile(string(*want))
		return h.saveCount() >= 1
	}, 10*time.Second, 50*time.Millisecond, "state file write should trigger a save before quit")

	// send quit and wait for return
	h.sendQuit()
	h.waitDone(5 * time.Second)

	// write the state file after quit
	settled := h.saveCount()
	h.writeStateFile(string(*want))
	// assert no further save is recorded
	assert.Never(t, func() bool {
		return h.saveCount() > settled
	}, 500*time.Millisecond, 25*time.Millisecond, "no save may occur after quit was honored")
}

// TestStreamState_NonStateFileEvent_Ignored covers a sibling file write
// being ignored while a later state file write still saves.
func TestStreamState_NonStateFileEvent_Ignored(t *testing.T) {
	h := newStreamHarness(t)
	// seed a valid canned read
	want := validStackState()
	h.infra.setReadState(want, nil)

	// start streamState
	h.start()

	// write a sibling file and assert no reads or saves
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		h.writeSiblingFile(`{"unrelated":true}`)
		require.Zero(t, h.infra.readStateCallCount(), "sibling file events must not trigger state file reads")
		require.Zero(t, h.saveCount(), "sibling file events must not trigger saves")
		time.Sleep(25 * time.Millisecond)
	}

	// write the state file to prove the loop stayed live
	require.Eventually(t, func() bool {
		h.writeStateFile(string(*want))
		return h.saveCount() >= 1
	}, 10*time.Second, 50*time.Millisecond, "state file write should still save after sibling events")

	// send quit and wait for return
	h.sendQuit()
	h.waitDone(5 * time.Second)
}
