package controller

import (
	"sync"
	"time"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
)

// FailureTracker records when each operation on each object began failing, so a
// reconciler can tell how long a failure has gone on. The time is the first
// failed attempt, not when the notification was published: a notification can
// wait in the queue for any length of time - a controller that was down, a
// backlog - and that wait is not time spent failing. A tracker also lets every
// worker in a controller share one account of an episode, where the redelivered
// notification may be picked up by a different one each time.
//
// State is held in memory. A restart forgets the episodes, which gives a
// failing operation another full window rather than a shorter one - the safe
// direction for something that stops retrying.
//
// The zero value is ready to use.
type FailureTracker struct {
	// Now returns the current time. Set by tests; nil means time.Now.
	Now func() time.Time

	mu        sync.Mutex
	episodes  map[failureKey]failureEpisode
	lastSweep time.Time
}

// episodeStaleAfter is how long an episode can go with no failure before it is
// treated as over. Attempts at a failing operation come at most a requeue delay
// apart, so a gap this long means the episode ended in a way that was not seen
// here. It is a multiple of the retry bound so that a long gap is not mistaken
// for the start of a new episode by the bound itself.
const episodeStaleAfter = 2 * DefaultMaxReconcileDuration

// failureKey identifies one operation on one object handled by one reconciler.
type failureKey struct {
	reconciler string
	objectType string
	objectID   uint
	operation  string
}

// failureEpisode is a run of failures with no success between them.
type failureEpisode struct {
	first time.Time
	last  time.Time
}

// defaultFailures is the tracker a Reconciler uses when it has not been given
// one. It is shared by every reconciler in the process, which is what lets the
// concurrent workers of one reconciler - separate Reconciler values - agree on
// when an episode began.
var defaultFailures = &FailureTracker{}

func (t *FailureTracker) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// record notes a failed attempt and returns how long the operation has been
// failing, as of the first failed attempt of the current episode. The attempt
// that begins an episode reports zero.
//
// An episode with no failure for episodeStaleAfter is over: whatever ended it
// - a success elsewhere, a deleted object - was not seen here, and the next
// failure starts a new one rather than inheriting an old start. Success and
// giving up end an episode explicitly, through clear.
func (t *FailureTracker) record(key failureKey) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	t.sweep(now)

	episode, ok := t.episodes[key]
	if !ok || now.Sub(episode.last) > episodeStaleAfter {
		episode = failureEpisode{first: now}
	}
	episode.last = now

	if t.episodes == nil {
		t.episodes = make(map[failureKey]failureEpisode)
	}
	t.episodes[key] = episode

	return now.Sub(episode.first)
}

// clear ends the episode for key, as a success does.
func (t *FailureTracker) clear(key failureKey) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.episodes, key)
}

// sweep drops episodes that have ended without being cleared, so objects that
// stop failing without a success being recorded - deleted, say - do not
// accumulate. It runs at most once a minute. The caller holds the lock.
func (t *FailureTracker) sweep(now time.Time) {
	if now.Sub(t.lastSweep) < time.Minute {
		return
	}
	t.lastSweep = now

	for key, episode := range t.episodes {
		if now.Sub(episode.last) > episodeStaleAfter {
			delete(t.episodes, key)
		}
	}
}

func (r *Reconciler) failures() *FailureTracker {
	if r.Failures != nil {
		return r.Failures
	}
	return defaultFailures
}

func (r *Reconciler) failureKey(object apilib.ReconciledThreeportApiObject, operation string) failureKey {
	return failureKey{
		reconciler: r.Name,
		objectType: object.GetFullyQualifiedType(),
		objectID:   object.GetId(),
		operation:  operation,
	}
}

// RecordFailure notes that an operation on an object failed and reports whether
// it has been failing for longer than DefaultMaxReconcileDuration, in which
// case the reconciler should stop retrying it. The window starts at the first
// failed attempt.
func (r *Reconciler) RecordFailure(object apilib.ReconciledThreeportApiObject, operation string) bool {
	return r.failures().record(r.failureKey(object, operation)) > DefaultMaxReconcileDuration
}

// ClearFailure ends the failure episode for an operation on an object. A
// reconciler calls it when the operation succeeds, and when it gives up: in
// both cases whatever notification comes next is new work, and a later failure
// starts a new window rather than finding one that has already run out.
func (r *Reconciler) ClearFailure(object apilib.ReconciledThreeportApiObject, operation string) {
	r.failures().clear(r.failureKey(object, operation))
}
