package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// init registers the module image migration.
func init() {
	// register the up and down steps
	goose.AddMigrationNoTxContext(Up000006, Down000006)
}

// Up000006 adds the image repository and tag a module was installed from.
// Cockroach rejects ADD COLUMN inside a transaction, so this migration runs outside one.
func Up000006(ctx context.Context, db *sql.DB) error {
	// add the image a module was installed from
	for _, statement := range []string{
		`ALTER TABLE v0_module_apis ADD COLUMN image_namespace STRING`,
		`ALTER TABLE v0_module_apis ADD COLUMN image_tag STRING`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to add module image columns: %w", err)
		}
	}
	return nil
}

// Down000006 drops the recorded module image.
func Down000006(ctx context.Context, db *sql.DB) error {
	// drop the recorded image
	if _, err := db.ExecContext(ctx, `
ALTER TABLE v0_module_apis DROP COLUMN IF EXISTS image_tag;
ALTER TABLE v0_module_apis DROP COLUMN IF EXISTS image_namespace;
`); err != nil {
		return fmt.Errorf("failed to drop module image columns: %w", err)
	}
	return nil
}
