package cockroach

import (
	"context"
	"database/sql"
	"testing"
	"time"

	migrator "github.com/threeport/threeport/pkg/database-migrator/v0"
)

// lockTestTable is the lock table the lock tests use.
const lockTestTable = "lock_check_goose_db_version_lock"

// newTestLock returns a lock for holder in the lock test database, giving up
// after wait.
func newTestLock(t *testing.T, db *sql.DB, holder string, lease, wait time.Duration) *migrator.TableLock {
	t.Helper()

	lock, err := migrator.NewTableLockWithTiming(context.Background(), db, lockTestTable, holder, lease, wait, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("create lock for %s: %v", holder, err)
	}

	return lock
}

// TestOneMigratorAtATime covers two migrators of one database against a real
// CockroachDB: while one holds the lock the other waits, and gets it once it
// is released.
func TestOneMigratorAtATime(t *testing.T) {
	ctx := context.Background()
	gormDb := freshDatabase(t, "lock_check")
	db, err := gormDb.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}

	first := newTestLock(t, db, "api-server-a", time.Minute, time.Second)
	second := newTestLock(t, db, "api-server-b", time.Minute, 200*time.Millisecond)

	if err := first.Acquire(ctx); err != nil {
		t.Fatalf("the first migrator did not get the free lock: %v", err)
	}
	if err := second.Acquire(ctx); err == nil {
		t.Fatal("the second migrator got the lock while the first held it")
	}

	// the second gets it once the first releases it, while still waiting
	waiting := newTestLock(t, db, "api-server-b", time.Minute, 5*time.Second)
	acquired := make(chan error, 1)
	go func() { acquired <- waiting.Acquire(ctx) }()
	time.Sleep(100 * time.Millisecond)
	if err := first.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := <-acquired; err != nil {
		t.Fatalf("the waiting migrator did not get the released lock: %v", err)
	}
	if err := waiting.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
}

// TestALockLeftBehindExpires covers a migrator that stopped without
// releasing the lock, such as one whose pod was killed: its lease expires and
// another migrator gets the lock.  A held lock is renewed, so it does not
// expire under its holder.
func TestALockLeftBehindExpires(t *testing.T) {
	ctx := context.Background()
	gormDb := freshDatabase(t, "lock_expiry_check")
	db, err := gormDb.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}

	// a lock held past its lease, by renewal
	holding := newTestLock(t, db, "api-server-a", 300*time.Millisecond, time.Second)
	if err := holding.Acquire(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	time.Sleep(time.Second)
	if err := newTestLock(t, db, "api-server-b", time.Minute, 50*time.Millisecond).Acquire(ctx); err == nil {
		t.Fatal("a renewed lock expired under its holder")
	}
	if err := holding.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}

	// a lock left behind by a holder that stopped
	if _, err := db.Exec("INSERT INTO " + lockTestTable + " (id, holder, expires_at) VALUES (1, 'killed-pod', now() + INTERVAL '200 milliseconds')"); err != nil {
		t.Fatalf("leave a lock behind: %v", err)
	}
	if err := newTestLock(t, db, "api-server-b", time.Minute, 2*time.Second).Acquire(ctx); err != nil {
		t.Fatalf("the lock left behind did not expire: %v", err)
	}
}

// TestAHolderTakesBackItsOwnLock covers an init container restarted in the
// same pod: it takes back the lock it held without waiting for it to expire.
func TestAHolderTakesBackItsOwnLock(t *testing.T) {
	gormDb := freshDatabase(t, "lock_holder_check")
	db, err := gormDb.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}

	if err := newTestLock(t, db, "api-server-a", time.Hour, time.Second).Acquire(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := newTestLock(t, db, "api-server-a", time.Hour, 50*time.Millisecond).Acquire(context.Background()); err != nil {
		t.Fatalf("the same holder could not take back its lock: %v", err)
	}
}

// TestMigratorsSetUpTogether covers what two migrators do before either
// holds the lock: both create the lock and failure record tables, at the same
// time, which must not fail either of them.
func TestMigratorsSetUpTogether(t *testing.T) {
	gormDb := freshDatabase(t, "lock_setup_check")
	db, err := gormDb.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}

	const migrators = 8
	errs := make(chan error, migrators)
	for i := 0; i < migrators; i++ {
		go func(i int) {
			ctx := context.Background()
			if _, err := migrator.NewTableLock(ctx, db, "setup_goose_db_version_lock", "api-server"); err != nil {
				errs <- err
				return
			}
			_, err := migrator.NewTableFailureRecord(ctx, db, "setup_goose_db_version_failure")
			errs <- err
		}(i)
	}
	for i := 0; i < migrators; i++ {
		if err := <-errs; err != nil {
			t.Errorf("a migrator failed setting up: %v", err)
		}
	}
}
