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

// Up000006 adds whether an EKS definition's clusters run a node group in each
// zone.  Existing clusters run one spanning every zone, which false records.
// 000001 builds its tables from the current models, so a database it created
// after the field was added has it already, and it is added only where it is
// missing.
func Up000006(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_aws_eks_kubernetes_runtime_definitions ADD COLUMN IF NOT EXISTS node_group_per_zone BOOL DEFAULT false"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000006 removes the column if it is there, so it also cleans up after
// Up000006 failing.
func Down000006(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_aws_eks_kubernetes_runtime_definitions DROP COLUMN IF EXISTS node_group_per_zone"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
