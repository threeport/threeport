package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000015, Down000015)
}

// gkeNodeSecurityTables are the tables that record a GKE cluster's secrets
// encryption, boot disk, Secure Boot and intranode visibility: the
// definition, as asked for, and the instance, as applied.
var gkeNodeSecurityTables = []string{
	"v0_gcp_gke_kubernetes_runtime_definitions",
	"v0_gcp_gke_kubernetes_runtime_instances",
}

// gkeNodeSecurityColumns are the columns of those settings, with their types.
var gkeNodeSecurityColumns = [][2]string{
	{"secrets_encryption_key", "STRING"},
	{"boot_disk_type", "STRING"},
	{"boot_disk_size_gb", "INT8"},
	{"boot_disk_kms_key", "STRING"},
	{"secure_boot", "BOOL"},
	{"intranode_visibility", "BOOL"},
}

// Up000015 adds a GKE cluster's secrets encryption, boot disk, Secure Boot
// and intranode visibility to its definition and its instance.  A row
// already there has none of them, which leaves its clusters as they are.
// 000001 builds its tables from the current models, so a database it
// created after the fields were added has them already, and each is added
// only where it is missing.
func Up000015(ctx context.Context, db *sql.DB) error {
	for _, table := range gkeNodeSecurityTables {
		for _, column := range gkeNodeSecurityColumns {
			statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", table, column[0], column[1])
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("failed to run %q: %w", statement, err)
			}
		}
	}

	return nil
}

// Down000015 removes each column that is there, so it also cleans up after
// Up000015 failing part way.
func Down000015(ctx context.Context, db *sql.DB) error {
	for _, table := range gkeNodeSecurityTables {
		for _, column := range gkeNodeSecurityColumns {
			statement := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", table, column[0])
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("failed to run %q: %w", statement, err)
			}
		}
	}

	return nil
}
