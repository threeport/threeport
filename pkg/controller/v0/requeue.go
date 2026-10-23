package controller

import "time"

const (
	DefaultInitialRequeueDelay = 1
	DefaultMaxRequeueDelay     = 30

	// DefaultMaxReconcileDuration is how long a reconciler keeps requeueing a
	// notification whose operation keeps failing. The requeue delay is capped
	// at DefaultMaxRequeueDelay, so without a bound a failure that cannot
	// resolve itself - a permission that is never granted - is retried every
	// 30 seconds for as long as the object exists. It is generous enough to
	// cover waiting on a dependency such as a cluster that is still being
	// provisioned.
	DefaultMaxReconcileDuration = time.Hour
)

// Done is the requeue delay signaling this reconcile pass is complete;
// the wrapper marks the subject reconciled without requeuing.
const Done int64 = 0

// Requeue30s is the standard requeue delay for waiting on a child's
// Reconciled=true state before marking the parent reconciled.
const Requeue30s int64 = 30

// SetRequeueDelay sets the requeue delay.  It will be set to the initial delay
// value if the first requeue for the object.  It will be set to double the
// previous delay if not the first, or the max delay if reached.
func SetRequeueDelay(creationTime *int64) int64 {
	var requeueDelay int64

	currentTime := time.Now().Unix()
	elapsedTime := currentTime - *creationTime

	if elapsedTime < DefaultInitialRequeueDelay {
		requeueDelay = DefaultInitialRequeueDelay
	} else if elapsedTime > DefaultMaxRequeueDelay {
		requeueDelay = DefaultMaxRequeueDelay
	} else {
		requeueDelay = elapsedTime * 2
	}

	return requeueDelay
}

// RetriesExhausted reports whether a notification has been failing for longer
// than DefaultMaxReconcileDuration, measured from when the notification was
// created. A redelivered notification keeps its creation time, so this grows
// across requeues. A notification with no creation time is never exhausted:
// there is nothing to measure from, and giving up on a guess would drop work.
func RetriesExhausted(creationTime *int64) bool {
	if creationTime == nil {
		return false
	}

	elapsed := time.Duration(time.Now().Unix()-*creationTime) * time.Second
	return elapsed > DefaultMaxReconcileDuration
}
