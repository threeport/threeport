package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000010, Down000010)
}

// Up000010 adds the module install request on the control plane instance.
// 000001 builds its tables from the current models, so a database it created
// after the field was added has the column already.
func Up000010(ctx context.Context, db *sql.DB) error {
	// add module_install on the control plane instance
	statement := "ALTER TABLE v0_control_plane_instances " +
		"ADD COLUMN IF NOT EXISTS module_install STRING"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000010 drops the module install request.
func Down000010(ctx context.Context, db *sql.DB) error {
	// drop module_install on the control plane instance
	statement := "ALTER TABLE v0_control_plane_instances DROP COLUMN IF EXISTS module_install"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
