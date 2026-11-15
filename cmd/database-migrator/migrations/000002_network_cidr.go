package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000002, Down000002)
}

// Up000002 adds the network range a kubernetes runtime instance's cluster is
// laid out in, which no two instances not deleted share, and the pool a
// control plane instance allocates ranges from.  000001 builds its tables
// from the current models, so a database it created after these were added
// has them already, and each is added only where it is missing.
func Up000002(ctx context.Context, db *sql.DB) error {
	return execStatements(ctx, db,
		"ALTER TABLE v0_kubernetes_runtime_instances ADD COLUMN IF NOT EXISTS network_cidr STRING",
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_v0_kubernetes_runtime_instances_network_cidr
			ON v0_kubernetes_runtime_instances (network_cidr) WHERE deleted_at IS NULL`,
		"ALTER TABLE v0_control_plane_instances ADD COLUMN IF NOT EXISTS network_cidr_pool JSONB",
	)
}

// Down000002 removes what Up000002 adds, whichever of it is there, so it also
// cleans up after Up000002 failing part way.
func Down000002(ctx context.Context, db *sql.DB) error {
	return execStatements(ctx, db,
		"DROP INDEX IF EXISTS v0_kubernetes_runtime_instances@idx_v0_kubernetes_runtime_instances_network_cidr",
		"ALTER TABLE v0_kubernetes_runtime_instances DROP COLUMN IF EXISTS network_cidr",
		"ALTER TABLE v0_control_plane_instances DROP COLUMN IF EXISTS network_cidr_pool",
	)
}

// execStatements runs statements in order, outside a transaction, since
// CockroachDB cannot change a schema inside one.
func execStatements(ctx context.Context, db *sql.DB, statements ...string) error {
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to run %q: %w", statement, err)
		}
	}

	return nil
}
