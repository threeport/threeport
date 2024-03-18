package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(Up00006, Down00006)
}

func Up00006(ctx context.Context, tx *sql.Tx) error {
	// create new columns
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE attached_object_references
		ADD COLUMN attached_object_type varchar(255),
		ADD COLUMN attached_object_id bigint,
		ADD COLUMN object_type varchar(255);
	`); err != nil {
		return err
	}

	return nil
}

func Down00006(ctx context.Context, tx *sql.Tx) error {
	// drop new columns
	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE attached_object_references
		DROP COLUMN attached_object_type,
		DROP COLUMN attached_object_id,
		DROP COLUMN object_type;
	`); err != nil {
		return err
	}

	return nil
}
