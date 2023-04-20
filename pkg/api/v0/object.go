// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

import notifications "github.com/threeport/threeport/pkg/notifications"

type APIObject interface {
	GetID() uint
	NotificationPayload(operation notifications.NotificationOperation, requeue bool, lastDelay int64) (*[]byte, error)
}
