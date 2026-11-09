package cockroach

import (
	"testing"

	goose "github.com/pressly/goose/v3"
	"gorm.io/gorm"

	migrationtest "github.com/threeport/threeport/pkg/migrationtest/v0"
)

// TestEveryMigrationRollsBack enforces the convention a failed upgrade's
// automatic rollback depends on: every migration has a tested down
// migration, which reverts it and also cleans up after it failing part way.
func TestEveryMigrationRollsBack(t *testing.T) {
	migrationtest.AssertMigrationsRollBackOn(
		t,
		func(name string) *gorm.DB { return freshDatabase(t, name) },
		"threeport_goose_db_version",
	)
}

// TestADownMigrationIsTriedWhereItsMigrationStarted covers the state each
// down migration is checked against: the database as the migrations before
// it left it.  A migration that adds a column to an earlier migration's table
// has a down migration that needs that table, and is correct.
func TestADownMigrationIsTriedWhereItsMigrationStarted(t *testing.T) {
	createTable := testMigration(1,
		execAll("CREATE TABLE widgets (id INT8 PRIMARY KEY)"),
		execAll("DROP TABLE IF EXISTS widgets"),
	)
	addColumn := testMigration(2,
		execAll("ALTER TABLE widgets ADD COLUMN name STRING"),
		execAll("ALTER TABLE widgets DROP COLUMN IF EXISTS name"),
	)

	previous := goose.TableName()
	t.Cleanup(func() { goose.SetTableName(previous) })
	migrationtest.AssertMigrationChainRollsBackOn(
		t,
		goose.Migrations{createTable, addColumn},
		func(name string) *gorm.DB { return freshDatabase(t, "chain_"+name) },
		"chain_goose_db_version",
	)
}
