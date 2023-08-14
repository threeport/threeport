package controller

import "time"

const (
	DefaultInitialRequeueDelay = 1
	DefaultMaxRequeueDelay     = 300
)

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
