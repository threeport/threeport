package cockroach

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	migrations "github.com/threeport/threeport/cmd/database-migrator/migrations"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestOutboxMigrationBuildsTableAndIsIdempotent covers the outbox migration,
// including the row-level TTL that keeps published messages from piling up.
func TestOutboxMigrationBuildsTableAndIsIdempotent(t *testing.T) {
	db := freshDatabase(t, "outbox_migration")

	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)

	require.NoError(t, migrations.Up000002(ctx, nil), "the migration builds the table")
	require.NoError(t, migrations.Up000002(ctx, nil), "a second run finds it built and changes nothing")
	require.True(t, db.Migrator().HasTable(&api_v0.OutboxMessage{}))

	// only published rows expire, so a pending message is never deleted
	var table, createStatement string
	require.NoError(t, db.Raw("SHOW CREATE TABLE v0_outbox_messages").Row().Scan(&table, &createStatement))
	assert.Contains(t, createStatement, "ttl_expiration_expression")
	assert.Contains(t, createStatement, "published_at IS NOT NULL")

	// the partial index the sweeper scans
	assert.Contains(t, createStatement, "idx_outbox_pending")

	require.NoError(t, migrations.Down000002(ctx, nil))
	assert.False(t, db.Migrator().HasTable(&api_v0.OutboxMessage{}))
}

// slowPublisher publishes slowly enough that concurrent sweepers overlap.
type slowPublisher struct {
	mu        sync.Mutex
	published map[string]int
}

func (p *slowPublisher) Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	time.Sleep(20 * time.Millisecond)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.published[subj]++

	return &nats.PubAck{}, nil
}

// TestSweepersInDifferentReplicasDoNotPublishTheSameMessage covers two API
// replicas sweeping one outbox. Messages a sweeper has locked are skipped by
// the other, so each goes out once.
func TestSweepersInDifferentReplicasDoNotPublishTheSameMessage(t *testing.T) {
	db := freshDatabase(t, "outbox_sweepers")

	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	require.NoError(t, migrations.Up000002(ctx, nil))

	const messages = apiserver_lib.OutboxSweepBatchSize * 2
	for i := 0; i < messages; i++ {
		require.NoError(t, db.Create(&api_v0.OutboxMessage{
			Subject:    fmt.Sprintf("subject-%02d", i),
			Payload:    []byte("{}"),
			ObjectType: "threeport.io/v0.KubernetesWorkloadDefinition",
			ObjectID:   uint(i + 1),
			CreatedAt:  time.Now().UTC().Add(-time.Minute),
		}).Error)
	}

	publisher := &slowPublisher{published: map[string]int{}}

	// two replicas sweep at the same time until the outbox is drained
	var wg sync.WaitGroup
	var swept [2]int
	for replica := range swept {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 10; attempt++ {
				published, _, err := apiserver_lib.SweepOutbox(db, publisher, zap.NewNop())
				if err != nil {
					// a serialization failure is the sweeper's to retry on its next tick
					continue
				}
				swept[replica] += published
				if published == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	var pending int64
	require.NoError(t, db.Model(&api_v0.OutboxMessage{}).Where("published_at IS NULL").Count(&pending).Error)
	assert.Zero(t, pending, "the sweepers drain the outbox")

	require.Len(t, publisher.published, messages)
	for subject, count := range publisher.published {
		assert.Equal(t, 1, count, "%s was published by both replicas", subject)
	}
	assert.Equal(t, messages, swept[0]+swept[1])
}
