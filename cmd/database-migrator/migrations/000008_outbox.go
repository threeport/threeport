package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// outboxRetention is how long a published outbox message is kept before the
// CockroachDB row-level TTL hard deletes it. Pending messages never expire.
const outboxRetention = "1 day"

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000008, Down000008)
}

// Up000008 creates the controller notification outbox and sets a TTL on its
// published rows. An existing table is left alone so a partial run can finish.
func Up000008(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := createMissingTables(gormDb, dbInterfaces000008()); err != nil {
		return err
	}

	// partial row-level time-to-live: only published messages expire
	if err := gormDb.Exec(fmt.Sprintf(`
		ALTER TABLE %s SET (
			ttl_expiration_expression = $$
				CASE
					WHEN published_at IS NOT NULL
						THEN published_at + INTERVAL '%s'
					ELSE NULL
				END
			$$,
			ttl_job_cron = '@hourly'
		)
	`, (v0.OutboxMessage{}).TableName(), outboxRetention)).Error; err != nil {
		return fmt.Errorf("failed to set time-to-live on %s: %w", (v0.OutboxMessage{}).TableName(), err)
	}

	return nil
}

// Down000008 drops the outbox table.
func Down000008(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	return dropTables(gormDb, dbInterfaces000008())
}

func dbInterfaces000008() []interface{} {
	return []interface{}{
		&v0.OutboxMessage{},
	}
}
