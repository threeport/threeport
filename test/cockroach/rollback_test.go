package cockroach

import (
	"testing"

	migrationtest "github.com/threeport/threeport/pkg/migrationtest/v0"
)

// TestEveryMigrationRollsBack enforces the convention a failed upgrade's
// automatic rollback depends on: every migration has a tested down
// migration, which reverts it and also cleans up after it failing part way.
func TestEveryMigrationRollsBack(t *testing.T) {
	migrationtest.AssertMigrationsRollBackOn(
		t,
		freshDatabase(t, "rollback_round_trip"),
		freshDatabase(t, "rollback_partial"),
		"threeport_goose_db_version",
	)
}
