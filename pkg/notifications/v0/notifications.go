package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// NotificationOperation informs a reconciler what operation was performed in
// the API to trigger the notification.
type NotificationOperation string

const (
	NotificationOperationCreated = "Created"
	NotificationOperationUpdated = "Updated"
	NotificationOperationDeleted = "Deleted"
)

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
	Operation NotificationOperation

	// Tracks the backoff delay last used in a requeue so that it may
	// incremented or repeated (when at max delay) as appropriate.
	CreationTime *int64

	// The API object that has been changed.
	Object interface{}

	// The API object version.  This allows controller code to determine which
	// version of the object has been delivered in the notification payload so
	// as to process it properly.
	ObjectVersion string
}

// ConsumeMessage generates a Notificatiion object from a json notification from
// NATS to a controller.
func ConsumeMessage(msgData []byte) (*Notification, error) {
	var notif Notification
	decoder := json.NewDecoder(bytes.NewReader(msgData))
	decoder.UseNumber()
	if err := decoder.Decode(&notif); err != nil {
		return nil, fmt.Errorf("failed to decode notification json from NATS message data: %w", err)
	}

	return &notif, nil
}
