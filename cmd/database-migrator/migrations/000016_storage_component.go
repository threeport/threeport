package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000016, Down000016)
}

// Up000016 adds the storage workload to a kubernetes runtime instance.
// A row already there has none, which reads as a runtime with no storage
// attached.  000001 builds its tables from the current models, so a
// database it created after the field was added has it already, and it is
// added only where it is missing.
func Up000016(ctx context.Context, db *sql.DB) error {
	// add the column when an older table does not have it
	statement := "ALTER TABLE v0_kubernetes_runtime_instances ADD COLUMN IF NOT EXISTS storage_component_instance_id INT8"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000016 removes the column, so it also cleans up after Up000016.
func Down000016(ctx context.Context, db *sql.DB) error {
	// drop the column when it is there
	statement := "ALTER TABLE v0_kubernetes_runtime_instances DROP COLUMN IF EXISTS storage_component_instance_id"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
