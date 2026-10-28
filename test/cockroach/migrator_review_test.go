package cockroach

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	goose "github.com/pressly/goose/v3"

	migrator "github.com/threeport/threeport/pkg/database-migrator/v0"
)

// sqlDbOf returns a fresh database's sql handle.
func sqlDbOf(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := freshDatabase(t, name).DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	return db
}

// TestAMigrationMergedOutOfOrderStopsTheUpgrade covers a migration whose
// version sorts below one the database already applied, as one merged out of
// order with migrations applied elsewhere first is.  It was never applied, and
// never would be: an upgrade refuses to go on without it rather than skip it
// for good, and changes nothing.
func TestAMigrationMergedOutOfOrderStopsTheUpgrade(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "missing_check")

	previousTable := goose.TableName()
	goose.SetTableName("missing_check_goose_db_version")
	t.Cleanup(func() { goose.SetTableName(previousTable) })
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	record, err := migrator.NewTableFailureRecord(ctx, db, "missing_check_goose_db_version_failure")
	if err != nil {
		t.Fatalf("create failure record: %v", err)
	}

	first := testMigration(1, execAll("CREATE TABLE a (id INT8 PRIMARY KEY)"), execAll("DROP TABLE IF EXISTS a"))
	third := testMigration(3, execAll("CREATE TABLE c (id INT8 PRIMARY KEY)"), execAll("DROP TABLE IF EXISTS c"))
	if err := migrator.Up(ctx, migrator.NewGooseSteps(db, goose.Migrations{first, third}), record, migrator.Options{}); err != nil {
		t.Fatalf("migrate to version 3: %v", err)
	}

	second := testMigration(2, execAll("CREATE TABLE b (id INT8 PRIMARY KEY)"), execAll("DROP TABLE IF EXISTS b"))
	fourth := testMigration(4, execAll("CREATE TABLE d (id INT8 PRIMARY KEY)"), execAll("DROP TABLE IF EXISTS d"))
	steps := migrator.NewGooseSteps(db, goose.Migrations{first, second, third, fourth})
	err = migrator.Up(ctx, steps, record, migrator.Options{})
	if err == nil {
		t.Fatal("the upgrade skipped migration 2 without a word")
	}
	if !strings.Contains(err.Error(), "found 1 missing migrations before current version 3: 2") {
		t.Errorf("error does not name the missing migration: %v", err)
	}
	t.Logf("reported: %v", err)
	if version, _ := steps.CurrentVersion(ctx); version != 3 {
		t.Errorf("version = %d, want it left at 3", version)
	}
	if tableExists(t, db, "b") || tableExists(t, db, "d") {
		t.Error("the upgrade changed the database before refusing")
	}
}

// TestTheBackupBeforeMigratingIsTheOneItTook covers the backup path the
// migrator records, which is what an operator restores during an incident.
// The scheduled backups go into the same collection, and can land between
// the migrator's backup and its looking for it, so the newest backup in the
// collection is not necessarily the migrator's.  Here backups land in the
// collection the whole time, and every path returned must be the backup taken
// as of the time CockroachDB itself records for it.
func TestTheBackupBeforeMigratingIsTheOneItTook(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "backup_path_check")
	if _, err := db.Exec("CREATE TABLE objects (id INT8 PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	rootDb, err := root.DB()
	if err != nil {
		t.Fatalf("get root sql db: %v", err)
	}
	const collection = "nodelocal://1/backup_path_check"

	// the schedule, backing up into the same collection throughout
	stop := make(chan struct{})
	var scheduled sync.WaitGroup
	var scheduledRuns atomic.Int64
	scheduled.Add(1)
	go func() {
		defer scheduled.Done()
		for {
			select {
			case <-stop:
				return
			default:
				scheduledRuns.Add(1)
				rootDb.Exec("BACKUP DATABASE backup_path_check INTO $1", collection)
			}
		}
	}()

	backup := migrator.CockroachBackup(rootDb, "backup_path_check", collection)
	var paths []string
	for i := 0; i < 3; i++ {
		path, err := backup(ctx)
		if err != nil {
			close(stop)
			scheduled.Wait()
			t.Fatalf("backup %d: %v", i, err)
		}
		paths = append(paths, path)
	}
	close(stop)
	scheduled.Wait()

	var total int
	rootDb.QueryRow("SELECT count(*) FROM [SHOW BACKUPS IN $1]", collection).Scan(&total)
	t.Logf("%d backups in the collection after %d scheduled runs, the migrator's %v", total, scheduledRuns.Load(), paths)
	if scheduledRuns.Load() < 2 {
		t.Errorf("only %d scheduled backups ran beside the migrator's, so nothing raced it", scheduledRuns.Load())
	}
	for _, path := range paths {
		var endTime time.Time
		if err := rootDb.QueryRow(
			"SELECT DISTINCT end_time FROM [SHOW BACKUP FROM $1 IN $2]", path, collection,
		).Scan(&endTime); err != nil {
			t.Fatalf("read backup %s: %v", path, err)
		}
		if named := endTime.UTC().Format("/2006/01/02-150405.00"); named != path {
			t.Errorf("returned %s, but that backup is as of %s", path, named)
		}
	}
}

// renewals counts the lock renewals running in this process.
func renewals() int {
	buf := make([]byte, 1<<20)
	stacks := string(buf[:runtime.Stack(buf, true)])
	return strings.Count(stacks, "(*TableLock).startRenewing.func1(")
}

// TestAcquiringTwiceKeepsOneRenewal covers Acquire called again on a lock it
// holds: a second renewal would outlive the Release that stops the first,
// renewing a lock nothing holds any more.
func TestAcquiringTwiceKeepsOneRenewal(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "renewal_check")
	before := renewals()

	lock := newTestLock(t, db, "api-server-a", time.Minute, time.Second)
	for i := 0; i < 2; i++ {
		if err := lock.Acquire(ctx); err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
	}
	if running := renewals() - before; running != 1 {
		t.Errorf("%d renewals running for one held lock", running)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for renewals() != before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if running := renewals() - before; running != 0 {
		t.Errorf("%d renewals left running after Release", running)
	}
}

// TestAHangingRenewalDoesNotHangRelease covers a renewal stuck in the
// database - here behind a transaction holding the lock's row - when the
// migrator is done.  Release stops it rather than waiting for the database,
// so a migration run returns even while the database does not answer.
func TestAHangingRenewalDoesNotHangRelease(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "hanging_renewal_check")

	lock, err := migrator.NewTableLockWithTiming(ctx, db, lockTestTable, "api-server-a", 3*time.Second, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("create lock: %v", err)
	}
	if err := lock.Acquire(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// hold the lock's row so the next renewal waits on it
	blocker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer blocker.Rollback()
	if _, err := blocker.Exec("SELECT * FROM " + lockTestTable + " WHERE id = 1 FOR UPDATE"); err != nil {
		t.Fatalf("hold the lock row: %v", err)
	}
	time.Sleep(1500 * time.Millisecond) // past the first renewal, now waiting

	released := make(chan error, 1)
	start := time.Now()
	go func() {
		releaseCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		released <- lock.Release(releaseCtx)
	}()
	select {
	case err := <-released:
		t.Logf("Release returned after %s: %v", time.Since(start), err)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Errorf("unexpected release error: %v", err)
		}
	case <-time.After(2 * time.Second):
		blocker.Rollback()
		<-released
		t.Fatal("Release waited on the hanging renewal")
	}
}

// racingConn runs a scheduled backup into the same collection right after the
// migrator's own BACKUP returns, before it can look for what it took - the
// window the schedule can land in.
//
// first, when set, runs instead just before the migrator's first BACKUP, with
// the statement it is about to run.
//
// seen, when set, is told every statement the migrator runs.
type racingConn struct {
	driver.Conn
	race  func(ctx context.Context) error
	first func(ctx context.Context, query string) error
	seen  func(query string)
	ran   bool
}

func (c *racingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	isBackup := strings.HasPrefix(query, "BACKUP DATABASE")
	if c.seen != nil {
		c.seen(query)
	}
	if isBackup && !c.ran && c.first != nil {
		c.ran = true
		if err := c.first(ctx, query); err != nil {
			return nil, err
		}
	}
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	if err == nil && !c.ran && isBackup && c.race != nil {
		c.ran = true
		if raceErr := c.race(ctx); raceErr != nil {
			return nil, raceErr
		}
	}
	return result, err
}

func (c *racingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (c *racingConn) CheckNamedValue(value *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(value)
}

type racingConnector struct {
	dsn   string
	race  func(ctx context.Context) error
	first func(ctx context.Context, query string) error
	seen  func(query string)
}

func (c racingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := stdlib.GetDefaultDriver().Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &racingConn{Conn: conn, race: c.race, first: c.first, seen: c.seen}, nil
}

func (c racingConnector) Driver() driver.Driver { return stdlib.GetDefaultDriver() }

// TestTheBackupRecordedIsNotTheScheduledOneThatLandedAfterIt is the review's
// case, made to happen every time: the scheduled backup lands in the same
// collection after the migrator's backup and before the migrator looks for
// it.  The scheduled one is the newest there, and the path recorded must
// still be the migrator's - the one an operator would restore in an incident.
func TestTheBackupRecordedIsNotTheScheduledOneThatLandedAfterIt(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "backup_race_check")
	if _, err := db.Exec("CREATE TABLE objects (id INT8 PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	scheduleDb, err := root.DB()
	if err != nil {
		t.Fatalf("get root sql db: %v", err)
	}
	const collection = "nodelocal://1/backup_race_check"

	var before []string
	var scheduledPath string
	migratorDb := sql.OpenDB(racingConnector{
		dsn: fmt.Sprintf("postgres://root@127.0.0.1:%s/defaultdb?sslmode=disable", testPort),
		race: func(ctx context.Context) error {
			// the paths before the scheduled backup: the migrator's is among them
			rows, err := scheduleDb.QueryContext(ctx, "SELECT path FROM [SHOW BACKUPS IN $1]", collection)
			if err != nil {
				return err
			}
			for rows.Next() {
				var path string
				rows.Scan(&path)
				before = append(before, path)
			}
			rows.Close()
			time.Sleep(20 * time.Millisecond) // into the next hundredth, as a real schedule would be
			if _, err := scheduleDb.ExecContext(ctx, "BACKUP DATABASE backup_race_check INTO $1", collection); err != nil {
				return fmt.Errorf("scheduled backup: %w", err)
			}
			return scheduleDb.QueryRowContext(ctx, "SELECT path FROM [SHOW BACKUPS IN $1] ORDER BY path DESC LIMIT 1", collection).Scan(&scheduledPath)
		},
	})
	migratorDb.SetMaxOpenConns(1)
	t.Cleanup(func() { migratorDb.Close() })

	path, err := migrator.CockroachBackup(migratorDb, "backup_race_check", collection)(ctx)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	t.Logf("migrator's backup %s, scheduled backup %s, before the schedule %v", path, scheduledPath, before)
	if scheduledPath == "" || len(before) != 1 {
		t.Fatalf("the scheduled backup did not land after the migrator's (before %v, scheduled %q)", before, scheduledPath)
	}
	if path == scheduledPath {
		t.Errorf("recorded the scheduled backup %s, which landed after the migrator's", path)
	}
	if path != before[0] {
		t.Errorf("recorded %s, but the migrator's backup is %s", path, before[0])
	}
}

// TestABackupWhosePathWasTakenIsTakenAgain covers a scheduled backup started
// in the same hundredth of a second as the migrator's, and finished first: it
// has the migrator's path.  CockroachDB refuses a second full backup there, so
// the migrator's is taken again as of a later time, rather than the run
// failing, or the scheduled one being recorded.
func TestABackupWhosePathWasTakenIsTakenAgain(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "backup_taken_check")
	if _, err := db.Exec("CREATE TABLE objects (id INT8 PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	scheduleDb, err := root.DB()
	if err != nil {
		t.Fatalf("get root sql db: %v", err)
	}
	const collection = "nodelocal://1/backup_taken_check"

	var takenPath string
	migratorDb := sql.OpenDB(racingConnector{
		dsn: fmt.Sprintf("postgres://root@127.0.0.1:%s/defaultdb?sslmode=disable", testPort),
		first: func(ctx context.Context, query string) error {
			// the scheduled backup, as of the same time, done first
			asOf := query[strings.Index(query, "AS OF SYSTEM TIME"):]
			if _, err := scheduleDb.ExecContext(ctx, "BACKUP DATABASE backup_taken_check INTO $1 "+asOf, collection); err != nil {
				return fmt.Errorf("scheduled backup: %w", err)
			}
			return scheduleDb.QueryRowContext(ctx, "SELECT path FROM [SHOW BACKUPS IN $1]", collection).Scan(&takenPath)
		},
	})
	migratorDb.SetMaxOpenConns(1)
	t.Cleanup(func() { migratorDb.Close() })

	path, err := migrator.CockroachBackup(migratorDb, "backup_taken_check", collection)(ctx)
	if err != nil {
		t.Fatalf("the backup failed rather than being taken again: %v", err)
	}
	t.Logf("scheduled backup took %s, migrator's backup %s", takenPath, path)
	if takenPath == "" || path == takenPath {
		t.Errorf("recorded %s, the path the scheduled backup took", path)
	}
	var backups int
	scheduleDb.QueryRow("SELECT count(*) FROM [SHOW BACKUPS IN $1] WHERE path = $2", collection, path).Scan(&backups)
	if backups != 1 {
		t.Errorf("the recorded backup %s is not in the collection", path)
	}
}

// TestALockTakenBackIsRenewedAgain covers a lock whose renewal stopped on its
// own, finding another migrator holding it, and that this migrator then
// acquires again.  It is renewed again, rather than left to expire under the
// migrator holding it.
func TestALockTakenBackIsRenewedAgain(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "renew_again_check")

	lock := newTestLock(t, db, "api-server-a", 900*time.Millisecond, 5*time.Second)
	if err := lock.Acquire(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// another migrator takes the lock over, so the renewal stops on its own
	if _, err := db.Exec("UPDATE " + lockTestTable + " SET holder = 'api-server-b' WHERE id = 1"); err != nil {
		t.Fatalf("take the lock over: %v", err)
	}
	time.Sleep(700 * time.Millisecond) // past a renewal tick
	if _, err := db.Exec("DELETE FROM " + lockTestTable + " WHERE id = 1"); err != nil {
		t.Fatalf("release the other migrator's lock: %v", err)
	}

	// taken back, and held for longer than its lease
	if err := lock.Acquire(ctx); err != nil {
		t.Fatalf("acquire again: %v", err)
	}
	time.Sleep(2 * time.Second)
	if err := lock.Check(ctx); err != nil {
		t.Errorf("the lock expired under its holder: %v", err)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
}

// TestABackupMixedWithAnotherIsNotRecorded covers two full backups started in
// the same hundredth of a second and running together: CockroachDB does not
// always refuse one of them, and the path can end up holding a mix of the two.
// A path is recorded only once the backup read back from it is whole and as
// of the time the migrator took it.
func TestABackupMixedWithAnotherIsNotRecorded(t *testing.T) {
	ctx := context.Background()
	db := sqlDbOf(t, "backup_mix_check")
	if _, err := db.Exec("CREATE TABLE objects (id INT8 PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	scheduleDb, err := root.DB()
	if err != nil {
		t.Fatalf("get root sql db: %v", err)
	}

	for i := 0; i < 15; i++ {
		collection := fmt.Sprintf("nodelocal://1/backup_mix_check_%d", i)
		var lastAsOf string
		var scheduled sync.WaitGroup
		migratorDb := sql.OpenDB(racingConnector{
			dsn: fmt.Sprintf("postgres://root@127.0.0.1:%s/defaultdb?sslmode=disable", testPort),
			first: func(ctx context.Context, query string) error {
				// the scheduled backup, as of the same time, running beside it
				asOf := query[strings.Index(query, "AS OF SYSTEM TIME"):]
				scheduled.Add(1)
				go func() {
					defer scheduled.Done()
					scheduleDb.Exec("BACKUP DATABASE backup_mix_check INTO $1 "+asOf, collection)
				}()
				return nil
			},
			seen: func(query string) {
				if strings.HasPrefix(query, "BACKUP DATABASE") {
					lastAsOf = strings.TrimSpace(query[strings.Index(query, "AS OF SYSTEM TIME")+len("AS OF SYSTEM TIME"):])
				}
			},
		})
		migratorDb.SetMaxOpenConns(1)

		path, err := migrator.CockroachBackup(migratorDb, "backup_mix_check", collection)(ctx)
		scheduled.Wait()
		migratorDb.Close()
		if err != nil {
			t.Logf("iteration %d: no backup recorded: %v", i, err)
			continue
		}

		var endTimes int
		var endTime time.Time
		if err := scheduleDb.QueryRow(
			"SELECT count(DISTINCT end_time), max(end_time) FROM [SHOW BACKUP FROM $1 IN $2]", path, collection,
		).Scan(&endTimes, &endTime); err != nil {
			t.Errorf("iteration %d: recorded %s, which cannot be read back: %v", i, path, err)
			continue
		}
		wall, _ := strconv.ParseInt(strings.SplitN(lastAsOf, ".", 2)[0], 10, 64)
		if endTimes != 1 || endTime.Sub(time.Unix(0, wall)).Abs() >= time.Microsecond {
			t.Errorf("iteration %d: recorded %s, which holds %d times, the latest %s, not the migrator's %s", i, path, endTimes, endTime.UTC(), time.Unix(0, wall).UTC())
		}
	}
}
