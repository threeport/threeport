package v0

import notifications "github.com/threeport/threeport/pkg/notifications/v0"

type APIObject interface {
	GetID() uint
	NotificationPayload(operation notifications.NotificationOperation, requeue bool, creationTime int64) (*[]byte, error)
}
