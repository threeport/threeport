package v0

import "time"

// OutboxMessage is a controller notification the API owes a write. It is
// inserted in the same database transaction as the write it announces, so a
// committed write always leaves a record that a notification is due. The API
// publishes the message to JetStream after the commit and stamps PublishedAt
// when the publish is acknowledged. A message that is not acknowledged stays
// pending and is retried by the outbox sweeper.
//
// It is not an API object: it has no routes, is never returned to clients and
// is not soft deleted.
type OutboxMessage struct {
	ID uint `gorm:"primarykey"`

	// The time the message was recorded. The partial index keeps the sweeper's
	// scan for pending messages small however many published rows are retained.
	CreatedAt time.Time `gorm:"index:idx_outbox_pending,where:published_at IS NULL"`

	// The JetStream subject to publish the payload on.
	Subject string `gorm:"not null"`

	// The exact notification payload to publish.
	Payload []byte `gorm:"not null"`

	// The fully qualified type of the object the notification is about. Used to
	// attach an event to the object when the message cannot be published.
	ObjectType string `gorm:"not null"`

	// The id of the object the notification is about.
	ObjectID uint `gorm:"not null"`

	// The time the publish was acknowledged. Nil while the message is pending.
	PublishedAt *time.Time

	// The number of publish attempts that have failed.
	Attempts int `gorm:"not null;default:0"`

	// The error from the most recent failed publish attempt.
	LastError string
}

// TableName sets the name of the table for the OutboxMessage objects in the database.
func (OutboxMessage) TableName() string {
	return "v0_outbox_messages"
}
