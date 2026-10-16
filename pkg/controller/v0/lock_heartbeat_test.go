package controller

import (
	"errors"
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

// fakeKeyValue is a key-value store that behaves like the one the lock talks
// to: Create refuses an existing key, Update refuses a stale revision.
//
// The embedded interface is nil, so any method the lock starts calling that
// this does not implement panics rather than quietly returning a zero value.
type fakeKeyValue struct {
	nats.KeyValue

	mu       sync.Mutex
	present  bool
	revision uint64
	owner    string

	creates, updates, deletes int

	// operations in the order they completed, so a test can see a refresh
	// that landed after the delete
	order []string

	// beforeUpdate runs inside Update, so a test can interleave a release
	// with a refresh that is already in flight
	beforeUpdate func()
}

func (k *fakeKeyValue) Create(key string, value []byte) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.present {
		return 0, errors.New("key exists")
	}
	k.present = true
	k.revision++
	k.owner = string(value)
	k.creates++
	k.order = append(k.order, "create")

	return k.revision, nil
}

func (k *fakeKeyValue) Update(key string, value []byte, revision uint64) (uint64, error) {
	if k.beforeUpdate != nil {
		k.beforeUpdate()
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	k.updates++
	k.order = append(k.order, "update")
	if !k.present || revision != k.revision {
		return 0, errors.New("wrong last sequence")
	}
	k.revision++
	k.owner = string(value)

	return k.revision, nil
}

func (k *fakeKeyValue) Delete(key string, opts ...nats.DeleteOpt) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.deletes++
	k.order = append(k.order, "delete")
	k.present = false

	return nil
}

// Put writes whatever is there, which is what the refresh must not do.
func (k *fakeKeyValue) Put(key string, value []byte) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.updates++
	k.order = append(k.order, "update")
	k.present = true
	k.revision++
	k.owner = string(value)

	return k.revision, nil
}

func (k *fakeKeyValue) Bucket() string { return "test-lock-bucket" }

func (k *fakeKeyValue) counts() (creates, updates, deletes int) {
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.creates, k.updates, k.deletes
}

func (k *fakeKeyValue) operations() []string {
	k.mu.Lock()
	defer k.mu.Unlock()

	return append([]string(nil), k.order...)
}

func (k *fakeKeyValue) heldBy() (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.owner, k.present
}

// lockTestObject is the little the lock asks of an object.
type lockTestObject struct {
	apilib.ReconciledThreeportApiObject

	id uint
}

func (o *lockTestObject) GetId() uint        { return o.id }
func (o *lockTestObject) GetType() string    { return "LockTestObject" }
func (o *lockTestObject) GetVersion() string { return "v0" }

var discardLog = logr.Discard()

// reconcilerFor builds a reconciler against a key-value store, with the lock
// timings shortened so these tests run in milliseconds.
func reconcilerFor(t *testing.T, keyValue nats.KeyValue) *Reconciler {
	t.Helper()
	originalTTL, originalInterval := LockTTL, LockRefreshInterval
	LockTTL, LockRefreshInterval = 30*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { LockTTL, LockRefreshInterval = originalTTL, originalInterval })

	return &Reconciler{
		Name:         "test",
		KeyValue:     keyValue,
		ControllerID: uuid.New(),
		Log:          &discardLog,
	}
}

// TestAHeldLockIsKeptAlive covers what lets the TTL be short.
//
// The TTL used to be twenty minutes because it had to outlast the longest
// reconcile: a lock expiring underneath a running one lets a second worker
// take the same object. The cost was that a controller which died held every
// lock it had for twenty minutes, and with a single worker that is twenty
// minutes in which nothing else of that kind reconciles, deletes included.
func TestAHeldLockIsKeptAlive(t *testing.T) {
	keyValue := &fakeKeyValue{}
	reconciler := reconcilerFor(t, keyValue)
	object := &lockTestObject{id: 1}

	require.True(t, reconciler.Lock(object))
	creates, _, _ := keyValue.counts()
	assert.Equal(t, 1, creates, "the lock is a create, so a second holder is refused")

	time.Sleep(60 * time.Millisecond)
	_, updates, _ := keyValue.counts()
	assert.GreaterOrEqual(
		t, updates, 2,
		"a reconcile still running has to keep its lock alive or the TTL takes it away",
	)

	require.True(t, reconciler.ReleaseLock(object, make(chan bool, 1), nil, false))
	_, updatesAtRelease, deletes := keyValue.counts()
	assert.Equal(t, 1, deletes)

	time.Sleep(60 * time.Millisecond)
	_, updatesAfter, _ := keyValue.counts()
	assert.Equal(
		t, updatesAtRelease, updatesAfter,
		"the refresh must stop with the lock, or it puts back one nobody holds",
	)

	_, present := keyValue.heldBy()
	assert.False(t, present, "the lock is gone once released")
}

// TestAReleaseWaitsForARefreshAlreadyInFlight covers the race between the two.
//
// Closing the stop channel only asks the refresher to finish. One that has
// already woken on a tick can be partway through its write, and if the
// release does not wait, that write lands after the key is deleted - putting
// back a lock nobody holds, which blocks the object until the TTL clears it.
func TestAReleaseWaitsForARefreshAlreadyInFlight(t *testing.T) {
	keyValue := &fakeKeyValue{}
	released := make(chan struct{})

	// hold the refresh inside Update until the release has been asked for
	var once sync.Once
	keyValue.beforeUpdate = func() {
		once.Do(func() { <-released })
	}

	reconciler := reconcilerFor(t, keyValue)
	object := &lockTestObject{id: 1}
	require.True(t, reconciler.Lock(object))

	// let a refresh begin and block inside Update
	time.Sleep(20 * time.Millisecond)

	go func() {
		time.Sleep(10 * time.Millisecond)
		close(released)
	}()
	reconciler.ReleaseLock(object, make(chan bool, 1), nil, false)

	// long enough for a refresh that was not waited for to land afterwards
	time.Sleep(50 * time.Millisecond)

	_, present := keyValue.heldBy()
	assert.False(
		t, present,
		"a refresh that finished after the delete would leave a lock nobody holds",
	)

	operations := keyValue.operations()
	require.NotEmpty(t, operations)
	assert.Equal(
		t, "delete", operations[len(operations)-1],
		"the delete has to be the last word; a refresh completing after it means the release "+
			"did not wait, and the revision the delete was given is already stale",
	)
}

// TestARefreshDoesNotTakeBackALockSomebodyElseHolds covers losing the lock.
//
// A controller cut off from NATS for longer than the TTL loses its lock, and
// another controller can take it. An unconditional write would stamp this
// controller's id back over the new owner's, and both would believe they held
// the same object.
func TestARefreshDoesNotTakeBackALockSomebodyElseHolds(t *testing.T) {
	keyValue := &fakeKeyValue{}
	reconciler := reconcilerFor(t, keyValue)
	object := &lockTestObject{id: 1}

	require.True(t, reconciler.Lock(object))
	ours, _ := keyValue.heldBy()

	// somebody else takes the key while this controller is not looking
	keyValue.mu.Lock()
	keyValue.revision += 5
	keyValue.owner = "another-controller"
	keyValue.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	owner, present := keyValue.heldBy()
	require.True(t, present)
	assert.Equal(
		t, "another-controller", owner,
		"the refresh must not write over a lock this controller no longer holds",
	)
	assert.NotEqual(t, ours, owner)
}

// TestTheLockTtlIsShortEnoughToMatter covers the values themselves.
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
