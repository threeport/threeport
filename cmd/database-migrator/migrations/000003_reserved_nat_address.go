package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000003, Down000003)
}

// Up000003 adds whether a GKE cluster's Cloud NAT uses an address reserved
// for it.  Existing clusters were created without one, which false records.
// 000001 builds its tables from the current models, so a database it created
// after the field was added has it already, and it is added only where it is
// missing.
func Up000003(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_gcp_gke_kubernetes_runtime_instances ADD COLUMN IF NOT EXISTS reserved_nat_address BOOL DEFAULT false"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000003 removes the column if it is there, so it also cleans up after
// Up000003 failing.
func Down000003(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_gcp_gke_kubernetes_runtime_instances DROP COLUMN IF EXISTS reserved_nat_address"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
