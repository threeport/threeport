package v0

import (
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// A write to the database and a publish to JetStream are two independent
// operations. If the write commits and the publish fails, no controller hears
// about the change and nothing records that a notification was owed. The
// transactional outbox closes that gap. The notification is recorded in the
// same transaction as the write, so it commits or rolls back with it. After the
// commit it is published right away, and anything that is not acknowledged is
// published later by the sweeper.
//
// Delivery is at least once. The sweeper can publish a message, crash before it
// stamps it published, and publish it again. Every publish carries the message
// id as the JetStream message id so a repeat inside the stream's duplicate
// window is dropped by the server, and a repeat outside of it costs a
// controller one redundant reconcile, which reconcilers are written to
// tolerate.

const (
	// OutboxSweepInterval is how often the sweeper looks for pending messages.
	OutboxSweepInterval = 5 * time.Second

	// OutboxSweepMaxBackoff caps how far the sweeper slows down while a publish
	// keeps failing, so a NATS outage is not logged every interval.
	OutboxSweepMaxBackoff = time.Minute

	// OutboxSweepGracePeriod is how old a pending message has to be before the
	// sweeper takes it. The handler that wrote the message is publishing it right
	// now and the sweeper stays out of its way. A message still pending after
	// this long is one the handler failed to publish or never got to.
	OutboxSweepGracePeriod = 10 * time.Second

	// OutboxSweepBatchSize is the most messages one sweep publishes. The sweep
	// holds a transaction open while it publishes, so it is kept short.
	OutboxSweepBatchSize = 20

	// OutboxWarnAfterAttempts is the number of failed publishes after which a
	// Warning event is recorded against the object, so the problem is visible
	// to a user and not only in the API logs.
	OutboxWarnAfterAttempts = 5

	// outboxLastErrorMax bounds the error text stored on a message.
	outboxLastErrorMax = 500

	// outboxEventReason is the reason on the event recorded for a message that
	// cannot be published.
	outboxEventReason = "NotificationPublishFailed"

	// outboxEventController is the reporting controller on that event.
	outboxEventController = "api-server"

	// outboxEventTypeWarning is the type of that event.
	outboxEventTypeWarning = "Warning"
)

// Publisher is the part of a JetStream context that publishing notifications
// needs. nats.JetStreamContext satisfies it.
type Publisher interface {
	Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Notifiable is implemented by every API object that has a controller.
type Notifiable interface {
	NotificationPayload(
		operation notifications.NotificationOperation,
		requeue bool,
		creationTime int64,
	) (*[]byte, error)
	GetId() uint
	GetFullyQualifiedType() string
}

// Notification is a controller notification that a write owes.
type Notification struct {
	// Subject is the JetStream subject to publish on.
	Subject string

	// Payload is the notification to publish.
	Payload []byte

	// ObjectType is the fully qualified type of the object the write changed.
	ObjectType string

	// ObjectID is the id of the object the write changed.
	ObjectID uint
}

// NewNotification builds the notification an operation on object owes the
// controller that subscribes to subject. The object has to be in the state the
// controller is to be told about, so for a create it has to be created.
func NewNotification(
	object Notifiable,
	operation notifications.NotificationOperation,
	subject string,
) (*Notification, error) {
	payload, err := object.NotificationPayload(operation, false, time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to create notification payload: %w", err)
	}

	return &Notification{
		Subject:    subject,
		Payload:    *payload,
		ObjectType: object.GetFullyQualifiedType(),
		ObjectID:   object.GetId(),
	}, nil
}

// RecordNotification inserts the notification into the outbox using tx, which
// has to be the transaction of the write the notification announces.
func RecordNotification(tx *gorm.DB, notification *Notification) (*v0.OutboxMessage, error) {
	message := &v0.OutboxMessage{
		Subject:    notification.Subject,
		Payload:    notification.Payload,
		ObjectType: notification.ObjectType,
		ObjectID:   notification.ObjectID,
	}

	// tx may carry the request's query scopes, which have no business on this
	// insert. A new session keeps the transaction and drops them.
	if err := tx.Session(&gorm.Session{NewDB: true}).Create(message).Error; err != nil {
		return nil, fmt.Errorf("failed to record notification for %s %d: %w", notification.ObjectType, notification.ObjectID, err)
	}

	return message, nil
}

// PublishOutboxMessage publishes a recorded message and stamps it published
// once JetStream acknowledges it. A failure is logged, counted on the message
// and returned, and leaves the message pending for the sweeper. db is the
// handle used to stamp the message; pass one that is not tied to a request so a
// client that hung up cannot stop the stamp.
func PublishOutboxMessage(
	db *gorm.DB,
	publisher Publisher,
	logger *zap.Logger,
	message *v0.OutboxMessage,
) error {
	if publisher == nil {
		err := errors.New("no JetStream context to publish with")
		recordPublishFailure(db, logger, message, err)
		return err
	}

	if _, err := publisher.Publish(
		message.Subject,
		message.Payload,
		nats.MsgId(fmt.Sprintf("outbox-%d", message.ID)),
	); err != nil {
		recordPublishFailure(db, logger, message, err)
		return fmt.Errorf("failed to publish notification to %s: %w", message.Subject, err)
	}

	now := time.Now().UTC()
	if err := db.Model(&v0.OutboxMessage{}).
		Where("id = ? AND published_at IS NULL", message.ID).
		Update("published_at", now).Error; err != nil {
		// the message went out. The sweeper publishes it again and JetStream
		// drops the repeat
		logger.Warn(
			"published notification but failed to mark it published",
			zap.Uint("outboxMessageId", message.ID),
			zap.String("subject", message.Subject),
			zap.Error(err),
		)
	}
	message.PublishedAt = &now

	return nil
}

// recordPublishFailure logs a failed publish, counts it on the message and
// records a Warning event against the object once failures pile up.
func recordPublishFailure(
	db *gorm.DB,
	logger *zap.Logger,
	message *v0.OutboxMessage,
	publishErr error,
) {
	message.Attempts++
	message.LastError = publishErr.Error()
	if len(message.LastError) > outboxLastErrorMax {
		message.LastError = message.LastError[:outboxLastErrorMax]
	}

	logger.Warn(
		"failed to publish controller notification, it will be retried",
		zap.Uint("outboxMessageId", message.ID),
		zap.String("subject", message.Subject),
		zap.String("objectType", message.ObjectType),
		zap.Uint("objectId", message.ObjectID),
		zap.Int("attempts", message.Attempts),
		zap.Error(publishErr),
	)

	if err := db.Model(&v0.OutboxMessage{}).
		Where("id = ?", message.ID).
		Updates(map[string]interface{}{
			"attempts":   gorm.Expr("attempts + 1"),
			"last_error": message.LastError,
		}).Error; err != nil {
		logger.Error(
			"failed to record publish failure on outbox message",
			zap.Uint("outboxMessageId", message.ID),
			zap.Error(err),
		)
	}

	// exactly once, so a message that never publishes does not raise an event
	// on every attempt
	if message.Attempts == OutboxWarnAfterAttempts {
		if err := recordPublishWarning(db, message); err != nil {
			logger.Error(
				"failed to record event for unpublished notification",
				zap.Uint("outboxMessageId", message.ID),
				zap.Error(err),
			)
		}
	}
}

// recordPublishWarning records a Warning event against the object a message is
// about. The event's note is the same for every message on a subject so the
// event dedup index folds a repeat into a count.
func recordPublishWarning(db *gorm.DB, message *v0.OutboxMessage) error {
	now := time.Now()
	reason := outboxEventReason
	note := fmt.Sprintf(
		"controllers are not being notified of changes to this object: failed to publish to %s, the API server is retrying",
		message.Subject,
	)
	eventType := outboxEventTypeWarning
	controller := outboxEventController
	count := uint(1)
	objectType := message.ObjectType
	objectID := message.ObjectID

	return db.Create(&v0.Event{
		Reason:              &reason,
		Note:                &note,
		Type:                &eventType,
		ReportingController: &controller,
		Count:               &count,
		EventTime:           &now,
		LastObservedTime:    &now,
		ObjectType:          &objectType,
		ObjectID:            &objectID,
	}).Error
}

// SweepOutbox publishes the pending messages that handlers did not publish. It
// takes the oldest messages first and stops at the first failure: a publish that
// failed is likely to be followed by more, and the rest wait their turn so
// messages go out in the order they were recorded. It reports whether a publish
// failed. Messages are locked while they are published, so sweepers in other
// API replicas skip them.
func SweepOutbox(
	db *gorm.DB,
	publisher Publisher,
	logger *zap.Logger,
) (published int, failed bool, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		var messages []v0.OutboxMessage
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL AND created_at < ?", time.Now().UTC().Add(-OutboxSweepGracePeriod)).
			Order("id asc").
			Limit(OutboxSweepBatchSize).
			Find(&messages).Error; err != nil {
			return fmt.Errorf("failed to find pending outbox messages: %w", err)
		}

		for i := range messages {
			if err := PublishOutboxMessage(tx, publisher, logger, &messages[i]); err != nil {
				// the failure is already recorded on the message. Commit what
				// was published and recorded so far.
				failed = true
				return nil
			}
			published++
		}

		return nil
	})
	if err != nil {
		return 0, false, err
	}

	return published, failed, nil
}

// RunOutboxSweeper publishes pending outbox messages in the background for the
// life of the process. It sweeps every OutboxSweepInterval and backs off, up to
// OutboxSweepMaxBackoff, while publishes keep failing.
func RunOutboxSweeper(db *gorm.DB, publisher Publisher, logger *zap.Logger) {
	go func() {
		delay := OutboxSweepInterval
		for {
			time.Sleep(delay)

			published, failed, err := SweepOutbox(db, publisher, logger)
			if err != nil {
				logger.Error("failed to sweep outbox", zap.Error(err))
			}
			if published > 0 {
				logger.Info("published pending controller notifications", zap.Int("count", published))
			}

			if failed || err != nil {
				delay = min(delay*2, OutboxSweepMaxBackoff)
			} else {
				delay = OutboxSweepInterval
			}
		}
	}()
}
