package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	migrations "github.com/threeport/threeport/cmd/database-migrator/migrations"
)

// Second AutoMigrate on CockroachDB tries to drop a unique constraint that
// was never created. The initial migration therefore only creates missing tables.

// nameGuarded is a model with the partial unique index API name fields use.
type nameGuarded struct {
	gorm.Model
	Name *string `gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`
}

// lateArrival is a table an interrupted install never reached.
type lateArrival struct {
	gorm.Model
	Name *string `gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`
}

// TestRepeatedAutoMigrateIsRejected covers a second AutoMigrate on the same table.
func TestRepeatedAutoMigrateIsRejected(t *testing.T) {
	db := freshDatabase(t, "automigrate_repeat")

	require.NoError(t, db.AutoMigrate(&nameGuarded{}),
		"the first pass builds the table")

	// second pass tries to drop uni_<table>_name, which was never created
	err := db.AutoMigrate(&nameGuarded{})
	require.Error(t, err, "the second pass is rejected")
	assert.Contains(t, err.Error(), "does not exist",
		"the rejection is the drop of a constraint that was never created")
}

// TestCreatingMissingTablesCompletesAPartialSchema covers creating only missing tables.
func TestCreatingMissingTablesCompletesAPartialSchema(t *testing.T) {
	db := freshDatabase(t, "create_missing_tables")

	// first pass: one of two tables
	createMissingTables(t, db, &nameGuarded{})

	// second pass: the missing table, leaving the first one's index in place
	createMissingTables(t, db, &nameGuarded{}, &lateArrival{})
	assert.True(t, db.Migrator().HasTable(&lateArrival{}),
		"the table the first run never reached is built")

	name := "one-per-name"
	require.NoError(t, db.Create(&nameGuarded{Name: &name}).Error,
		"the first row is accepted")
	assert.Error(t, db.Create(&nameGuarded{Name: &name}).Error,
		"a second row under the same name is still refused")
}

// TestInitialMigrationIsIdempotent covers running Up000001 twice.
func TestInitialMigrationIsIdempotent(t *testing.T) {
	db := freshDatabase(t, "initial_migration_idempotent")

	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)

	require.NoError(t, migrations.Up000001(ctx, nil),
		"the first migration builds the schema")
	// existing tables are left alone
	require.NoError(t, migrations.Up000001(ctx, nil),
		"the second migration finds the schema already built and adds nothing")
}

// createMissingTables creates a table for each model that has none.
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
