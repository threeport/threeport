package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000004, Down000004)
}

// kubernetesVersionTables are the kubernetes runtime definition tables that
// record the Kubernetes version their clusters are created on.
var kubernetesVersionTables = []string{
	"v0_kubernetes_runtime_definitions",
	"v0_aws_eks_kubernetes_runtime_definitions",
	"v0_gcp_gke_kubernetes_runtime_definitions",
	"v0_oci_oke_kubernetes_runtime_definitions",
}

// Up000004 adds the Kubernetes version to each kubernetes runtime definition.
// A definition already there has none, which reads as one created before
// versions were recorded.  000001 builds its tables from the current models,
// so a database it created after the field was added has it already, and it
// is added only where it is missing.
func Up000004(ctx context.Context, db *sql.DB) error {
	for _, table := range kubernetesVersionTables {
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS kubernetes_version STRING", table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}

// Down000004 removes the column from each table that has it, so it also
// cleans up after Up000004 failing part way.
func Down000004(ctx context.Context, db *sql.DB) error {
	for _, table := range kubernetesVersionTables {
		statement := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS kubernetes_version", table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}
