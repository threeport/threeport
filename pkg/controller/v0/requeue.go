package controller

import "time"

const (
	DefaultInitialRequeueDelay = 1
	DefaultMaxRequeueDelay     = 30

	// DefaultMaxReconcileDuration is how long a reconciler keeps requeueing an
	// operation that keeps failing, measured from its first failed attempt (see
	// FailureTracker). The requeue delay is capped
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
