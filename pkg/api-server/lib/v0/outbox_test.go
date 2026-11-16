package v0

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	sqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// fakePublisher records what it is asked to publish and fails on request.
type fakePublisher struct {
	mu sync.Mutex

	// failOn holds the 1-based publish calls that fail. A nil map fails none.
	failOn map[int]bool

	// failAll fails every call.
	failAll bool

	// failErr is the error a failing call returns. It defaults to a resource
	// limit error, which a retry can fix.
	failErr error

	calls    int
	subjects []string
}

func (f *fakePublisher) Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	if f.failAll || f.failOn[f.calls] {
		if f.failErr != nil {
			return nil, f.failErr
		}
		return nil, errors.New("nats: JetStream resource limits exceeded for server")
	}
	f.subjects = append(f.subjects, subj)

	return &nats.PubAck{}, nil
}

// newOutboxDb returns an empty sqlite database with the outbox and event tables.
func newOutboxDb(t *testing.T) *gorm.DB {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&v0.OutboxMessage{}, &v0.Event{}))

	return db
}

// addPending records a pending message created at the given age.
func addPending(t *testing.T, db *gorm.DB, subject string, age time.Duration) *v0.OutboxMessage {
	t.Helper()

	message := &v0.OutboxMessage{
		Subject:    subject,
		Payload:    []byte(`{"Operation":"Created"}`),
		ObjectType: "threeport.io/v0.KubernetesWorkloadDefinition",
		ObjectID:   7,
	}
	require.NoError(t, db.Create(message).Error)
	require.NoError(t, db.Model(message).
		UpdateColumn("created_at", time.Now().UTC().Add(-age)).Error)

	return message
}

// reload returns the stored copy of a message.
func reload(t *testing.T, db *gorm.DB, id uint) v0.OutboxMessage {
	t.Helper()

	var message v0.OutboxMessage
	require.NoError(t, db.First(&message, id).Error)

	return message
}

func TestRecordNotificationRollsBackWithTheWrite(t *testing.T) {
	db := newOutboxDb(t)
	notification := &Notification{Subject: "s", Payload: []byte("p"), ObjectType: "t", ObjectID: 1}

	// a write that fails after the notification is recorded leaves no message
	err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := RecordNotification(tx, notification); err != nil {
			return err
		}
		return errors.New("the write failed")
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&v0.OutboxMessage{}).Count(&count).Error)
	assert.Zero(t, count, "a rolled back write must not leave a notification on record")

	// a write that commits leaves exactly one pending message
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := RecordNotification(tx, notification)
		return err
	}))
	require.NoError(t, db.Model(&v0.OutboxMessage{}).Where("published_at IS NULL").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestPublishOutboxMessageStampsOnAck(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{}
	message := addPending(t, db, "workload.create", 0)

	require.NoError(t, PublishOutboxMessage(db, publisher, zap.NewNop(), message))

	assert.Equal(t, []string{"workload.create"}, publisher.subjects)
	assert.NotNil(t, reload(t, db, message.ID).PublishedAt, "an acknowledged message is stamped published")
}

func TestPublishOutboxMessageLeavesFailureOnRecord(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{failAll: true}
	message := addPending(t, db, "workload.create", 0)

	err := PublishOutboxMessage(db, publisher, zap.NewNop(), message)
	require.Error(t, err)

	stored := reload(t, db, message.ID)
	assert.Nil(t, stored.PublishedAt, "a failed publish leaves the message pending")
	assert.Equal(t, 1, stored.Attempts)
	assert.Contains(t, stored.LastError, "resource limits exceeded")
}

func TestPublishOutboxMessageWithoutJetStreamFails(t *testing.T) {
	db := newOutboxDb(t)
	message := addPending(t, db, "workload.create", 0)

	require.Error(t, PublishOutboxMessage(db, nil, zap.NewNop(), message),
		"a missing JetStream context is a failed publish, not a panic")
	assert.Equal(t, 1, reload(t, db, message.ID).Attempts)
}

func TestPublishOutboxMessageWarnsOnceAfterRepeatedFailures(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{failAll: true}
	message := addPending(t, db, "workload.create", 0)

	for attempt := 1; attempt < OutboxWarnAfterAttempts; attempt++ {
		_ = PublishOutboxMessage(db, publisher, zap.NewNop(), message)
	}
	var events int64
	require.NoError(t, db.Model(&v0.Event{}).Count(&events).Error)
	assert.Zero(t, events, "no event before the threshold")

	// the threshold and several failures after it
	for attempt := 0; attempt < 3; attempt++ {
		_ = PublishOutboxMessage(db, publisher, zap.NewNop(), message)
	}

	var stored []v0.Event
	require.NoError(t, db.Find(&stored).Error)
	require.Len(t, stored, 1, "one warning, however long the publish keeps failing")
	assert.Equal(t, "Warning", *stored[0].Type)
	assert.Equal(t, "NotificationPublishFailed", *stored[0].Reason)
	assert.Equal(t, message.ObjectType, *stored[0].ObjectType)
	assert.Equal(t, message.ObjectID, *stored[0].ObjectID)
}

func TestSweepOutboxPublishesOldestFirstAndSkipsFreshMessages(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{}

	second := addPending(t, db, "second", 40*time.Second)
	first := addPending(t, db, "first", 50*time.Second)
	fresh := addPending(t, db, "fresh", time.Second)
	// the order of ids, not of creation times, is the order of recording
	require.Less(t, second.ID, first.ID)

	published, failed, err := SweepOutbox(db, publisher, zap.NewNop())
	require.NoError(t, err)
	assert.False(t, failed)
	assert.Equal(t, 2, published)
	assert.Equal(t, []string{"second", "first"}, publisher.subjects)
	assert.NotNil(t, reload(t, db, second.ID).PublishedAt)
	assert.NotNil(t, reload(t, db, first.ID).PublishedAt)
	assert.Nil(t, reload(t, db, fresh.ID).PublishedAt,
		"a message the handler may still be publishing is left alone")

	// a second sweep has nothing to do
	published, _, err = SweepOutbox(db, publisher, zap.NewNop())
	require.NoError(t, err)
	assert.Zero(t, published)
}

func TestSweepOutboxStopsAtFirstFailure(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{failOn: map[int]bool{2: true}}

	one := addPending(t, db, "one", time.Minute)
	two := addPending(t, db, "two", time.Minute)
	three := addPending(t, db, "three", time.Minute)

	published, failed, err := SweepOutbox(db, publisher, zap.NewNop())
	require.NoError(t, err)
	assert.True(t, failed)
	assert.Equal(t, 1, published)

	assert.NotNil(t, reload(t, db, one.ID).PublishedAt, "what published before the failure stays published")
	failedMessage := reload(t, db, two.ID)
	assert.Nil(t, failedMessage.PublishedAt)
	assert.Equal(t, 1, failedMessage.Attempts, "the failure is counted and kept")
	untouched := reload(t, db, three.ID)
	assert.Nil(t, untouched.PublishedAt)
	assert.Zero(t, untouched.Attempts, "messages behind a failure wait their turn, so order is kept")

	// the next sweep, with JetStream back, drains the rest in order
	published, failed, err = SweepOutbox(db, publisher, zap.NewNop())
	require.NoError(t, err)
	assert.False(t, failed)
	assert.Equal(t, 2, published)
	assert.Equal(t, []string{"one", "two", "three"}, publisher.subjects)
}

func TestPublishOutboxMessageAbandonsWhatNoRetryCanFix(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{failAll: true, failErr: nats.ErrMaxPayload}
	message := addPending(t, db, "workload.create", 0)

	err := PublishOutboxMessage(db, publisher, zap.NewNop(), message)
	require.ErrorIs(t, err, ErrNotificationAbandoned)
	require.ErrorIs(t, err, nats.ErrMaxPayload, "the cause is still reachable")

	stored := reload(t, db, message.ID)
	assert.NotNil(t, stored.AbandonedAt, "the message is given up on")
	assert.Nil(t, stored.PublishedAt, "and stays on record unpublished")
	assert.Equal(t, 1, stored.Attempts)
	assert.Contains(t, stored.LastError, "maximum payload exceeded")

	var events []v0.Event
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 1, "the user is told straight away, not after five attempts")
	assert.Equal(t, "Warning", *events[0].Type)
	assert.Equal(t, message.ObjectID, *events[0].ObjectID)
	assert.Contains(t, *events[0].Note, "will not be retried")
}

func TestSweepOutboxSkipsAbandonedMessageAndPublishesTheRest(t *testing.T) {
	db := newOutboxDb(t)
	publisher := &fakePublisher{failOn: map[int]bool{2: true}, failErr: nats.ErrMaxPayload}

	one := addPending(t, db, "one", time.Minute)
	oversized := addPending(t, db, "oversized", time.Minute)
	three := addPending(t, db, "three", time.Minute)

	published, failed, err := SweepOutbox(db, publisher, zap.NewNop())
	require.NoError(t, err)
	assert.False(t, failed, "an abandoned message is not an outage")
	assert.Equal(t, 2, published)
	assert.Equal(t, []string{"one", "three"}, publisher.subjects)
	assert.NotNil(t, reload(t, db, one.ID).PublishedAt)
	assert.NotNil(t, reload(t, db, three.ID).PublishedAt, "the message behind the bad one still goes out")
	assert.NotNil(t, reload(t, db, oversized.ID).AbandonedAt)

	// the abandoned message is never taken again
	calls := publisher.calls
	published, failed, err = SweepOutbox(db, publisher, zap.NewNop())
	require.NoError(t, err)
	assert.Zero(t, published)
	assert.False(t, failed)
	assert.Equal(t, calls, publisher.calls, "the sweeper leaves an abandoned message alone")
}

func TestIsPermanentPublishError(t *testing.T) {
	for name, tc := range map[string]struct {
		err       error
		permanent bool
	}{
		"payload over the server limit": {nats.ErrMaxPayload, true},
		"wrapped payload error":         {fmt.Errorf("publish: %w", nats.ErrMaxPayload), true},
		"invalid subject":               {nats.ErrBadSubject, true},
		"message over the stream limit": {&nats.APIError{ErrorCode: 10054, Code: 400}, true},
		"wrapped message over stream":   {fmt.Errorf("publish: %w", &nats.APIError{ErrorCode: 10054}), true},
		"timeout":                       {nats.ErrTimeout, false},
		"no responders":                 {nats.ErrNoResponders, false},
		"connection closed":             {nats.ErrConnectionClosed, false},
		"resource limits":               {errors.New("nats: JetStream resource limits exceeded for server"), false},
		"other JetStream error":         {&nats.APIError{ErrorCode: 10077, Code: 400}, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.permanent, isPermanentPublishError(tc.err))
		})
	}
}

func TestNotificationTooLarge(t *testing.T) {
	const limit = 4096
	sized := func(n int) *Notification { return &Notification{Payload: make([]byte, n)} }

	assert.False(t, NotificationTooLarge(sized(limit*10), 0), "an unknown limit rejects nothing")
	assert.False(t, NotificationTooLarge(sized(limit-OutboxPayloadHeadroom), limit))
	assert.True(t, NotificationTooLarge(sized(limit-OutboxPayloadHeadroom+1), limit),
		"headers have to fit in the limit too")
	assert.True(t, NotificationTooLarge(sized(limit*2), limit))
}
