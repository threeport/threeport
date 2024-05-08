package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(Up00011, Down00011)
}

func Up00011(ctx context.Context, tx *sql.Tx) error {
	// events table ttl
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE events SET (ttl_expire_after = '1 minute');
	`); err != nil {
		return fmt.Errorf("failed to add ttl to events table: %w", err)
	}

	return nil
}

func Down00011(ctx context.Context, tx *sql.Tx) error {
	// remove events table ttl
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE events RESET (ttl);
	`); err != nil {
		return fmt.Errorf("failed to remove ttl from events table: %w", err)
	}
	return nil
}
