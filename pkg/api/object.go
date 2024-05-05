package api

import (
	"time"

	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// ReconciledThreeportApiObject is the interface each reconciled object in the
// Threeport API must satisfy for compatibility with the controlllers.
type ReconciledThreeportApiObject interface {
	NotificationPayload(
		operation notifications.NotificationOperation,
		requeue bool,
		creationTime int64,
	) (*[]byte, error)
	DecodeNotifObject(object interface{}) error
	GetId() uint
	Type() string
	Version() string
	ScheduledForDeletion() *time.Time
}
