package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000011, Down000011)
}

// auditLogTables are the kubernetes runtime definition tables that record
// whether their clusters' Kubernetes API audit log is enabled.
var auditLogTables = []string{
	"v0_kubernetes_runtime_definitions",
	"v0_aws_eks_kubernetes_runtime_definitions",
	"v0_gcp_gke_kubernetes_runtime_definitions",
	"v0_oci_oke_kubernetes_runtime_definitions",
}

// Up000011 adds the audit log to each kubernetes runtime definition.  A
// definition already there has none, as its clusters were created without
// one.  000001 builds its tables from the current models, so a database it
// created after the field was added has it already, and it is added only
// where it is missing.
func Up000011(ctx context.Context, db *sql.DB) error {
	for _, table := range auditLogTables {
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS audit_log BOOL DEFAULT false", table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}

// Down000011 removes the column from each table that has it, so it also
// cleans up after Up000011 failing part way.
func Down000011(ctx context.Context, db *sql.DB) error {
	for _, table := range auditLogTables {
		statement := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS audit_log", table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}
