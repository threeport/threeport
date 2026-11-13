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

// Up000010 makes 60 the NodeMaximum of a kubernetes runtime definition
// created without one, rather than 250.  It changes only the column's
// default: a definition already there keeps the maximum it records, and its
// clusters keep the node groups they were created with.  Setting a default
// is the same whether or not it is already set, so the migration is
// idempotent.
func Up000010(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_kubernetes_runtime_definitions ALTER COLUMN node_maximum SET DEFAULT 60"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}

// Down000010 restores the default of 250.
func Down000010(ctx context.Context, db *sql.DB) error {
	statement := "ALTER TABLE v0_kubernetes_runtime_definitions ALTER COLUMN node_maximum SET DEFAULT 250"
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("failed to run %q: %w", statement, err)
	}

	return nil
}
