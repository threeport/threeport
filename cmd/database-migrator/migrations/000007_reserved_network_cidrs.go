package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000007, Down000007)
}

// Up000007 adds the network ranges a control plane reserves for the clusters
// of its deleted children.  A control plane already there has deleted none
// since it began allocating ranges, so it has none.  000001 builds its tables
// from the current models, so a database it created after the field was
// added has it already, and it is added only where it is missing.
func Up000007(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_control_plane_instances ADD COLUMN IF NOT EXISTS reserved_network_cidrs JSONB"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000007 removes the column if it is there, so it also cleans up after
// Up000007 failing.
func Down000007(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_control_plane_instances DROP COLUMN IF EXISTS reserved_network_cidrs"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
