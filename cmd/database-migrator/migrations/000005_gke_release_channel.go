package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000005, Down000005)
}

// gkeUpgradeWindowColumns are the columns of a GKE definition's release
// channel and maintenance window, with their types.
var gkeUpgradeWindowColumns = [][2]string{
	{"release_channel", "STRING"},
	{"maintenance_window_days", "STRING"},
	{"maintenance_window_start", "STRING"},
	{"maintenance_window_hours", "INT8"},
}

// Up000005 adds a GKE definition's release channel and maintenance window.  A
// definition already there has none, which reads as the defaults.  000001
// builds its tables from the current models, so a database it created after
// the fields were added has them already, and each is added only where it is
// missing.
func Up000005(ctx context.Context, db *sql.DB) error {
	for _, column := range gkeUpgradeWindowColumns {
		statement := fmt.Sprintf(
			"ALTER TABLE v0_gcp_gke_kubernetes_runtime_definitions ADD COLUMN IF NOT EXISTS %s %s",
			column[0], column[1],
		)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}

// Down000005 removes each column that is there, so it also cleans up after
// Up000005 failing part way.
func Down000005(ctx context.Context, db *sql.DB) error {
	for _, column := range gkeUpgradeWindowColumns {
		statement := fmt.Sprintf(
			"ALTER TABLE v0_gcp_gke_kubernetes_runtime_definitions DROP COLUMN IF EXISTS %s",
			column[0],
		)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}
