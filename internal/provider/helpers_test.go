package provider

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

// testLifecycleConfig returns a test config with production 240s stale
// threshold and capacity 5, 1h refresh, one persist attempt, 1ms delay.
func testLifecycleConfig() LifecycleConfig {
	return LifecycleConfig{
		StaleAckThreshold: 240 * time.Second,
		RefreshInterval:   time.Hour,
		SemaphoreCapacity: 5,
		PersistRetries:    1,
		PersistRetryDelay: time.Millisecond,
	}
}

// TestCheckStaleAck_Boundary covers the exclusive 240s stale bound:
// an ack aged exactly to the threshold is not stale.
func TestCheckStaleAck_Boundary(t *testing.T) {
	// freeze now at 2026-01-01 12:00 UTC
	clk := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	// install the fake clock for stale checks
	restoreClock := setLifecycleClock(clk)
	t.Cleanup(restoreClock)

	// install the 240s stale threshold
	restoreConfig := setLifecycleConfig(testLifecycleConfig())
	t.Cleanup(restoreConfig)

	// cases straddle the exclusive 240s boundary
	cases := []struct {
		name      string
		ackAge    time.Duration
		wantStale bool
	}{
		{
			name:      "ack aged 239s is not stale",
			ackAge:    239 * time.Second,
			wantStale: false,
		},
		{
			name:      "ack aged exactly 240s is not stale",
			ackAge:    240 * time.Second,
			wantStale: false,
		},
		{
			name:      "ack aged 240s plus 100ms is stale",
			ackAge:    240*time.Second + 100*time.Millisecond,
			wantStale: true,
		},
	}

	// check each age against the frozen now
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// place the ack tc.ackAge before fake now
			ackTimestamp := clk.Now().Add(-tc.ackAge)
			// assert stale matches wantStale
			assert.Equal(t, tc.wantStale, checkStaleAck(ackTimestamp))
		})
	}
}

// TestVerifyState_NilEmptyInvalid rejects nil, empty, and non-JSON state.
func TestVerifyState_NilEmptyInvalid(t *testing.T) {
	// cases for the three reject paths
	cases := []struct {
		name       string
		state      *datatypes.JSON
		wantErrSub string
	}{
		{
			name:       "nil state is rejected",
			state:      nil,
			wantErrSub: "state is nil",
		},
		{
			name:       "empty state is rejected",
			state:      jsonPtr(""),
			wantErrSub: "state is empty",
		},
		{
			name:       "non-JSON bytes are rejected",
			state:      jsonPtr("this is not json {{"),
			wantErrSub: "not valid JSON",
		},
	}

	// reject each invalid state
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// verify the invalid state
			err := verifyState(tc.state, newTestLogger())
			// assert the error names the reject path
			assert.ErrorContains(t, err, tc.wantErrSub)
		})
	}
}

// TestVerifyState_CheckpointFormat_CountsResources accepts a checkpoint
// format state that lists resources.
func TestVerifyState_CheckpointFormat_CountsResources(t *testing.T) {
	// checkpoint.latest.resources with three entries
	state := jsonPtr(`{"checkpoint":{"latest":{"resources":[{"urn":"a"},{"urn":"b"},{"urn":"c"}]}}}`)
	// accept the populated checkpoint state
	assert.NoError(t, verifyState(state, newTestLogger()))
}

// TestVerifyState_DeploymentFormat accepts a deployment format state
// that lists resources.
func TestVerifyState_DeploymentFormat(t *testing.T) {
	// deployment.resources with one entry
	state := jsonPtr(`{"deployment":{"resources":[{"urn":"a"}]}}`)
	// accept the populated deployment state
	assert.NoError(t, verifyState(state, newTestLogger()))
}

// TestVerifyState_NoResources rejects state whose checkpoint and
// deployment resource lists are both empty or absent.
func TestVerifyState_NoResources(t *testing.T) {
	// cases with no countable resources
	cases := []struct {
		name  string
		state *datatypes.JSON
	}{
		{
			name:  "both formats missing",
			state: jsonPtr(`{"other":"content"}`),
		},
		{
			name:  "checkpoint format with empty resources",
			state: jsonPtr(`{"checkpoint":{"latest":{"resources":[]}}}`),
		},
		{
			name:  "deployment format with empty resources",
			state: jsonPtr(`{"deployment":{"resources":[]}}`),
		},
		{
			name:  "both formats present with empty resources",
			state: jsonPtr(`{"checkpoint":{"latest":{"resources":[]}},"deployment":{"resources":[]}}`),
		},
	}

	// reject each empty-resource state
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// verify the empty-resource state
			err := verifyState(tc.state, newTestLogger())
			// assert the error names the missing resources
			assert.ErrorContains(t, err, "state contains no resources")
		})
	}
}

// TestInventoryCleared_Table covers which inventory values count as
// cleared: nil, empty, {}, and null.
func TestInventoryCleared_Table(t *testing.T) {
	// cases for cleared versus populated inventory
	cases := []struct {
		name      string
		inventory *datatypes.JSON
		want      bool
	}{
		{
			name:      "nil inventory is cleared",
			inventory: nil,
			want:      true,
		},
		{
			name:      "zero-length inventory is cleared",
			inventory: jsonPtr(""),
			want:      true,
		},
		{
			name:      "empty object inventory is cleared",
			inventory: jsonPtr("{}"),
			want:      true,
		},
		{
			name:      "json null inventory is cleared",
			inventory: jsonPtr("null"),
			want:      true,
		},
		{
			name:      "populated inventory is not cleared",
			inventory: validStackState(),
			want:      false,
		},
	}

	// check each inventory value
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// assert cleared matches want
			assert.Equal(t, tc.want, inventoryCleared(tc.inventory))
		})
	}
}

// TestHasExistingState_Table covers which state values count as existing
// state: not nil, empty, {}, or null.
func TestHasExistingState_Table(t *testing.T) {
	// cases for existing versus absent state
	cases := []struct {
		name  string
		state *datatypes.JSON
		want  bool
	}{
		{
			name:  "nil state is not existing state",
			state: nil,
			want:  false,
		},
		{
			name:  "zero-length state is not existing state",
			state: jsonPtr(""),
			want:  false,
		},
		{
			name:  "empty object state is not existing state",
			state: jsonPtr("{}"),
			want:  false,
		},
		{
			name:  "json null state is not existing state",
			state: jsonPtr("null"),
			want:  false,
		},
		{
			name:  "populated state is existing state",
			state: validStackState(),
			want:  true,
		},
	}

	// check each state value
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// assert existing-state matches want
			assert.Equal(t, tc.want, hasExistingState(tc.state))
		})
	}
}

// TestPersistFailure_SucceedsFirstTry asserts a successful persist
// returns without waiting the retry delay.
func TestPersistFailure_SucceedsFirstTry(t *testing.T) {
	// allow three attempts so a retry would be possible
	config := testLifecycleConfig()
	config.PersistRetries = 3
	// set a 10s retry delay so a wait would exceed the 1s bound
	config.PersistRetryDelay = 10 * time.Second
	// install the three-attempt, 10s-delay config
	restore := setLifecycleConfig(config)
	t.Cleanup(restore)

	// persist succeeds on the first call
	calls := 0
	persist := func() error {
		calls++
		return nil
	}

	// call persistFailure and time it
	start := time.Now()
	persistFailure(persist, newTestLogger())
	elapsed := time.Since(start)

	// assert one call and no retry-delay wait
	assert.Equal(t, 1, calls)
	assert.Less(t, elapsed, time.Second,
		"first-try success must return without waiting the retry delay")
}

// TestPersistFailure_Exhaustion asserts a failing persist is attempted
// three times and then returns.
func TestPersistFailure_Exhaustion(t *testing.T) {
	// allow three attempts with a 1ms delay
	config := testLifecycleConfig()
	config.PersistRetries = 3
	config.PersistRetryDelay = time.Millisecond
	restore := setLifecycleConfig(config)
	t.Cleanup(restore)

	// persist fails on every call
	calls := 0
	persist := func() error {
		calls++
		return errors.New("persist always fails")
	}

	// exhaust persistFailure
	persistFailure(persist, newTestLogger())

	// assert three attempts and no more
	assert.Equal(t, 3, calls)
}

// TestDefaultLifecycleConfig_ProductionValues asserts the production
// lifecycle default durations, capacity, and persist budget.
func TestDefaultLifecycleConfig_ProductionValues(t *testing.T) {
	// assert production defaults
	assert.Equal(t, 240*time.Second, defaultLifecycleConfig.StaleAckThreshold)
	assert.Equal(t, 60*time.Second, defaultLifecycleConfig.RefreshInterval)
	assert.Equal(t, 5, defaultLifecycleConfig.SemaphoreCapacity)
	assert.Equal(t, 30, defaultLifecycleConfig.PersistRetries)
	assert.Equal(t, 10*time.Second, defaultLifecycleConfig.PersistRetryDelay)
}

// TestInfraSemaphore_CapacityMatchesDefaultConfig asserts the live
// semaphore capacity matches the production default.
func TestInfraSemaphore_CapacityMatchesDefaultConfig(t *testing.T) {
	// assert live semaphore capacity matches the default
	assert.Equal(t, defaultLifecycleConfig.SemaphoreCapacity, cap(currentSemaphore()))
}

// TestCheckStaleAck_RealClock covers stale detection on the real clock:
// an ack taken now is fresh, one older than the threshold is stale.
func TestCheckStaleAck_RealClock(t *testing.T) {
	// use the real wall clock
	restoreClock := setLifecycleClock(realClock{})
	t.Cleanup(restoreClock)

	// install the 240s stale threshold
	cfg := testLifecycleConfig()
	restoreConfig := setLifecycleConfig(cfg)
	t.Cleanup(restoreConfig)

	// an ack taken now is not stale
	assert.False(
		t,
		checkStaleAck(time.Now().UTC()),
		"an ack taken just now is not stale",
	)
	// an ack older than the threshold is stale
	assert.True(
		t,
		checkStaleAck(time.Now().UTC().Add(-cfg.StaleAckThreshold-time.Second)),
		"an ack older than the threshold is stale",
	)
}

// TestCheckStaleAck_AdvancingClock covers one ack as a frozen clock
// advances through the exclusive 240s threshold, re-reading now
// on every call.
func TestCheckStaleAck_AdvancingClock(t *testing.T) {
	// freeze now at 2026-01-01 12:00 UTC
	clk := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	// install the fake clock for stale checks
	restoreClock := setLifecycleClock(clk)
	t.Cleanup(restoreClock)

	// install the 240s stale threshold
	cfg := testLifecycleConfig()
	restoreConfig := setLifecycleConfig(cfg)
	t.Cleanup(restoreConfig)

	// a fresh ack is not stale
	ack := clk.Now()
	assert.False(t, checkStaleAck(ack), "a fresh ack is not stale")

	// advancing exactly to the threshold is still not stale
	clk.Advance(cfg.StaleAckThreshold)
	assert.False(t, checkStaleAck(ack), "an ack aged exactly to the threshold is not stale")

	// advancing past the threshold makes it stale
	clk.Advance(time.Second)
	assert.True(t, checkStaleAck(ack), "an ack aged past the threshold is stale")
}

// TestSetLifecycleConfig_ZeroFieldsFallBackToDefaults asserts a zero
// config takes the production defaults, including semaphore capacity.
func TestSetLifecycleConfig_ZeroFieldsFallBackToDefaults(t *testing.T) {
	// install an all-zero config
	restore := setLifecycleConfig(LifecycleConfig{})
	t.Cleanup(restore)

	// assert config and semaphore match production defaults
	assert.Equal(t, defaultLifecycleConfig, currentConfig())
	assert.Equal(t, defaultLifecycleConfig.SemaphoreCapacity, cap(currentSemaphore()))
}

// TestSetLifecycleConfig_SetFieldsSurvive asserts the set threshold and
// capacity stick and the unset fields take the production defaults.
func TestSetLifecycleConfig_SetFieldsSurvive(t *testing.T) {
	// install a config with only threshold and capacity set
	restore := setLifecycleConfig(LifecycleConfig{
		StaleAckThreshold: time.Second,
		SemaphoreCapacity: 2,
	})
	t.Cleanup(restore)

	// assert set fields stick and the rest fall back
	got := currentConfig()
	assert.Equal(t, time.Second, got.StaleAckThreshold)
	assert.Equal(t, 2, got.SemaphoreCapacity)
	assert.Equal(t, 2, cap(currentSemaphore()))
	assert.Equal(t, defaultLifecycleConfig.RefreshInterval, got.RefreshInterval)
	assert.Equal(t, defaultLifecycleConfig.PersistRetries, got.PersistRetries)
	assert.Equal(t, defaultLifecycleConfig.PersistRetryDelay, got.PersistRetryDelay)
}
