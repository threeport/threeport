package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeClock is a clock a test advances by hand.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newTrackerWithClock() (*FailureTracker, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return &FailureTracker{Now: clock.Now}, clock
}

var testKey = failureKey{reconciler: "r", objectType: "t", objectID: 1, operation: "Updated"}

func TestFailureTracker_WindowStartsAtFirstFailure(t *testing.T) {
	tracker, clock := newTrackerWithClock()

	assert.Equal(t, time.Duration(0), tracker.record(testKey), "the attempt that begins an episode has been failing for no time")

	clock.advance(10 * time.Minute)
	assert.Equal(t, 10*time.Minute, tracker.record(testKey))

	clock.advance(DefaultMaxReconcileDuration)
	assert.Greater(t, tracker.record(testKey), DefaultMaxReconcileDuration)
}

func TestFailureTracker_ClearStartsANewEpisode(t *testing.T) {
	tracker, clock := newTrackerWithClock()
	tracker.record(testKey)
	clock.advance(45 * time.Minute)
	tracker.record(testKey)

	tracker.clear(testKey)

	clock.advance(time.Minute)
	assert.Equal(t, time.Duration(0), tracker.record(testKey), "a failure after a success is a new episode")
}

func TestFailureTracker_LongGapWithinAnEpisodeKeepsItsStart(t *testing.T) {
	tracker, clock := newTrackerWithClock()
	tracker.record(testKey)

	// an hour with no attempt, as when a controller is overloaded: still the
	// same failure, and long enough that it has outlasted the bound
	clock.advance(DefaultMaxReconcileDuration + time.Minute)

	assert.Greater(t, tracker.record(testKey), DefaultMaxReconcileDuration)
}

func TestFailureTracker_EpisodeWithNoFailureForTooLongIsOver(t *testing.T) {
	tracker, clock := newTrackerWithClock()
	tracker.record(testKey)

	clock.advance(episodeStaleAfter + time.Minute)

	assert.Equal(t, time.Duration(0), tracker.record(testKey),
		"a failure long after the last one is not part of the old episode")
}

func TestFailureTracker_EpisodesAreIndependent(t *testing.T) {
	tracker, clock := newTrackerWithClock()
	other := testKey
	other.operation = "Created"

	tracker.record(testKey)
	clock.advance(30 * time.Minute)

	assert.Equal(t, time.Duration(0), tracker.record(other), "another operation on the same object has its own window")
	assert.Equal(t, 30*time.Minute, tracker.record(testKey))
}

func TestFailureTracker_SweepDropsEndedEpisodes(t *testing.T) {
	tracker, clock := newTrackerWithClock()
	tracker.record(testKey)

	clock.advance(episodeStaleAfter + time.Minute)
	other := testKey
	other.objectID = 2
	tracker.record(other)

	assert.Len(t, tracker.episodes, 1, "an episode that ended without being cleared must not be kept forever")
}
