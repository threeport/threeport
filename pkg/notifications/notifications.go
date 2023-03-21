package notifications

// Notification is the message that is sent to NATS to alert a controller that a
// change has been made to an object.  A notification is sent by the API Server
// when a change is make by a client, or by a controller when reconciliation was
// not completed and it needs to be requeued.
type Notification struct {
	// The operation performed to trigger the notification.
	// One of:
	// * Created
	// * Updated
	// * Deleted
	Operation string

	// Whether the notification was a part of a requeue.  It will be false when
	// the API Server sends the notification in response to a client change.  It
	// will be true when requeued by a controller.
	Requeue bool

	// Tracks the backoff delay last used in a requeue so that it may
	// incremented or repeated (when at max delay) as appropriate.
	LastRequeueDelay *int64

	// The API object that has been changed.
	Object interface{}
}
