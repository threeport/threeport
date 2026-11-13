package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000009, Down000009)
}

// deletionProtectionTables are the kubernetes runtime definition tables that
// record whether their clusters are protected from deletion.
var deletionProtectionTables = []string{
	"v0_kubernetes_runtime_definitions",
	"v0_aws_eks_kubernetes_runtime_definitions",
	"v0_gcp_gke_kubernetes_runtime_definitions",
	"v0_oci_oke_kubernetes_runtime_definitions",
}

// Up000009 adds deletion protection to each kubernetes runtime definition.  A
// definition already there isn't protected, as its clusters weren't created
// protected.  000001 builds its tables from the current models, so a database
// it created after the field was added has it already, and it is added only
// where it is missing.
func Up000009(ctx context.Context, db *sql.DB) error {
	for _, table := range deletionProtectionTables {
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS deletion_protection BOOL DEFAULT false", table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}

// Down000009 removes the column from each table that has it, so it also
// cleans up after Up000009 failing part way.
func Down000009(ctx context.Context, db *sql.DB) error {
	for _, table := range deletionProtectionTables {
		statement := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS deletion_protection", table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}
