package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(Up00010, Down00010)
}

func Up00010(ctx context.Context, tx *sql.Tx) error {
	// add foreign key constraint
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE events
		ADD CONSTRAINT fk_attached_object_reference_id
		FOREIGN KEY (attached_object_reference_id)
		REFERENCES attached_object_references(id);
	`); err != nil {
		return fmt.Errorf("failed to add unique constraint on object_id: %w", err)
	}

	return nil
}

func Down00010(ctx context.Context, tx *sql.Tx) error {
	// drop foreign key constraint
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE events
		DROP CONSTRAINT fk_attached_object_reference_id;
	`); err != nil {
		return fmt.Errorf("failed to drop foreign key constraint: %w", err)
	}
	return nil
}
