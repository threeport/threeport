package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000006, Down000006)
}

// Up000006 adds from_namespace to the control plane instance table.
// A restored control plane records the namespace it replaced. 000001 builds
// its tables from the current models, so a database created after the field
// was added has it already, and it is added only where it is missing.
func Up000006(ctx context.Context, db *sql.DB) error {
	// add from_namespace on the control plane instance
	statement := "ALTER TABLE v0_control_plane_instances " +
		"ADD COLUMN IF NOT EXISTS from_namespace STRING"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000006 drops from_namespace from the control plane instance table.
func Down000006(ctx context.Context, db *sql.DB) error {
	// drop from_namespace on the control plane instance
	statement := "ALTER TABLE v0_control_plane_instances DROP COLUMN IF EXISTS from_namespace"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
