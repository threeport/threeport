package v0

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSteps is a database whose migrations are versions, recording every
// step taken in order.
type fakeSteps struct {
	applied  []int64
	all      []int64
	failUp   map[int64]bool
	failDown map[int64]bool
	calls    []string
}

func newFakeSteps(applied, all []int64) *fakeSteps {
	return &fakeSteps{applied: applied, all: all, failUp: map[int64]bool{}, failDown: map[int64]bool{}}
}

func (s *fakeSteps) CurrentVersion(context.Context) (int64, error) {
	if len(s.applied) == 0 {
		return 0, nil
	}
	return s.applied[len(s.applied)-1], nil
}

func (s *fakeSteps) PendingVersions(ctx context.Context) ([]int64, error) {
	current, _ := s.CurrentVersion(ctx)
	var pending []int64
	for _, v := range s.all {
		if v > current {
			pending = append(pending, v)
		}
	}
	return pending, nil
}

func (s *fakeSteps) Up(_ context.Context, version int64) error {
	s.calls = append(s.calls, fmt.Sprintf("up %d", version))
	if s.failUp[version] {
		return fmt.Errorf("column already exists")
	}
	s.applied = append(s.applied, version)
	return nil
}

func (s *fakeSteps) Down(_ context.Context, version int64) error {
	s.calls = append(s.calls, fmt.Sprintf("down %d", version))
	if s.failDown[version] {
		return errors.New("cannot drop")
	}
	s.applied = s.applied[:len(s.applied)-1]
	return nil
}

func (s *fakeSteps) CleanUp(_ context.Context, version int64) error {
	s.calls = append(s.calls, fmt.Sprintf("clean up %d", version))
	if s.failDown[version] {
		return errors.New("cannot drop")
	}
	return nil
}

// fakeRecord keeps a failure in memory.
type fakeRecord struct{ failure *Failure }

func (r *fakeRecord) LastFailure(context.Context) (*Failure, error) { return r.failure, nil }

func (r *fakeRecord) RecordFailure(_ context.Context, failure *Failure) error {
	r.failure = failure
	return nil
}

func (r *fakeRecord) ClearInstructions() string { return "clear the record" }

// countingBackup counts backups and returns a backup path.
func countingBackup(count *int) BackupFunc {
	return func(context.Context) (string, error) {
		*count++
		return "/2026/10/20-171732.15", nil
	}
}

// TestNothingPendingIsANoOp covers every start of a control plane whose
// database is up to date: nothing is applied and no backup is taken.
func TestNothingPendingIsANoOp(t *testing.T) {
	steps := newFakeSteps([]int64{1, 2}, []int64{1, 2})
	backups := 0

	require.NoError(t, Up(context.Background(), steps, &fakeRecord{}, Options{Backup: countingBackup(&backups)}))
	assert.Empty(t, steps.calls)
	assert.Zero(t, backups)
}

// TestPendingMigrationsAreBackedUpThenApplied covers an upgrade: one backup
// before anything changes, then each migration in order.
func TestPendingMigrationsAreBackedUpThenApplied(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2, 3})
	backups := 0

	require.NoError(t, Up(context.Background(), steps, &fakeRecord{}, Options{Backup: countingBackup(&backups)}))
	assert.Equal(t, []string{"up 2", "up 3"}, steps.calls)
	assert.Equal(t, []int64{1, 2, 3}, steps.applied)
	assert.Equal(t, 1, backups)
}

// TestAFailedMigrationRollsBackToWhereTheRunStarted covers the issue's
// acceptance criteria: a migration that fails is cleaned up, the ones applied
// before it in the same run are rolled back newest first, and the failure is
// recorded and reported, leaving the database at its starting version.
func TestAFailedMigrationRollsBackToWhereTheRunStarted(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2, 3, 4})
	steps.failUp[4] = true
	record := &fakeRecord{}

	err := Up(context.Background(), steps, record, Options{Backup: countingBackup(new(int))})

	require.ErrorIs(t, err, ErrMigrationFailed)
	assert.Contains(t, err.Error(), "migration 4 failed")
	assert.Contains(t, err.Error(), "rolled back to version 1")
	assert.Contains(t, err.Error(), "clear the record")
	assert.Equal(t, []string{"up 2", "up 3", "up 4", "clean up 4", "down 3", "down 2"}, steps.calls)
	assert.Equal(t, []int64{1}, steps.applied)

	require.NotNil(t, record.failure)
	assert.Equal(t, int64(4), record.failure.Version)
	assert.Equal(t, int64(1), record.failure.FromVersion)
	assert.True(t, record.failure.RolledBack)
	assert.Equal(t, "/2026/10/20-171732.15", record.failure.Backup)
}

// TestAFailedRollbackPointsAtTheBackup covers a rollback that fails too: the
// database is between versions, so the error names the backup taken before
// migrating, and nothing older is rolled back under the migration that could
// not be.
func TestAFailedRollbackPointsAtTheBackup(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2, 3, 4})
	steps.failUp[4] = true
	steps.failDown[3] = true
	record := &fakeRecord{}

	err := Up(context.Background(), steps, record, Options{Backup: countingBackup(new(int))})

	require.ErrorIs(t, err, ErrMigrationFailed)
	assert.Contains(t, err.Error(), "between version 1 and version 4")
	assert.Contains(t, err.Error(), "restore the backup taken before migrating, /2026/10/20-171732.15")
	assert.Equal(t, []string{"up 2", "up 3", "up 4", "clean up 4", "down 3"}, steps.calls)
	assert.False(t, record.failure.RolledBack)
}

// TestARecordedFailureStopsLaterRuns covers the migrator restarting after a
// failure, as an init container does: it neither migrates nor backs up
// again, and keeps reporting the failure until it is cleared.
func TestARecordedFailureStopsLaterRuns(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	record := &fakeRecord{failure: &Failure{Version: 2, FromVersion: 1, Error: "boom", RolledBack: true}}
	backups := 0

	err := Up(context.Background(), steps, record, Options{Backup: countingBackup(&backups)})

	require.ErrorIs(t, err, ErrMigrationFailed)
	assert.Contains(t, err.Error(), "migration 2 failed")
	assert.Contains(t, err.Error(), "clear the record")
	assert.Empty(t, steps.calls)
	assert.Zero(t, backups)
}

// TestAFailedBackupStopsTheMigration covers a backup that cannot be taken:
// nothing is migrated without one.
func TestAFailedBackupStopsTheMigration(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})

	err := Up(context.Background(), steps, &fakeRecord{}, Options{Backup: func(context.Context) (string, error) {
		return "", errors.New("access denied")
	}})

	assert.ErrorContains(t, err, "no migration was applied")
	assert.Empty(t, steps.calls)
}

// TestWithoutABackupMigrationsStillRollBack covers a development control
// plane or a module, which take no backup: a failed migration is rolled back
// all the same.
func TestWithoutABackupMigrationsStillRollBack(t *testing.T) {
	steps := newFakeSteps(nil, []int64{1, 2})
	steps.failUp[2] = true
	record := &fakeRecord{}

	err := Up(context.Background(), steps, record, Options{})

	require.ErrorIs(t, err, ErrMigrationFailed)
	assert.Equal(t, []string{"up 1", "up 2", "clean up 2", "down 1"}, steps.calls)
	assert.Empty(t, steps.applied)
	assert.Empty(t, record.failure.Backup)
}

// TestANewInstallIsNotBackedUp covers the first migration of a new install:
// the database is empty, so there is nothing to back up.
func TestANewInstallIsNotBackedUp(t *testing.T) {
	steps := newFakeSteps(nil, []int64{1, 2})
	backups := 0

	require.NoError(t, Up(context.Background(), steps, &fakeRecord{}, Options{Backup: countingBackup(&backups)}))
	assert.Equal(t, []int64{1, 2}, steps.applied)
	assert.Zero(t, backups)
}
