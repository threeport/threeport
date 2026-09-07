// Package migrationtest holds helpers that check a database migration chain
// against the models the chain is meant to persist. It imports testing, so only
// a test file imports it.
package migrationtest

import (
	"context"
	"sort"
	"testing"

	goose "github.com/pressly/goose/v3"
	sqlite "gorm.io/driver/sqlite"
	gorm "gorm.io/gorm"
)

// threeport-sdk generates a schema drift test for the core API and for every
// module, and both forms call in here so the comparison is written once. The
// migrations they run are Go migrations that register themselves with goose at
// import time and build their tables through gorm, taking the gorm handle from
// the context rather than the sql.DB goose passes them. The version table name
// is a parameter because the generator gives the deployed migrator the same
// one.
//
// The core API deploys on CockroachDB, and its initial migration sets
// ttl_expire_after on the events table. sqlite has no syntax for a table
// storage parameter, and CockroachDB backs that one with a hidden
// crdb_internal_expiration column no model declares, so the core check runs
// against a live server and reads its columns by hand:
// https://docs.cockroachlabs.com/docs/stable/row-level-ttl
//
// Hidden columns are a class rather than that one column, since a table with no
// declared primary key gets a hidden one too. gorm's own column reader queries
// information_schema.columns with no filter on is_hidden and its ColumnType
// carries no way to ask, so every hidden column comes back through it and the
// comparison reports it as a column with no field. visibleColumns filters on
// is_hidden instead, which
// CockroachDB has and PostgreSQL does not. goose names no dialect of its own for
// CockroachDB, so the migrations run under the postgres one.

// AssertMigrationsCoverModels applies the registered migrations to an in-memory
// sqlite database and reports drift between the schema they build and the
// models. Migrations carrying CockroachDB-only statements need the On variant.
func AssertMigrationsCoverModels(t *testing.T, versionTableName string, models []interface{}) {
	t.Helper()

	gormDb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	applyMigrations(t, gormDb, "sqlite3", versionTableName)
	assertCoverage(t, gormDb, models, gormColumns)
}

// AssertMigrationsCoverModelsOn applies the registered migrations to an empty
// CockroachDB database the caller opened and reports drift between the schema
// they build and the models.
func AssertMigrationsCoverModelsOn(
	t *testing.T,
	gormDb *gorm.DB,
	versionTableName string,
	models []interface{},
) {
	t.Helper()

	applyMigrations(t, gormDb, "postgres", versionTableName)
	assertCoverage(t, gormDb, models, visibleColumns)
}

// applyMigrations runs every registered migration up against gormDb. The
// dialect names the SQL goose uses for its own version table, and a CockroachDB
// target takes postgres.
func applyMigrations(t *testing.T, gormDb *gorm.DB, dialect, versionTableName string) {
	t.Helper()

	// take the pool goose writes through from the gorm handle, so the
	// migrations and the assertions read one database
	sqlDb, err := gormDb.DB()
	if err != nil {
		t.Fatalf("resolve sql db: %v", err)
	}

	// set the dialect and version table name, which goose keeps in package
	// state, so nothing else may migrate while this runs
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	goose.SetTableName(versionTableName)

	// hand the gorm handle over under the untyped key the migrations read
	ctx := context.WithValue(context.Background(), "gormdb", gormDb)

	// run the migrations goose holds in its registry; goose still scans the
	// directory, so it has to exist and hold no migration file of its own
	if err := goose.UpContext(ctx, sqlDb, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}

// assertCoverage compares the columns each model declares against the columns
// columnsOf reads back, reporting fields left without a column and columns left
// without a field.
func assertCoverage(
	t *testing.T,
	gormDb *gorm.DB,
	models []interface{},
	columnsOf func(*testing.T, *gorm.DB, interface{}, string) map[string]bool,
) {
	t.Helper()

	for _, model := range models {
		// read the column names the model declares, embedded structs included
		stmt := &gorm.Statement{DB: gormDb}
		if err := stmt.Parse(model); err != nil {
			t.Fatalf("parse %T: %v", model, err)
		}
		declared := make(map[string]bool, len(stmt.Schema.DBNames))
		for _, name := range stmt.Schema.DBNames {
			declared[name] = true
		}

		if !gormDb.Migrator().HasTable(model) {
			t.Errorf("no migration creates table %s for %T", stmt.Schema.Table, model)
			continue
		}

		// read the column names the migrations created
		created := columnsOf(t, gormDb, model, stmt.Schema.Table)

		// find fields with no column and columns with no field
		var missingColumns []string
		for name := range declared {
			if !created[name] {
				missingColumns = append(missingColumns, name)
			}
		}

		var missingFields []string
		for name := range created {
			if !declared[name] {
				missingFields = append(missingFields, name)
			}
		}

		// sort so a failing run reports the same order every time
		sort.Strings(missingColumns)
		sort.Strings(missingFields)
		if len(missingColumns) > 0 {
			t.Errorf("%s has fields with no column: %v", stmt.Schema.Table, missingColumns)
		}
		if len(missingFields) > 0 {
			t.Errorf("%s has columns with no field: %v", stmt.Schema.Table, missingFields)
		}
	}
}

// gormColumns returns the names of the columns gorm reads back for model.
func gormColumns(t *testing.T, gormDb *gorm.DB, model interface{}, _ string) map[string]bool {
	t.Helper()

	columnTypes, err := gormDb.Migrator().ColumnTypes(model)
	if err != nil {
		t.Fatalf("read columns for %T: %v", model, err)
	}
	created := make(map[string]bool, len(columnTypes))
	for _, columnType := range columnTypes {
		created[columnType.Name()] = true
	}

	return created
}

// visibleColumns returns the names of the columns of table that a client sees.
func visibleColumns(t *testing.T, gormDb *gorm.DB, _ interface{}, table string) map[string]bool {
	t.Helper()

	var names []string
	err := gormDb.Raw(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ? AND is_hidden = 'NO'
	`, table).Scan(&names).Error
	if err != nil {
		t.Fatalf("read columns for %s: %v", table, err)
	}
	created := make(map[string]bool, len(names))
	for _, name := range names {
		created[name] = true
	}

	return created
}
