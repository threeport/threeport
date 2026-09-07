// Package migrationtest checks that a goose/gorm migration chain created
// every column the models declare.
package migrationtest

import (
	"context"
	"sort"
	"testing"

	goose "github.com/pressly/goose/v3"
	sqlite "gorm.io/driver/sqlite"
	gorm "gorm.io/gorm"
)

// AssertMigrationsCoverModels applies migrations to in-memory sqlite.
// Cockroach-only DDL (row-level TTL) needs AssertMigrationsCoverModelsOn.
func AssertMigrationsCoverModels(t *testing.T, versionTableName string, models []interface{}) {
	t.Helper()

	gormDb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	applyMigrations(t, gormDb, "sqlite3", versionTableName)
	assertCoverage(t, gormDb, models, gormColumns)
}

// AssertMigrationsCoverModelsOn applies migrations to the caller's CockroachDB
// and ignores hidden columns such as crdb_internal_expiration.
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

// applyMigrations runs every registered goose migration against gormDb.
// dialect is goose's version-table dialect (postgres for CockroachDB).
func applyMigrations(t *testing.T, gormDb *gorm.DB, dialect, versionTableName string) {
	t.Helper()

	sqlDb, err := gormDb.DB()
	if err != nil {
		t.Fatalf("resolve sql db: %v", err)
	}

	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	goose.SetTableName(versionTableName)

	// migrations read the gorm handle off the context, not the sql.DB goose passes
	ctx := context.WithValue(context.Background(), "gormdb", gormDb)
	if err := goose.UpContext(ctx, sqlDb, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}

// assertCoverage reports fields with no column and columns with no field.
func assertCoverage(
	t *testing.T,
	gormDb *gorm.DB,
	models []interface{},
	columnsOf func(*testing.T, *gorm.DB, interface{}, string) map[string]bool,
) {
	t.Helper()

	for _, model := range models {
		// columns the model declares
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

		// columns the migrations created
		created := columnsOf(t, gormDb, model, stmt.Schema.Table)

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

// gormColumns returns the column names gorm reads back for model.
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

// visibleColumns lists columns a client sees, excluding Cockroach hidden ones.
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
