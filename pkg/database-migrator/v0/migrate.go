// Package v0 applies database migrations so that a failed one leaves the
// database where it started rather than part way through.
//
// Migrations run one at a time.  When one fails, its own down migration
// cleans up whatever it changed before failing, and the migrations applied
// earlier in the same run are rolled back, newest first.  On a control plane
// that backs its database up, a backup is taken before the first migration,
// for the case where the rollback fails too.
//
// A failure is recorded in the database and stops every later run until an
// operator clears it.  A migrator that runs as an init container is restarted
// by Kubernetes, and retrying a migration that just failed would only fail
// again, taking a fresh backup each time.
package v0

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Steps are the migrations of one database.
type Steps interface {
	// CurrentVersion returns the version of the last migration applied.
	CurrentVersion(ctx context.Context) (int64, error)

	// PendingVersions returns the versions of the migrations not yet
	// applied, oldest first.
	PendingVersions(ctx context.Context) ([]int64, error)

	// Up applies a migration and records it as applied.
	Up(ctx context.Context, version int64) error

	// Down reverts an applied migration and records it as not applied.
	Down(ctx context.Context, version int64) error

	// CleanUp runs the down migration of a migration that failed before it
	// was recorded as applied, to remove whatever it changed before failing.
	CleanUp(ctx context.Context, version int64) error
}

// Failure is a migration run that failed.
type Failure struct {
	// The version of the migration that failed.
	Version int64

	// The version the database was at before the run.
	FromVersion int64

	// The migration's error.
	Error string

	// Whether the database was rolled back to FromVersion.  When it was not,
	// the database is somewhere between the two versions.
	RolledBack bool

	// The backup taken before the run, empty when none was taken.
	Backup string

	// When the migration failed.
	FailedAt time.Time
}

// FailureRecord stores the last failed migration run of a database.
type FailureRecord interface {
	// LastFailure returns the recorded failure, or nil when there is none.
	LastFailure(ctx context.Context) (*Failure, error)

	// RecordFailure records a failure.
	RecordFailure(ctx context.Context, failure *Failure) error

	// ClearInstructions tells an operator how to clear the recorded failure
	// so the next run migrates again.
	ClearInstructions() string
}

// BackupFunc backs the database up and returns where the backup is.
type BackupFunc func(ctx context.Context) (string, error)

// Options configures a migration run.
type Options struct {
	// Backup backs the database up before the first migration, unless no
	// migration has been applied yet.  Nil runs the migrations without a
	// backup.
	Backup BackupFunc

	// Logf reports progress.  Nil reports nothing.
	Logf func(format string, args ...interface{})
}

// ErrMigrationFailed is returned when a migration failed in this run or a
// failure recorded by an earlier run has not been cleared.
var ErrMigrationFailed = errors.New("database migration failed")

// Up applies every pending migration.  See the package documentation for
// what happens when one fails.
func Up(ctx context.Context, steps Steps, record FailureRecord, opts Options) error {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}

	// refuse to run again until an earlier failure is looked at
	previous, err := record.LastFailure(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the record of failed migrations: %w", err)
	}
	if previous != nil {
		return fmt.Errorf(
			"%w: %s; migrations stay stopped until this is looked at, then %s",
			ErrMigrationFailed, describe(previous), record.ClearInstructions(),
		)
	}

	pending, err := steps.PendingVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list pending migrations: %w", err)
	}
	if len(pending) == 0 {
		logf("no pending migrations")
		return nil
	}
	from, err := steps.CurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the database version: %w", err)
	}

	// back up before changing anything, unless there is nothing yet: a new
	// install migrates an empty database
	var backup string
	if opts.Backup != nil && from > 0 {
		logf("backing the database up before migrating from version %d", from)
		backup, err = opts.Backup(ctx)
		if err != nil {
			return fmt.Errorf("failed to back the database up before migrating, so no migration was applied: %w", err)
		}
		logf("database backed up to %s", backup)
	}

	var applied []int64
	for _, version := range pending {
		logf("applying migration %d", version)
		migrateErr := steps.Up(ctx, version)
		if migrateErr == nil {
			applied = append(applied, version)
			continue
		}

		// undo the failed migration, then the ones before it in this run
		logf("migration %d failed, rolling back to version %d: %v", version, from, migrateErr)
		var rollbackErrors []error
		if err := steps.CleanUp(ctx, version); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to clean up migration %d: %w", version, err))
		}
		for i := len(applied) - 1; i >= 0; i-- {
			if err := steps.Down(ctx, applied[i]); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("failed to roll back migration %d: %w", applied[i], err))
				// an older migration cannot be rolled back under a newer one
				break
			}
		}
		rollbackErr := errors.Join(rollbackErrors...)

		failure := &Failure{
			Version:     version,
			FromVersion: from,
			Error:       migrateErr.Error(),
			RolledBack:  rollbackErr == nil,
			Backup:      backup,
			FailedAt:    time.Now().UTC(),
		}
		// a failure that could not be recorded does not stop the next run,
		// which tries the migration again, so it is said rather than
		// pointing at a record to clear
		recordErr := record.RecordFailure(ctx, failure)
		if recordErr != nil {
			recordErr = fmt.Errorf(
				"failed to record the failure, so the next run will try the migration again: %w",
				recordErr,
			)
		}

		if rollbackErr != nil {
			return fmt.Errorf("%w: %s: %w", ErrMigrationFailed, describe(failure), errors.Join(rollbackErr, recordErr))
		}
		if recordErr != nil {
			return fmt.Errorf("%w: %s; fix the migration: %w", ErrMigrationFailed, describe(failure), recordErr)
		}
		return fmt.Errorf("%w: %s; fix the migration, then %s", ErrMigrationFailed, describe(failure), record.ClearInstructions())
	}

	logf("migrated the database from version %d to %d", from, applied[len(applied)-1])

	return nil
}

// describe says what happened to a failed run and where it left the
// database.
func describe(failure *Failure) string {
	text := fmt.Sprintf(
		"migration %d failed at %s: %s",
		failure.Version, failure.FailedAt.Format(time.RFC3339), failure.Error,
	)
	if failure.RolledBack {
		return fmt.Sprintf("%s; the database was rolled back to version %d", text, failure.FromVersion)
	}

	text = fmt.Sprintf(
		"%s; rolling it back failed, so the database is between version %d and version %d",
		text, failure.FromVersion, failure.Version,
	)
	if failure.Backup != "" {
		return fmt.Sprintf("%s; restore the backup taken before migrating, %s", text, failure.Backup)
	}

	return text
}
