package controller

import "time"

const (
	DefaultInitialRequeueDelay = 1
	DefaultMaxRequeueDelay     = 30
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
