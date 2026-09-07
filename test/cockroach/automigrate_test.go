package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	migrations "github.com/threeport/threeport/cmd/database-migrator/migrations"
)

// An interrupted install repeats a migration over the tables it already built.
// The initial migration registers with goose as a no-transaction Go migration,
// and goose records the version only once the migration function returns, so a
// run that dies part way through the schema is run again in full.
//
// That is why the migration creates each missing table rather than calling
// gorm's AutoMigrate. AutoMigrate builds a missing table from the struct tags,
// and reads an existing one back out of the catalog to emit corrective DDL for
// every difference it finds there. CockroachDB applies a table-level UNIQUE
// constraint for every unique index, and gorm compares that against the
// column's unique tag rather than its index tags, so a column tagged
// uniqueIndex draws a drop of a constraint named uni_<table>_<column> that no
// run created. See https://docs.cockroachlabs.com/docs/stable/create-index.

// nameGuarded is a model carrying the partial unique index an API definition or
// instance puts on its name column, which CockroachDB reports as a constraint.
type nameGuarded struct {
	gorm.Model
	Name *string `gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`
}

// lateArrival is a second model of the same shape, held back from the first
// pass to stand for a table an interrupted install never reached.
type lateArrival struct {
	gorm.Model
	Name *string `gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`
}

// TestRepeatedAutoMigrateIsRejected asserts a second AutoMigrate over a table
// gorm already built is refused. A failure here means gorm and CockroachDB have
// stopped disagreeing rather than that the schema regressed.
func TestRepeatedAutoMigrateIsRejected(t *testing.T) {
	db := freshDatabase(t, "automigrate_repeat")

	require.NoError(t, db.AutoMigrate(&nameGuarded{}),
		"the first pass builds the table")

	err := db.AutoMigrate(&nameGuarded{})
	require.Error(t, err, "the second pass is rejected")
	assert.Contains(t, err.Error(), "does not exist",
		"the rejection is the drop of a constraint that was never created")
}

// TestCreatingMissingTablesCompletesAPartialSchema asserts a second pass builds
// the table the first pass missed and leaves the first table's index in place.
func TestCreatingMissingTablesCompletesAPartialSchema(t *testing.T) {
	db := freshDatabase(t, "create_missing_tables")

	// build one of the two models, the half-built schema an interrupted run leaves
	createMissingTables(t, db, &nameGuarded{})

	createMissingTables(t, db, &nameGuarded{}, &lateArrival{})
	assert.True(t, db.Migrator().HasTable(&lateArrival{}),
		"the table the first run never reached is built")

	// check the first table's index still refuses a repeated name
	name := "one-per-name"
	require.NoError(t, db.Create(&nameGuarded{Name: &name}).Error,
		"the first row is accepted")
	assert.Error(t, db.Create(&nameGuarded{Name: &name}).Error,
		"a second row under the same name is still refused")
}

// TestInitialMigrationIsIdempotent asserts the initial migration accepts a
// second run over the schema it already built. It calls the deployed migration,
// so every persisted model and the time-to-live statements after it are covered.
func TestInitialMigrationIsIdempotent(t *testing.T) {
	db := freshDatabase(t, "initial_migration_idempotent")

	// a context key matches on type as well as value, so a defined string type
	// here would read back as absent
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)

	// the migration takes its gorm handle off the context, so the sql handle goes unused
	require.NoError(t, migrations.Up000001(ctx, nil),
		"the first migration builds the schema")
	require.NoError(t, migrations.Up000001(ctx, nil),
		"the second migration finds the schema already built and adds nothing")
}

// createMissingTables creates a table for each model that has none, the loop the
// initial migration runs in place of AutoMigrate. It is spelled out here so a
// test can run it over models the migration's own list does not carry.
func createMissingTables(t *testing.T, db *gorm.DB, models ...interface{}) {
	t.Helper()

	for _, model := range models {
		if db.Migrator().HasTable(model) {
			continue
		}
		if err := db.Migrator().CreateTable(model); err != nil {
			t.Fatalf("create table for %T: %v", model, err)
		}
	}
}
