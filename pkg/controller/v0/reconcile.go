package controller

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// ReconcilerConfig contains values needed to start new reconcilers in
// controllers.
type ReconcilerConfig struct {
	// The name to use for the reconciler.
	Name string

	// The function that will perform object reconciliation.
	ReconcileFunc func(r *Reconciler)

	// The maximum number of concurrent reconilation process to run.  This
	// number should be tuned (programatically) to be higher for reconcilers
	// that are called more often.  The tuning should be based on the length of
	// queue in NATS with a cap at some reasonable number that will keep CPU
	// consumption at a reasonable level for each individual controller.  The
	// proportion of activity among reconcilers within a controller is the key.
	ConcurrentReconciles int

	// The NATS Jetstream subject used for notifications to a reconciler
	NotifSubject string
}

// Reconciler contains the assets needed by reconcilers to recieve subscription
// messages and update the key-value store in order to lock reconciliation of
// objects.
type Reconciler struct {
	// Reconciler name for display in logs.
	Name string

	// APIServer is the endpoint to reach Threeport REST API.
	// format: [protocol]://[hostname]:[port]
	APIServer string

	// APIClient is the HTTP client used to make requests to the Threeport API.
	APIClient *http.Client

	// JetStreamContext is the context for the NATS persistence layer.
	JetStreamContext nats.JetStreamContext

	// Sub is the NATS subscription used to get messages to reconcile.
	Sub *nats.Subscription

	// Ready reflects whether this reconciler's JetStream consumer and
	// subscription are currently known to be alive. It is set true once
	// startup succeeds and cleared if PullMessage detects a permanent
	// subscription error.
	Ready *atomic.Bool

	// KeyValue is the NATS key-value store to be used for locking object
	// reconciliation.
	KeyValue nats.KeyValue

	// ControllerID is the unique identifier for each controller instance.
	ControllerID uuid.UUID

	// heartbeats holds the refresher for each lock this controller is
	// currently holding, keyed by lock key. See Lock.
	heartbeats   map[string]*lockHeartbeat
	heartbeatsMu sync.Mutex

	// Log is the logger used to write logs.
	Log *logr.Logger

	// Shutdown is used to instruct a reconciler to shut down.
	Shutdown chan bool

	// ShutdownWait is the wait group that waits for reconcilers to finish
	// before shutting down the controller.
	ShutdownWait *sync.WaitGroup

	// EncryptionKey is the key used to encrypt and decrypt sensitive fields.
	EncryptionKey string

	// EventsRecorder is the recorder used to record events.
	EventsRecorder Recorder
}

// Recorder is an interface for recording events. The
// fullyQualifiedObjectType string passed to either method must be the
// form returned by GetFullyQualifiedType.
type Recorder interface {
	RecordEvent(event *v0.Event, objectId uint, fullyQualifiedObjectType string) error
	HandleEventOverride(event *v0.Event, objectId uint, fullyQualifiedObjectType string, err error, log *logr.Logger)
}

// PullMessage checks the queue for a message and returns it if there was a
// message to retrieve.  It fetches only one message at a time and waits 20
// seconds for a message to become available.  If no message is returned in 20
// seconds, it returns nil so the reconciler can reconnect to NATS.
func (r *Reconciler) PullMessage() *nats.Msg {
	messages, err := r.Sub.Fetch(1, nats.MaxWait(time.Second*20))
	if err != nil {
		if errors.Is(err, nats.ErrConsumerNotFound) ||
			errors.Is(err, nats.ErrConsumerDeleted) ||
			errors.Is(err, nats.ErrStreamNotFound) {
			r.Ready.Store(false)
			r.Log.Error(err, "permanent JetStream consumer error, restarting")
			os.Exit(1)
		}
		if !errors.Is(err, nats.ErrTimeout) {
			r.Log.Error(err, "failed to fetch message from pull subscription")
		}
		return nil
	}
	if len(messages) == 0 {
		return nil
	}
	msg := messages[0]
	r.Log.V(1).Info("new message received", "msgSubject", msg.Subject)
	return msg
}

// RequeueRaw requeues a notification when the last one could not be
// unmarshalled properly or when a new notification payload could not be
// properly constructed.  Since a backoff cannot be properly calculated we
// requeue after 10 sec.
func (r *Reconciler) RequeueRaw(msg *nats.Msg) {
	msg.NakWithDelay(time.Duration(time.Duration(10).Seconds()))
	r.Log.V(1).Info("raw message requeued",
		"messageSubject", msg.Subject,
		"messagePayload", string(msg.Data),
	)
}

// UnlockAndRequeue releases the lock on the object and requeues reconciliation
// for that object.
func (r *Reconciler) UnlockAndRequeue(
	object apilib.ReconciledThreeportApiObject,
	requeueDelay int64,
	lockReleased chan bool,
	msg *nats.Msg,
) {
	if ok := r.ReleaseLock(object, lockReleased, msg, false); !ok {
		r.Log.V(1).Info(
			"object remains locked - will unlock when TTL expires",
			"objectType", object.GetType(),
			"objectVersion", object.GetVersion(),
			"objectID", object.GetId(),
		)
	} else {
		r.Log.V(1).Info(
			"object unlocked",
			"objectType", object.GetType(),
			"objectVersion", object.GetVersion(),
			"objectID", object.GetId(),
		)
	}

	r.Requeue(object, requeueDelay, msg)
}

// Requeue waits for the delay duration and then sends the notifcation to the
// NATS server to trigger reconciliation.
func (r *Reconciler) Requeue(
	object apilib.ReconciledThreeportApiObject,
	requeueDelay int64,
	msg *nats.Msg,
) {
	err := msg.NakWithDelay(time.Duration(requeueDelay) * time.Second)
	if err != nil {
		r.Log.V(1).Info(
			"failed to perform negative acknowledgement with delay",
			"objectType", object.GetType(),
			"objectVersion", object.GetVersion(),
			"objectID", object.GetId(),
		)
	} else {
		r.Log.V(1).Info(
			"requeue notification sent",
			"reconcilerName", r.Name,
			"objectType", object.GetType(),
			"objectVersion", object.GetVersion(),
			"objectID", object.GetId(),
			"requeueDelay", requeueDelay,
		)
	}
}

// lockKey constructs the lock string for an object.
func (r *Reconciler) lockKey(id uint) string {
	return fmt.Sprintf("%s.%d", r.Name, id)
}

// CheckLock returns two bool values when checking for a lock on an object.  The
// first value is whether the object is locked and the second is the status of
// the check.  If the check was unsuccessful and unable to be clearly determined
// the second value will be false.
func (r *Reconciler) CheckLock(object apilib.ReconciledThreeportApiObject) (bool, bool) {
	lockKey := r.lockKey(object.GetId())

	kvEntry, err := r.KeyValue.Get(lockKey)
	if err != nil {
		if !errors.Is(err, nats.ErrKeyNotFound) && !errors.Is(err, nats.ErrKeyDeleted) {
			r.Log.Error(
				err, "failed to get key-value record",
				"lockKey", lockKey,
				"bucket", r.KeyValue.Bucket(),
			)
			return false, false
		}
	}
	if kvEntry != nil {
		r.Log.V(1).Info(
			"object is locked - requeue",
			"objectType", object.GetType(),
			"objectVersion", object.GetVersion(),
			"objectID", object.GetId(),
		)
		return true, true
	}

	return false, true
}

// Lock puts a lock on the given object so that no other reconcilation of this
// object is attempted until unlocked.  Returns true if successful.
func (r *Reconciler) Lock(object apilib.ReconciledThreeportApiObject) bool {
	lockKey := r.lockKey(object.GetId())

	rev, err := r.KeyValue.Create(lockKey, []byte(r.ControllerID.String()))
	if err != nil {
		r.Log.Error(
			err, "failed to apply lock to object for reconciliation",
			"lockKey", lockKey,
			"bucket", r.KeyValue.Bucket(),
		)
		return false
	}
	r.startHeartbeat(lockKey, rev)

	r.Log.V(1).Info(
		"object locked for reconciliation",
		"keyValueRevision", rev,
		"objectType", object.GetType(),
		"objectVersion", object.GetVersion(),
		"objectID", object.GetId(),
	)

	return true
}

// LockTTL is how long a lock survives without being refreshed, and
// LockRefreshInterval is how often a held lock is refreshed.
//
// The TTL is a backstop for a controller that dies without releasing its
// locks - a kill, an out-of-memory, a node going away. A graceful shutdown
// already releases them: the reconcilers run a handler on SIGINT and SIGTERM
// that unlocks and requeues.
//
// It used to be twenty minutes, long enough to cover the longest reconcile,
// because a lock that expired underneath a running reconcile would let a
// second worker pick up the same object. The cost was that a crash held every
// lock it had for twenty minutes, and with one worker that is twenty minutes
// in which nothing else of that kind reconciles at all - including deletes.
//
// Refreshing the lock while the reconcile is alive separates the two. The TTL
// no longer has to cover how long work takes, only how long a dead controller
// goes unnoticed, so it is short. A reconcile that runs for an hour keeps its
// lock for an hour.
// They are variables rather than constants so a test can run the refresh in
// milliseconds instead of waiting out a real interval.
var (
	LockTTL             = time.Minute
	LockRefreshInterval = LockTTL / 3
)

// lockHeartbeat is the refresher for one held lock.
type lockHeartbeat struct {
	stop chan struct{}
	done chan struct{}

	mu       sync.Mutex
	revision uint64
	lost     bool
}

// startHeartbeat keeps a held lock alive until stopHeartbeat is called.
//
// The refresh interval is a third of the TTL, so two refreshes have to be
// missed before a live reconcile loses its lock.
//
// The refresh is an Update against the revision this controller holds, not a
// Put. A Put would write the key whatever its state, so a controller that had
// been cut off from NATS long enough for its lock to expire - and be taken by
// somebody else - would come back and stamp its own id over the new owner's.
// Two controllers would then believe they held the same object. An Update
// fails instead, and the refresher stops and says so.
func (r *Reconciler) startHeartbeat(lockKey string, revision uint64) {
	beat := &lockHeartbeat{
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		revision: revision,
	}

	r.heartbeatsMu.Lock()
	if r.heartbeats == nil {
		r.heartbeats = make(map[string]*lockHeartbeat)
	}
	// a lock already being refreshed is left alone rather than given a
	// second goroutine writing the same key - unless the entry there is one
	// whose refresher already lost the lock and exited. That can still be in
	// the map: only stopHeartbeat removes an entry, and the goroutine that
	// loses a lock returns without calling it. If this object's lock was then
	// reacquired by a fresh reconcile before the dead one finished and called
	// ReleaseLock, skipping here would leave that reacquired lock with no
	// refresher at all, unguarded until LockTTL expires it on its own.
	if existing, running := r.heartbeats[lockKey]; running {
		existing.mu.Lock()
		lost := existing.lost
		existing.mu.Unlock()
		if !lost {
			r.heartbeatsMu.Unlock()
			return
		}
	}
	r.heartbeats[lockKey] = beat
	r.heartbeatsMu.Unlock()

	go func() {
		defer close(beat.done)

		ticker := time.NewTicker(LockRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-beat.stop:
				return
			case <-ticker.C:
				beat.mu.Lock()
				current := beat.revision
				beat.mu.Unlock()

				next, err := r.KeyValue.Update(lockKey, []byte(r.ControllerID.String()), current)
				if err != nil {
					// the lock is somebody else's now, or gone. Either way
					// this controller no longer holds it, and writing again
					// would take it back from whoever does.
					beat.mu.Lock()
					beat.lost = true
					beat.mu.Unlock()
					r.Log.Error(
						err, "lost the lock on an object still being reconciled",
						"lockKey", lockKey,
						"bucket", r.KeyValue.Bucket(),
					)

					return
				}

				beat.mu.Lock()
				beat.revision = next
				beat.mu.Unlock()
			}
		}
	}()
}

// stopHeartbeat ends the refresh for a lock and waits for it to finish.
//
// Waiting matters: closing the channel only asks the goroutine to stop, and
// one that has already woken on a tick can still be partway through its
// write. Returning before it finishes lets that write land after the key has
// been deleted, which puts back a lock nobody holds and blocks the object
// until the TTL clears it.
//
// It returns the revision this controller last held, and whether the lock was
// still its own.
func (r *Reconciler) stopHeartbeat(lockKey string) (uint64, bool) {
	r.heartbeatsMu.Lock()
	beat, running := r.heartbeats[lockKey]
	if running {
		delete(r.heartbeats, lockKey)
	}
	r.heartbeatsMu.Unlock()

	if !running {
		return 0, false
	}

	close(beat.stop)
	<-beat.done

	beat.mu.Lock()
	defer beat.mu.Unlock()

	return beat.revision, !beat.lost
}

// ReleaseLock deletes the kev-value key so that future reconciliation will no
// longer be locked.  Rerturns true if successful.  If the lock fails to be
// released it will remain locked until the TTL expires in NATS.
func (r *Reconciler) ReleaseLock(object apilib.ReconciledThreeportApiObject, lockReleased chan bool, msg *nats.Msg, reconcileSuccess bool) bool {
	lockKey := r.lockKey(object.GetId())
	revision, stillOurs := r.stopHeartbeat(lockKey)

	// deleting by revision rather than by key: if the lock expired and was
	// taken by another controller, the key under this name is theirs, and
	// removing it would hand the object to a third
	deleteOpts := []nats.DeleteOpt{}
	if revision > 0 {
		deleteOpts = append(deleteOpts, nats.LastRevision(revision))
	}
	if !stillOurs {
		r.Log.Error(
			fmt.Errorf("lock was no longer held"),
			"not deleting a lock this controller no longer owns",
			"lockKey", lockKey,
			"bucket", r.KeyValue.Bucket(),
		)
	}

	if err := r.KeyValue.Delete(lockKey, deleteOpts...); err != nil {
		r.Log.Error(
			err, "failed to delete key-value record",
			"lockKey", lockKey,
			"bucket", r.KeyValue.Bucket(),
		)
		return false
	}

	// send a message to the lockReleased channel to indicate that the
	// lock has been released
	lockReleased <- true

	if reconcileSuccess {
		// acknowledge message so nats does not requeue and wait for response
		// before continuing to avoid race condition of re-pulling the same
		// message again
		if err := msg.AckSync(); err != nil {
			r.Log.Error(
				err, "failed to perform acknowledgement",
				"objectType", object.GetType(),
				"objectVersion", object.GetVersion(),
				"objectID", object.GetId(),
			)
		}
	}

	return true
}
