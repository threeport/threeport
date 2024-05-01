package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
)

// ReconcilerConfig contains values needed to start new reconcilers in
// controllers.
type ReconcilerConfig struct {
	// The name to use for the reconciler.
	Name string

	// The object type the reconciler will manage.
	ObjectType string

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

	// ObjectType is the name of the object that is being reconciled.
	ObjectType string

	// APIServer is the endpoint to reach Threeport REST API.
	// format: [protocol]://[hostname]:[port]
	APIServer string

	// APIClient is the HTTP client used to make requests to the Threeport API.
	APIClient *http.Client

	// JetStreamContext is the context for the NATS persistence layer.
	JetStreamContext nats.JetStreamContext

	// Sub is the NATS subscription used to get messages to reconcile.
	Sub *nats.Subscription

	// KeyValue is the NATS key-value store to be used for locking object
	// reconciliation.
	KeyValue nats.KeyValue

	// ControllerID is the unique identifier for each controller instance.
	ControllerID uuid.UUID

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

// Recorder is an interface for recording events.
type Recorder interface {
	RecordEvent(*v1.Event, *uint) error
	HandleEventOverride(*v1.Event, *uint, error, *logr.Logger)
}

// PullMessage checks the queue for a message and returns it if there was a
// message to retrieve.  It fetches only one message at a time and waits 20
// seconds for a message to become available.  If no message is returned in 20
// seconds, it returns nil so the reconciler can reconnect to NATS.
func (r *Reconciler) PullMessage() *nats.Msg {
	messages, err := r.Sub.Fetch(1, nats.MaxWait(time.Second*20))
	if err != nil && !errors.Is(err, nats.ErrTimeout) {
		r.Log.Error(err, "failed to fetch message from pull subscription")
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
	object v0.APIObject,
	requeueDelay int64,
	lockReleased chan bool,
	msg *nats.Msg,
) {
	if ok := r.ReleaseLock(object, lockReleased, msg, false); !ok {
		r.Log.V(1).Info(
			"object remains locked - will unlock when TTL expires",
			"objectType", r.ObjectType,
			"objectID", object.GetID(),
		)
	} else {
		r.Log.V(1).Info(
			"object unlocked",
			"objectType", r.ObjectType,
			"objectID", object.GetID(),
		)
	}

	r.Requeue(object, requeueDelay, msg)
}

// Requeue waits for the delay duration and then sends the notifcation to the
// NATS server to trigger reconciliation.
func (r *Reconciler) Requeue(
	object v0.APIObject,
	requeueDelay int64,
	msg *nats.Msg,
) {
	err := msg.NakWithDelay(time.Duration(requeueDelay) * time.Second)
	if err != nil {
		r.Log.V(1).Info(
			"failed to perform negative acknowledgement with delay",
			"objectType", r.ObjectType,
			"objectID", object.GetID(),
		)
	} else {
		r.Log.V(1).Info(
			"requeue notification sent",
			"reconcilerName", r.Name,
			"objectType", r.ObjectType,
			"objectID", object.GetID(),
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
func (r *Reconciler) CheckLock(object v0.APIObject) (bool, bool) {
	lockKey := r.lockKey(object.GetID())

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
			"objectType", r.ObjectType,
			"objectID", object.GetID(),
		)
		return true, true
	}

	return false, true
}

// Lock puts a lock on the given object so that no other reconcilation of this
// object is attempted until unlocked.  Returns true if successful.
func (r *Reconciler) Lock(object v0.APIObject) bool {
	lockKey := r.lockKey(object.GetID())

	rev, err := r.KeyValue.Create(lockKey, []byte(r.ControllerID.String()))
	if err != nil {
		r.Log.Error(
			err, "failed to apply lock to object for reconciliation",
			"lockKey", lockKey,
			"bucket", r.KeyValue.Bucket(),
		)
		return false
	}
	r.Log.V(1).Info(
		"object locked for reconciliation",
		"keyValueRevision", rev,
		"objectType", r.ObjectType,
		"objectID", object.GetID(),
	)

	return true
}

// ReleaseLock deletes the kev-value key so that future reconciliation will no
// longer be locked.  Rerturns true if successful.  If the lock fails to be
// released it will remain locked until the TTL expires in NATS.
func (r *Reconciler) ReleaseLock(object v0.APIObject, lockReleased chan bool, msg *nats.Msg, reconcileSuccess bool) bool {
	lockKey := r.lockKey(object.GetID())

	if err := r.KeyValue.Delete(lockKey); err != nil {
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
				"objectType", r.ObjectType,
				"objectID", object.GetID(),
			)
		}
	}

	return true
}
