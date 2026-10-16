package controller

import (
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
)

// countingKeyValue records the calls the lock makes.
//
// The embedded interface is nil: any method this test does not exercise
// panics rather than quietly returning a zero value, so a change that starts
// calling something else is reported rather than passing.
type countingKeyValue struct {
	nats.KeyValue

	mu      sync.Mutex
	creates int
	puts    int
	deletes int
}

func (k *countingKeyValue) Create(key string, value []byte) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.creates++

	return uint64(k.creates), nil
}

func (k *countingKeyValue) Put(key string, value []byte) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.puts++

	return uint64(k.puts), nil
}

func (k *countingKeyValue) Delete(key string, opts ...nats.DeleteOpt) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.deletes++

	return nil
}

func (k *countingKeyValue) Bucket() string { return "test-lock-bucket" }

func (k *countingKeyValue) counts() (creates, puts, deletes int) {
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.creates, k.puts, k.deletes
}

// TestAHeldLockIsKeptAlive covers what lets the TTL be short.
//
// The TTL used to be twenty minutes because it had to outlast the longest
// reconcile - a lock that expired underneath a running one would let a second
// worker pick up the same object. The cost was that a controller which died
// held every lock it had for twenty minutes, and with a single worker that is
// twenty minutes in which nothing else of that kind reconciles, deletes
// included.
//
// Refreshing a held lock separates the two. The TTL covers how long a dead
// controller goes unnoticed; how long the work takes stops mattering.
func TestAHeldLockIsKeptAlive(t *testing.T) {
	originalTTL, originalInterval := LockTTL, LockRefreshInterval
	LockTTL, LockRefreshInterval = 30*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { LockTTL, LockRefreshInterval = originalTTL, originalInterval })

	keyValue := &countingKeyValue{}
	reconciler := &Reconciler{
		Name:         "test",
		KeyValue:     keyValue,
		ControllerID: uuid.New(),
		Log:          &discardLog,
	}
	object := &lockTestObject{id: 1}

	require.True(t, reconciler.Lock(object))
	creates, _, _ := keyValue.counts()
	assert.Equal(t, 1, creates, "the lock itself is a create, so a second holder is refused")

	// long enough for several refreshes
	time.Sleep(60 * time.Millisecond)
	_, puts, _ := keyValue.counts()
	assert.GreaterOrEqual(
		t, puts, 2,
		"a reconcile still running has to keep its lock alive or the TTL takes it away",
	)

	require.True(t, reconciler.ReleaseLock(object, make(chan bool, 1), nil, false))
	_, putsAtRelease, deletes := keyValue.counts()
	assert.Equal(t, 1, deletes)

	// nothing should touch the key after the lock is gone
	time.Sleep(60 * time.Millisecond)
	_, putsAfter, _ := keyValue.counts()
	assert.Equal(
		t, putsAtRelease, putsAfter,
		"the refresh must stop with the lock, or it recreates a lock nobody holds",
	)
}

// TestLockingTwiceDoesNotStartASecondRefresh covers the bookkeeping.
func TestLockingTwiceDoesNotStartASecondRefresh(t *testing.T) {
	originalTTL, originalInterval := LockTTL, LockRefreshInterval
	LockTTL, LockRefreshInterval = 30*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { LockTTL, LockRefreshInterval = originalTTL, originalInterval })

	keyValue := &countingKeyValue{}
	reconciler := &Reconciler{
		Name:         "test",
		KeyValue:     keyValue,
		ControllerID: uuid.New(),
		Log:          &discardLog,
	}
	object := &lockTestObject{id: 1}

	require.True(t, reconciler.Lock(object))
	require.True(t, reconciler.Lock(object))

	time.Sleep(45 * time.Millisecond)
	_, puts, _ := keyValue.counts()
	assert.LessOrEqual(
		t, puts, 6,
		"two refresh goroutines on one key would roughly double the writes",
	)

	reconciler.ReleaseLock(object, make(chan bool, 1), nil, false)
	_, putsAtRelease, _ := keyValue.counts()
	time.Sleep(45 * time.Millisecond)
	_, putsAfter, _ := keyValue.counts()
	assert.Equal(t, putsAtRelease, putsAfter, "releasing once has to stop every refresh")
}

// TestTheLockTtlIsShortEnoughToMatter covers the value itself.
func TestTheLockTtlIsShortEnoughToMatter(t *testing.T) {
	assert.LessOrEqual(
		t, LockTTL, 5*time.Minute,
		"the TTL is how long a dead controller blocks everything else of its kind",
	)
	assert.Less(
		t, LockRefreshInterval, LockTTL,
		"a refresh slower than the TTL loses the lock it is meant to hold",
	)
	assert.LessOrEqual(
		t, LockRefreshInterval*2, LockTTL,
		"two refreshes have to be missable before a live reconcile loses its lock",
	)
}

// lockTestObject is the little the lock asks of an object.
type lockTestObject struct {
	apilib.ReconciledThreeportApiObject

	id uint
}

func (o *lockTestObject) GetId() uint        { return o.id }
func (o *lockTestObject) GetType() string    { return "LockTestObject" }
func (o *lockTestObject) GetVersion() string { return "v0" }

// discardLog is the logger these tests hand the reconciler.
var discardLog = logr.Discard()
