package v0

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	goose "github.com/pressly/goose/v3"
)

// GooseSteps are migrations registered with goose.  The goose version table
// name must be set with goose.SetTableName before they are used.
type GooseSteps struct {
	db         *sql.DB
	migrations goose.Migrations
}

// NewGooseSteps returns the given migrations of a database.
func NewGooseSteps(db *sql.DB, migrations goose.Migrations) *GooseSteps {
	return &GooseSteps{db: db, migrations: migrations}
}

// RegisteredGooseSteps returns the migrations registered with goose and
// built into this binary.
func RegisteredGooseSteps(db *sql.DB) (*GooseSteps, error) {
	migrations, err := goose.CollectMigrations(".", 0, goose.MaxVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to collect migrations: %w", err)
	}

	return NewGooseSteps(db, migrations), nil
}

// CurrentVersion returns the version of the last migration applied.
func (s *GooseSteps) CurrentVersion(ctx context.Context) (int64, error) {
	return goose.EnsureDBVersionContext(ctx, s.db)
}

// PendingVersions returns the migrations newer than the database, oldest
// first.
//
// A migration older than the database's version that was never applied - one
// merged out of order with migrations applied elsewhere first - would never
// be pending again, and be skipped for good.  So it is an error, as goose's
// own `up` makes it, rather than left out.
func (s *GooseSteps) PendingVersions(ctx context.Context) ([]int64, error) {
	current, err := s.CurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	registered := make([]int64, len(s.migrations))
	for i, migration := range s.migrations {
		registered[i] = migration.Version
	}
	pending, missing := pendingAndMissing(registered, applied, current)
	if len(missing) > 0 {
		versions := make([]string, len(missing))
		for i, version := range missing {
			versions[i] = fmt.Sprint(version)
		}
		return nil, fmt.Errorf(
			"found %d missing migrations before current version %d: %s; each sorts below a migration already applied, so it would never be applied - give it a version above %d",
			len(missing), current, strings.Join(versions, ", "), current,
		)
	}

	return pending, nil
}

// pendingAndMissing sorts registered migrations into those newer than the
// database's current version, which are pending, and those older that were
// never applied, which are missing.
func pendingAndMissing(registered []int64, applied map[int64]bool, current int64) (pending, missing []int64) {
	for _, version := range registered {
		switch {
		case version > current:
			pending = append(pending, version)
		case !applied[version]:
			missing = append(missing, version)
		}
	}

	return pending, missing
}

// appliedVersions returns the versions goose's version table records as
// applied.  The table keeps a row for every time a migration was applied or
// rolled back, and the newest row for a version says which it is now.
func (s *GooseSteps) appliedVersions(ctx context.Context) (map[int64]bool, error) {
	table := goose.TableName()
	if !identifierPattern.MatchString(table) {
		return nil, fmt.Errorf("invalid goose version table name %q", table)
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("SELECT version_id, is_applied FROM %s ORDER BY id DESC", table))
	if err != nil {
		return nil, fmt.Errorf("failed to read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int64]bool{}
	seen := map[int64]bool{}
	for rows.Next() {
		var version int64
		var isApplied bool
		if err := rows.Scan(&version, &isApplied); err != nil {
			return nil, fmt.Errorf("failed to read applied migrations: %w", err)
		}
		if seen[version] {
			continue
		}
		seen[version] = true
		applied[version] = isApplied
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read applied migrations: %w", err)
	}

	return applied, nil
}

// Up applies a migration and records it as applied.
func (s *GooseSteps) Up(ctx context.Context, version int64) error {
	migration, err := s.migration(version)
	if err != nil {
		return err
	}

	return migration.UpContext(ctx, s.db)
}

// Down reverts an applied migration and records it as not applied.
func (s *GooseSteps) Down(ctx context.Context, version int64) error {
	migration, err := s.migration(version)
	if err != nil {
		return err
	}

	return migration.DownContext(ctx, s.db)
}

// CleanUp runs the down migration of a migration that failed before it was
// recorded.
//
// A migration that ran in a transaction cannot be cleaned up after: its down
// migration expects what the migration completes, and CockroachDB does not
// undo every schema change a failed transaction made.  So it is reported as
// not cleaned up rather than assumed clean, which points the operator at the
// backup.  migrationtest rejects such migrations, so none should be
// registered.
func (s *GooseSteps) CleanUp(ctx context.Context, version int64) error {
	migration, err := s.migration(version)
	if err != nil {
		return err
	}
	if migration.UseTx {
		return fmt.Errorf("migration %d ran in a transaction, which may have left schema changes behind on CockroachDB", version)
	}
	if migration.DownFnNoTxContext == nil {
		return fmt.Errorf("migration %d has no down migration to clean up with", version)
	}

	return migration.DownFnNoTxContext(ctx, s.db)
}

// migration returns the migration of a version.
func (s *GooseSteps) migration(version int64) (*goose.Migration, error) {
	for _, migration := range s.migrations {
		if migration.Version == version {
			return migration, nil
		}
	}

	return nil, fmt.Errorf("no migration for version %d", version)
}

// identifierPattern matches a table name that is safe to put in SQL as is.
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// TableFailureRecord records a failed migration run in a table of the
// migrated database, next to goose's version table.
type TableFailureRecord struct {
	db    *sql.DB
	table string
}

// NewTableFailureRecord returns a failure record kept in the given table,
// which is created when it does not exist.
func NewTableFailureRecord(ctx context.Context, db *sql.DB, table string) (*TableFailureRecord, error) {
	if !identifierPattern.MatchString(table) {
		return nil, fmt.Errorf("invalid failure record table name %q", table)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INT8 PRIMARY KEY DEFAULT 1 CHECK (id = 1),
		version INT8 NOT NULL,
		from_version INT8 NOT NULL,
		error STRING NOT NULL,
		rolled_back BOOL NOT NULL,
		backup STRING NOT NULL,
		failed_at TIMESTAMPTZ NOT NULL
	)`, table)); err != nil {
		return nil, fmt.Errorf("failed to create failure record table %s: %w", table, err)
	}

	return &TableFailureRecord{db: db, table: table}, nil
}

// LastFailure returns the recorded failure, or nil when there is none.
func (r *TableFailureRecord) LastFailure(ctx context.Context) (*Failure, error) {
	var failure Failure
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT version, from_version, error, rolled_back, backup, failed_at FROM %s WHERE id = 1",
		r.table,
	)).Scan(
		&failure.Version, &failure.FromVersion, &failure.Error,
		&failure.RolledBack, &failure.Backup, &failure.FailedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &failure, nil
}

// RecordFailure records a failure, replacing any earlier one.
func (r *TableFailureRecord) RecordFailure(ctx context.Context, failure *Failure) error {
	_, err := r.db.ExecContext(ctx, fmt.Sprintf(
		"UPSERT INTO %s (id, version, from_version, error, rolled_back, backup, failed_at) VALUES (1, $1, $2, $3, $4, $5, $6)",
		r.table,
	), failure.Version, failure.FromVersion, failure.Error, failure.RolledBack, failure.Backup, failure.FailedAt)

	return err
}

// ClearInstructions tells an operator how to clear the recorded failure.
func (r *TableFailureRecord) ClearInstructions() string {
	return fmt.Sprintf("clear it by running `DELETE FROM %s` against the database", r.table)
}

// CockroachBackup returns a backup of a CockroachDB database into a
// collection, as a new full backup.  The returned location is the backup's
// path in the collection, which `tptctl restore --backup` takes.
//
// The scheduled backups go into the same collection, so the newest backup in
// it may be one of theirs rather than this one.  Instead the backup is taken
// as of a time read from the cluster first: CockroachDB names a full backup's
// path after the time it is taken as of, so the path is known before the
// backup runs.  It is then read back, and returned only if it is a backup of
// this database as of that time - a backup started in the same hundredth of a
// second gets the same path, and CockroachDB does not always keep the two
// apart, so a path alone does not say the backup there is whole, or this one.
// One that is not is taken again, as of a later time.
func CockroachBackup(db *sql.DB, database, collectionURI string) BackupFunc {
	return func(ctx context.Context) (string, error) {
		if !identifierPattern.MatchString(database) {
			return "", fmt.Errorf("invalid database name %q", database)
		}

		var lastErr error
		tried := map[string]bool{}
		for attempt := 0; attempt < backupAttempts; attempt++ {
			// another backup that took this one's path may be one of a run of
			// them, so the next attempt waits a while, by a varying amount,
			// rather than start in step with the next of those
			if attempt > 0 {
				wait := time.Duration(attempt)*backupRetryDelay + time.Duration(rand.Int63n(int64(backupRetryDelay)))
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(wait):
				}
			}

			// a path already tried holds another backup, and a time read
			// within the same hundredth of a second names it again
			var clusterTime, path string
			var asOf time.Time
			for {
				if err := db.QueryRowContext(ctx, "SELECT cluster_logical_timestamp()::STRING").Scan(&clusterTime); err != nil {
					return "", fmt.Errorf("failed to read the cluster's time to back up as of: %w", err)
				}
				var err error
				path, asOf, err = backupPathAsOf(clusterTime)
				if err != nil {
					return "", err
				}
				if !tried[path] {
					break
				}
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(5 * time.Millisecond):
				}
			}
			tried[path] = true

			_, err := db.ExecContext(ctx, fmt.Sprintf(
				"BACKUP DATABASE %s INTO $1 AS OF SYSTEM TIME %s", database, clusterTime,
			), collectionURI)
			if err != nil {
				if !backupPathTaken(err) {
					return "", fmt.Errorf("failed to back up database %s: %w", database, err)
				}
				lastErr = fmt.Errorf("another backup was writing to %s: %w", path, err)
				continue
			}

			// the backup at the path, read back: whole, and as of this time
			var endTimes int
			var endTime time.Time
			err = db.QueryRowContext(
				ctx, "SELECT count(DISTINCT end_time), max(end_time) FROM [SHOW BACKUP FROM $1 IN $2]",
				path, collectionURI,
			).Scan(&endTimes, &endTime)
			switch {
			case err != nil:
				lastErr = fmt.Errorf("the backup at %s could not be read back: %w", path, err)
			case endTimes != 1:
				lastErr = fmt.Errorf("the backup at %s holds %d backups' worth of times", path, endTimes)
			// CockroachDB keeps the time to the microsecond, rounded
			case endTime.Sub(asOf).Abs() >= time.Microsecond:
				lastErr = fmt.Errorf("the backup at %s is as of %s, not %s", path, endTime.UTC(), asOf)
			default:
				return path, nil
			}
		}

		return "", fmt.Errorf("failed to back up database %s after %d attempts: %w", database, backupAttempts, lastErr)
	}
}

// backupPathTakenMarkers are in the errors CockroachDB returns for a full
// backup into a path another backup is writing to, or has written to.
var backupPathTakenMarkers = []string{
	"already contains a `BACKUP-LOCK` file",
	"A full backup already exists in",
}

// backupPathTaken reports whether a backup failed because another backup took
// its path.
func backupPathTaken(err error) bool {
	for _, marker := range backupPathTakenMarkers {
		if strings.Contains(err.Error(), marker) {
			return true
		}
	}
	return false
}

// backupAttempts is how many times a backup is taken, each as of a new time,
// when another backup took its path.
const backupAttempts = 5

// backupRetryDelay is the step by which the wait before each new attempt
// grows.
const backupRetryDelay = 100 * time.Millisecond

// clusterTimePattern matches a CockroachDB cluster timestamp: wall time in
// nanoseconds since the epoch, then a logical counter.
var clusterTimePattern = regexp.MustCompile(`^(\d+)\.\d+$`)

// backupFolderLayout is how CockroachDB names a full backup's path in its
// collection: after the time the backup is as of, in UTC, to the hundredth of
// a second.
const backupFolderLayout = "/2006/01/02-150405.00"

// backupPathAsOf returns the path in its collection of a full backup taken as
// of a cluster timestamp, and the time it is as of.
func backupPathAsOf(clusterTime string) (string, time.Time, error) {
	match := clusterTimePattern.FindStringSubmatch(clusterTime)
	if match == nil {
		return "", time.Time{}, fmt.Errorf("unexpected cluster timestamp %q", clusterTime)
	}
	wallNanos, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("unexpected cluster timestamp %q: %w", clusterTime, err)
	}
	asOf := time.Unix(0, wallNanos).UTC()

	return asOf.Format(backupFolderLayout), asOf, nil
}
