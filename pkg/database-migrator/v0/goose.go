package v0

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

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
func (s *GooseSteps) PendingVersions(ctx context.Context) ([]int64, error) {
	current, err := s.CurrentVersion(ctx)
	if err != nil {
		return nil, err
	}

	var pending []int64
	for _, migration := range s.migrations {
		if migration.Version > current {
			pending = append(pending, migration.Version)
		}
	}

	return pending, nil
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
// recorded.  A migration that runs in a transaction left nothing behind, so
// only one that runs outside of one is cleaned up.
func (s *GooseSteps) CleanUp(ctx context.Context, version int64) error {
	migration, err := s.migration(version)
	if err != nil {
		return err
	}
	if migration.UseTx {
		return nil
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
func CockroachBackup(db *sql.DB, database, collectionURI string) BackupFunc {
	return func(ctx context.Context) (string, error) {
		if !identifierPattern.MatchString(database) {
			return "", fmt.Errorf("invalid database name %q", database)
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("BACKUP DATABASE %s INTO $1", database), collectionURI); err != nil {
			return "", fmt.Errorf("failed to back up database %s: %w", database, err)
		}

		var path string
		if err := db.QueryRowContext(
			ctx, "SELECT path FROM [SHOW BACKUPS IN $1] ORDER BY path DESC LIMIT 1", collectionURI,
		).Scan(&path); err != nil {
			return "", fmt.Errorf("failed to find the backup just taken: %w", err)
		}

		return path, nil
	}
}
