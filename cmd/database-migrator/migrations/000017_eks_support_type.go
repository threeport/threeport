package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000017, Down000017)
}

// Up000017 adds an EKS definition's support type.  A definition already
// there has none, which reads as the default, STANDARD, the support type
// its clusters were created with since EKS clusters got an upgrade policy;
// a cluster created before keeps the one EKS gave it, as its stack holds.
// 000001 builds its tables from the current models, so a database it
// created after the field was added has it already, and it is added only
// where it is missing.
func Up000017(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_aws_eks_kubernetes_runtime_definitions ADD COLUMN IF NOT EXISTS support_type STRING"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000017 removes the column, if it is there.
func Down000017(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_aws_eks_kubernetes_runtime_definitions DROP COLUMN IF EXISTS support_type"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
