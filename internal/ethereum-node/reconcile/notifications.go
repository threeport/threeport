package reconcile

import (
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

type EthereumNodeNotification struct {
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
	Object v0.EthereumNodeDefinition
}
