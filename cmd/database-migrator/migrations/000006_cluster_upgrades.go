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

// clusterUpgradeTables are the EKS and GKE kubernetes runtime instance tables
// that record a cluster's version and its upgrades.
var clusterUpgradeTables = []string{
	"v0_aws_eks_kubernetes_runtime_instances",
	"v0_gcp_gke_kubernetes_runtime_instances",
}

// clusterUpgradeColumns are the columns of an instance's version and
// upgrades, with their types.
var clusterUpgradeColumns = [][2]string{
	{"kubernetes_version", "STRING"},
	{"upgrade_requested_version", "STRING"},
	{"upgrade_acknowledged", "TIMESTAMPTZ"},
	{"upgrading", "BOOL DEFAULT false"},
	{"upgrade_failed", "BOOL DEFAULT false"},
	{"upgrade_refused", "BOOL DEFAULT false"},
	{"upgrade_status", "STRING"},
}

// Up000006 adds an EKS or GKE instance's version and upgrade state.  An
// instance already there has no version until its controller reads it from
// the provider, and isn't upgrading.  000001 builds its tables from the
// current models, so a database it created after the fields were added has
// them already, and each is added only where it is missing.
func Up000006(ctx context.Context, db *sql.DB) error {
	for _, table := range clusterUpgradeTables {
		for _, column := range clusterUpgradeColumns {
			statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", table, column[0], column[1])
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("failed to run %q: %w", statement, err)
			}
		}
	}

	return nil
}

// Down000006 removes each column that is there, so it also cleans up after
// Up000006 failing part way.
func Down000006(ctx context.Context, db *sql.DB) error {
	for _, table := range clusterUpgradeTables {
		for _, column := range clusterUpgradeColumns {
			statement := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", table, column[0])
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("failed to run %q: %w", statement, err)
			}
		}
	}

	return nil
}
