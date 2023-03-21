package controller

const (
	DefaultInitialRequeueDelay = 1
	DefaultMaxRequeueDelay     = 300
)

// SetRequeueDelay sets the requeue delay.  It will be set to the initial delay
// value if the first requeue for the object.  It will be set to double the
// previous delay if not the first, or the max delay if reached.
func SetRequeueDelay(lastDelay *int64, initialDelay, maxDelay int64) int64 {
	var requeueDelay int64

	switch {
	case *lastDelay == 0:
		requeueDelay = initialDelay
	case lastDelay != nil:
		requeueDelay = *lastDelay * 2
	default:
		requeueDelay = initialDelay
	}

	if requeueDelay > maxDelay {
		requeueDelay = maxDelay
	}

	return requeueDelay
}
