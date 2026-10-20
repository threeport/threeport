package migrationtest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	goose "github.com/pressly/goose/v3"
	gorm "gorm.io/gorm"
)

// AssertMigrationsRollBackOn checks the convention a failed migration's
// rollback relies on, for every registered migration: each has a down
// migration that reverts it, and that also cleans up after its own migration
// failing part way.  newDatabase returns a new empty database for each case.
func AssertMigrationsRollBackOn(t *testing.T, newDatabase func(name string) *gorm.DB, versionTableName string) {
	t.Helper()

	migrations, err := goose.CollectMigrations(".", 0, goose.MaxVersion)
	if err != nil {
		t.Fatalf("collect migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations are registered")
	}

	AssertMigrationChainRollsBackOn(t, migrations, newDatabase, versionTableName)
}

// AssertMigrationChainRollsBackOn checks the rollback convention for a chain
// of migrations, oldest first.  The whole chain goes up, down to nothing and
// up again on one database.  Then each down migration runs on a database built
// through the migrations before it, as it does after its migration failed
// before changing anything.
func AssertMigrationChainRollsBackOn(
	t *testing.T,
	migrations goose.Migrations,
	newDatabase func(name string) *gorm.DB,
	versionTableName string,
) {
	t.Helper()

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
	migrated := newDatabase("rollback_round_trip")
	migratedDb := sqlDb(t, migrated)
	migratedCtx := context.WithValue(context.Background(), "gormdb", migrated)
	applyEach(t, migratedCtx, migratedDb, migrations)
	for i := len(migrations) - 1; i >= 0; i-- {
		if err := migrations[i].DownContext(migratedCtx, migratedDb); err != nil {
			t.Fatalf("roll migration %d back: %v", migrations[i].Version, err)
		}
	}
	if version, err := goose.GetDBVersionContext(migratedCtx, migratedDb); err != nil || version != 0 {
		t.Fatalf("version after rolling every migration back = %d (%v), want 0", version, err)
	}
	applyEach(t, migratedCtx, migratedDb, migrations)

	// each down migration on the database its migration would have started
	// from, which the migration never changed
	for i, migration := range migrations {
		if migration.DownFnNoTxContext == nil {
			continue
		}
		partial := newDatabase(fmt.Sprintf("rollback_partial_%d", migration.Version))
		partialDb := sqlDb(t, partial)
		partialCtx := context.WithValue(context.Background(), "gormdb", partial)
		applyEach(t, partialCtx, partialDb, migrations[:i])
		if err := migration.DownFnNoTxContext(partialCtx, partialDb); err != nil {
			t.Errorf("migration %d's down migration fails on a database its migration did not change: %v", migration.Version, err)
		}
	}
}

// applyEach applies migrations in order, creating goose's version table
// first if it is not there yet.
func applyEach(t *testing.T, ctx context.Context, db *sql.DB, migrations goose.Migrations) {
	t.Helper()

	if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
		t.Fatalf("create the goose version table: %v", err)
	}
	for _, migration := range migrations {
		if err := migration.UpContext(ctx, db); err != nil {
			t.Fatalf("apply migration %d: %v", migration.Version, err)
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
