package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(Up00008, Down00008)
}

func Up00008(ctx context.Context, tx *sql.Tx) error {
	// remove the `type` column
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE attached_object_references
		DROP COLUMN type;
	`); err != nil {
		return fmt.Errorf("failed to drop type column: %w", err)
	}

	// remove the `workload_instance_id` column
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE attached_object_references
		DROP COLUMN workload_instance_id;
	`); err != nil {
		return fmt.Errorf("failed to drop type column: %w", err)
	}

	return nil
}

func Down00008(ctx context.Context, tx *sql.Tx) error {
	// add the 'workload_instance_id' column
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE attached_object_references
		ADD COLUMN workload_instance_id bigint;
	`); err != nil {
		return fmt.Errorf("failed to add workload_instance_id column: %w", err)
	}

	// add the `type` column
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE attached_object_references
		ADD COLUMN type varchar(255);
	`); err != nil {
		return fmt.Errorf("failed to add type column: %w", err)
	}

	return nil
}
