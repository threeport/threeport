package cockroach

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The restore tests use a database and owner of their own, so replacing the
// database leaves the one the other tests share alone.
const (
	restoreTestDatabase = "restore_check"
	restoreTestOwner    = "restore_owner"
	restoreTestURI      = "nodelocal://1/restore_check"
)

// runInOneSession runs statements one at a time in a single session, as the
// restore job's database client does.
func runInOneSession(t *testing.T, db *gorm.DB, statements []string) error {
	t.Helper()

	sqlDb, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	conn, err := sqlDb.Conn(context.Background())
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	defer conn.Close()

	for _, statement := range statements {
		if _, err := conn.ExecContext(context.Background(), statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
	}

	return nil
}

// openAsOwner opens the restore test database as its owner, the way the API
// server opens its own.
func openAsOwner(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := openAs(testPort, restoreTestOwner, restoreTestDatabase)
	if err != nil {
		t.Fatalf("open %s as %s: %v", restoreTestDatabase, restoreTestOwner, err)
	}

	return db
}

// mustExec runs a statement and fails the test on an error.
func mustExec(t *testing.T, db *gorm.DB, statement string) {
	t.Helper()

	if err := db.Exec(statement).Error; err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// TestRestoreReplacesTheDatabaseAndKeepsItUsable covers a restore end to
// end against a real database: it brings back the data the backup holds,
// the user the API server connects as can still read, write and migrate
// it, and the scheduled incremental backups keep working on top of it.
func TestRestoreReplacesTheDatabaseAndKeepsItUsable(t *testing.T) {
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}

	// the database as the install leaves it, with the tables its owner creates
	mustExec(t, root, "CREATE USER IF NOT EXISTS "+restoreTestOwner)
	mustExec(t, root, "CREATE DATABASE "+restoreTestDatabase)
	mustExec(t, root, fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", restoreTestDatabase, restoreTestOwner))
	owner := openAsOwner(t)
	mustExec(t, owner, "CREATE TABLE objects (id INT PRIMARY KEY, name STRING)")
	mustExec(t, owner, "INSERT INTO objects VALUES (1, 'backed-up')")

	// back it up, then change it
	mustExec(t, root, fmt.Sprintf("BACKUP DATABASE %s INTO '%s'", restoreTestDatabase, restoreTestURI))
	mustExec(t, owner, "UPDATE objects SET name = 'changed' WHERE id = 1")
	mustExec(t, owner, "INSERT INTO objects VALUES (2, 'written-after')")

	// restore the latest backup
	statements, err := installer.DatabaseRestoreStatements(restoreTestDatabase, restoreTestOwner, restoreTestURI, installer.LatestBackup)
	if err != nil {
		t.Fatalf("restore statements: %v", err)
	}
	if err := runInOneSession(t, root, statements); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// the data is the backup's
	owner = openAsOwner(t)
	var names []string
	if err := owner.Raw("SELECT name FROM objects ORDER BY id").Scan(&names).Error; err != nil {
		t.Fatalf("the owner cannot read the restored data: %v", err)
	}
	if len(names) != 1 || names[0] != "backed-up" {
		t.Fatalf("restored rows = %v, want the backed up row only", names)
	}

	// the owner writes to it and migrates it as before
	mustExec(t, owner, "INSERT INTO objects VALUES (3, 'written-after-restore')")
	mustExec(t, owner, "ALTER TABLE objects ADD COLUMN reconciled BOOL")
	mustExec(t, owner, "CREATE TABLE added_by_a_migration (id INT PRIMARY KEY)")

	// the database and its schema are owned as a new database is, not by the API's user
	var databaseOwner, schemaOwner string
	root.Raw("SELECT owner FROM [SHOW DATABASES] WHERE database_name = ?", restoreTestDatabase).Scan(&databaseOwner)
	root.Raw(fmt.Sprintf("SELECT owner FROM [SHOW SCHEMAS FROM %s] WHERE schema_name = 'public'", restoreTestDatabase)).Scan(&schemaOwner)
	if databaseOwner != "root" || schemaOwner != "root" {
		t.Errorf("database owner %q and schema owner %q, want root for both", databaseOwner, schemaOwner)
	}

	// the next incremental backup into the same location, as the schedule
	// takes it, works on top of the restore, and restores
	mustExec(t, root, fmt.Sprintf("BACKUP DATABASE %s INTO LATEST IN '%s'", restoreTestDatabase, restoreTestURI))
	if err := runInOneSession(t, root, statements); err != nil {
		t.Fatalf("restore from the backup taken after a restore: %v", err)
	}
	var count int
	openAsOwner(t).Raw("SELECT count(*) FROM objects").Scan(&count)
	if count != 2 {
		t.Errorf("rows after restoring the post-restore backup = %d, want 2", count)
	}
}

// TestAFailedRestoreLeavesTheDatabaseInPlace covers a restore that cannot
// complete, here from a backup that does not exist: the live database is
// dropped only after the backup is restored, so it survives.
func TestAFailedRestoreLeavesTheDatabaseInPlace(t *testing.T) {
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	database := "restore_failure_check"
	mustExec(t, root, "CREATE DATABASE "+database)
	mustExec(t, root, fmt.Sprintf("CREATE TABLE %s.objects (id INT PRIMARY KEY)", database))
	mustExec(t, root, fmt.Sprintf("INSERT INTO %s.objects VALUES (1)", database))

	statements, err := installer.DatabaseRestoreStatements(database, restoreTestOwner, "nodelocal://1/no-backups-here", "/2020/01/01-000000.00")
	if err != nil {
		t.Fatalf("restore statements: %v", err)
	}
	if err := runInOneSession(t, root, statements); err == nil {
		t.Fatal("the restore of a backup that does not exist succeeded")
	}

	var count int
	if err := root.Raw(fmt.Sprintf("SELECT count(*) FROM %s.objects", database)).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("the database did not survive the failed restore: count %d, err %v", count, err)
	}
}
