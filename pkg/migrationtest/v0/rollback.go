package migrationtest

import (
	"context"
	"database/sql"
	"testing"

	goose "github.com/pressly/goose/v3"
	gorm "gorm.io/gorm"
)

// AssertMigrationsRollBackOn checks the convention a failed migration's
// rollback relies on: every registered migration has a down migration that
// reverts it, and a down migration also cleans up after its own migration
// failing part way, so it runs against a database the migration never got
// through.  Each database must be empty: migrated holds the round trip, and
// partial is where the down migrations run without their migration.
func AssertMigrationsRollBackOn(t *testing.T, migrated, partial *gorm.DB, versionTableName string) {
	t.Helper()

	migrations, err := goose.CollectMigrations(".", 0, goose.MaxVersion)
	if err != nil {
		t.Fatalf("collect migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations are registered")
	}
	for _, migration := range migrations {
		if migration.DownFnNoTxContext == nil && migration.DownFnContext == nil {
			t.Errorf("migration %d has no down migration, so a failed upgrade cannot be rolled back past it", migration.Version)
		}
	}
	if t.Failed() {
		return
	}

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	goose.SetTableName(versionTableName)

	// up, down to nothing, and up again
	migratedDb := sqlDb(t, migrated)
	migratedCtx := context.WithValue(context.Background(), "gormdb", migrated)
	if err := goose.UpContext(migratedCtx, migratedDb, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := goose.DownToContext(migratedCtx, migratedDb, ".", 0); err != nil {
		t.Fatalf("roll every migration back: %v", err)
	}
	if version, err := goose.GetDBVersionContext(migratedCtx, migratedDb); err != nil || version != 0 {
		t.Fatalf("version after rolling every migration back = %d (%v), want 0", version, err)
	}
	if err := goose.UpContext(migratedCtx, migratedDb, "."); err != nil {
		t.Fatalf("apply migrations again after rolling them back: %v", err)
	}

	// each down migration on a database its migration never changed, newest
	// first, as a rollback runs them
	partialDb := sqlDb(t, partial)
	partialCtx := context.WithValue(context.Background(), "gormdb", partial)
	for i := len(migrations) - 1; i >= 0; i-- {
		migration := migrations[i]
		if migration.DownFnNoTxContext == nil {
			continue
		}
		if err := migration.DownFnNoTxContext(partialCtx, partialDb); err != nil {
			t.Errorf("migration %d's down migration fails on a database its migration did not finish: %v", migration.Version, err)
		}
	}
}

// sqlDb returns the database a gorm handle wraps.
func sqlDb(t *testing.T, gormDb *gorm.DB) *sql.DB {
	t.Helper()

	db, err := gormDb.DB()
	if err != nil {
		t.Fatalf("resolve sql db: %v", err)
	}

	return db
}
