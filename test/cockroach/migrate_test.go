package cockroach

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	goose "github.com/pressly/goose/v3"

	migrator "github.com/threeport/threeport/pkg/database-migrator/v0"
)

// migrateTestDatabase is the database the migration tests migrate, apart from
// the one the other tests share.
const migrateTestDatabase = "migrate_check"

// testMigration returns a migration that runs outside a transaction, as
// threeport's migrations do since CockroachDB cannot change a schema inside
// one.
func testMigration(version int64, up, down func(context.Context, *sql.DB) error) *goose.Migration {
	return &goose.Migration{
		Type:              goose.TypeGo,
		Version:           version,
		Source:            "test_migration.go",
		Registered:        true,
		UpFnNoTxContext:   up,
		DownFnNoTxContext: down,
	}
}

// execAll returns a migration function that runs statements in order.
func execAll(statements ...string) func(context.Context, *sql.DB) error {
	return func(ctx context.Context, db *sql.DB) error {
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}
}

// failAfter returns a migration function that runs statements and then
// fails, leaving what they changed behind, as a migration that breaks part
// way does.
func failAfter(statements ...string) func(context.Context, *sql.DB) error {
	run := execAll(statements...)
	return func(ctx context.Context, db *sql.DB) error {
		if err := run(ctx, db); err != nil {
			return err
		}
		return errors.New("deliberately broken migration")
	}
}

// tableExists reports whether a table exists in the migrate test database.
// information_schema still lists a table CockroachDB is dropping, so the
// name is resolved the way a query would resolve it.
func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var exists bool
	if err := db.QueryRow("SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
		t.Fatalf("check for table %s: %v", table, err)
	}

	return exists
}

// TestABrokenMigrationIsBackedUpAndRolledBack is the issue's acceptance test,
// against a real database: an upgrade whose last migration fails part way
// takes a backup first, cleans up the broken migration, rolls back the one
// applied before it, records the failure and reports it, leaving the database
// at the version it started from.  Later runs refuse to migrate until the
// failure is cleared, and a fixed migration then applies.
func TestABrokenMigrationIsBackedUpAndRolledBack(t *testing.T) {
	ctx := context.Background()
	gormDb := freshDatabase(t, migrateTestDatabase)
	db, err := gormDb.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	rootDb, err := root.DB()
	if err != nil {
		t.Fatalf("get root sql db: %v", err)
	}

	previousTable := goose.TableName()
	goose.SetTableName("migrate_check_goose_db_version")
	t.Cleanup(func() { goose.SetTableName(previousTable) })
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}

	first := testMigration(1,
		execAll("CREATE TABLE objects (id INT8 PRIMARY KEY)"),
		execAll("DROP TABLE IF EXISTS objects"),
	)
	second := testMigration(2,
		execAll("ALTER TABLE objects ADD COLUMN name STRING"),
		execAll("ALTER TABLE objects DROP COLUMN IF EXISTS name"),
	)
	broken := testMigration(3,
		failAfter("CREATE TABLE events (id INT8 PRIMARY KEY)"),
		execAll("DROP TABLE IF EXISTS events"),
	)

	record, err := migrator.NewTableFailureRecord(ctx, db, "migrate_check_goose_db_version_failure")
	if err != nil {
		t.Fatalf("create failure record: %v", err)
	}
	backup := migrator.CockroachBackup(rootDb, migrateTestDatabase, "nodelocal://1/migrate_check")

	// the release the database is installed at
	if err := migrator.Up(ctx, migrator.NewGooseSteps(db, goose.Migrations{first}), record, migrator.Options{}); err != nil {
		t.Fatalf("install at version 1: %v", err)
	}
	if _, err := db.Exec("INSERT INTO objects (id) VALUES (1)"); err != nil {
		t.Fatalf("write a row: %v", err)
	}

	// an upgrade whose last migration breaks part way
	upgrade := migrator.NewGooseSteps(db, goose.Migrations{first, second, broken})
	err = migrator.Up(ctx, upgrade, record, migrator.Options{Backup: backup})
	if !errors.Is(err, migrator.ErrMigrationFailed) {
		t.Fatalf("the broken migration was not reported: %v", err)
	}
	t.Logf("reported: %v", err)

	// the database is back at version 1, data included
	version, err := upgrade.CurrentVersion(ctx)
	if err != nil || version != 1 {
		t.Fatalf("version after the rollback = %d (%v), want 1", version, err)
	}
	if tableExists(t, db, "events") {
		t.Error("the broken migration's table was left behind")
	}
	var columns int
	db.QueryRow("SELECT count(*) FROM [SHOW COLUMNS FROM objects] WHERE column_name = 'name'").Scan(&columns)
	if columns != 0 {
		t.Error("the migration applied before the broken one was not rolled back")
	}
	var rows int
	db.QueryRow("SELECT count(*) FROM objects").Scan(&rows)
	if rows != 1 {
		t.Errorf("rows after the rollback = %d, want the 1 written before the upgrade", rows)
	}

	// the backup taken before migrating exists and is recorded
	failure, err := record.LastFailure(ctx)
	if err != nil || failure == nil {
		t.Fatalf("no failure recorded: %v", err)
	}
	if failure.Version != 3 || failure.FromVersion != 1 || !failure.RolledBack || failure.Backup == "" {
		t.Errorf("recorded failure = %+v", failure)
	}
	var backups int
	rootDb.QueryRow("SELECT count(*) FROM [SHOW BACKUPS IN 'nodelocal://1/migrate_check'] WHERE path = $1", failure.Backup).Scan(&backups)
	if backups != 1 {
		t.Errorf("the recorded backup %q is not in the collection", failure.Backup)
	}

	// a restart refuses to migrate again
	if err := migrator.Up(ctx, upgrade, record, migrator.Options{Backup: backup}); !errors.Is(err, migrator.ErrMigrationFailed) {
		t.Fatalf("a restart migrated past a recorded failure: %v", err)
	}
	if version, _ := upgrade.CurrentVersion(ctx); version != 1 {
		t.Fatalf("a restart changed the version to %d", version)
	}

	// once the failure is cleared, the fixed migration applies
	if _, err := db.Exec("DELETE FROM migrate_check_goose_db_version_failure"); err != nil {
		t.Fatalf("clear the failure: %v", err)
	}
	fixed := testMigration(3,
		execAll("CREATE TABLE events (id INT8 PRIMARY KEY)"),
		execAll("DROP TABLE IF EXISTS events"),
	)
	if err := migrator.Up(ctx, migrator.NewGooseSteps(db, goose.Migrations{first, second, fixed}), record, migrator.Options{Backup: backup}); err != nil {
		t.Fatalf("the fixed upgrade failed: %v", err)
	}
	if version, _ := upgrade.CurrentVersion(ctx); version != 3 {
		t.Errorf("version after the fixed upgrade = %d, want 3", version)
	}
}
