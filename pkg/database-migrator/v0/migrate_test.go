package v0

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// failingRecord cannot store a failure.
type failingRecord struct{ fakeRecord }

func (r *failingRecord) RecordFailure(context.Context, *Failure) error {
	return errors.New("connection reset")
}

// TestAFailureThatCannotBeRecordedIsSaid covers a rolled back migration
// whose failure could not be recorded: the next run will try it again, so
// the error says so, carries why, and does not point at a record to clear.
func TestAFailureThatCannotBeRecordedIsSaid(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	steps.failUp[2] = true

	err := Up(context.Background(), steps, &failingRecord{}, Options{})

	require.ErrorIs(t, err, ErrMigrationFailed)
	assert.Contains(t, err.Error(), "rolled back to version 1")
	assert.Contains(t, err.Error(), "failed to record the failure, so the next run will try the migration again")
	assert.Contains(t, err.Error(), "connection reset")
	assert.NotContains(t, err.Error(), "clear the record")
}

// TestAFailedRollbackAndRecordReportsBoth covers a rollback that fails and a
// failure that cannot be recorded: both are in the error.
func TestAFailedRollbackAndRecordReportsBoth(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	steps.failUp[2] = true
	steps.failDown[2] = true

	err := Up(context.Background(), steps, &failingRecord{}, Options{})

	require.ErrorIs(t, err, ErrMigrationFailed)
	assert.Contains(t, err.Error(), "failed to clean up migration 2")
	assert.Contains(t, err.Error(), "connection reset")
}

// fakeLock records when it is acquired and released relative to the steps.
type fakeLock struct {
	steps      *fakeSteps
	acquireErr error
	releaseErr error
	events     []string
	// how many checks pass before the lock is lost; negative never loses it
	checksLeft int
}

func (l *fakeLock) Acquire(context.Context) error {
	l.events = append(l.events, fmt.Sprintf("acquire after %d steps", len(l.steps.calls)))
	return l.acquireErr
}

func (l *fakeLock) Check(context.Context) error {
	if l.checksLeft == 0 {
		return ErrLockLost
	}
	l.checksLeft--
	return nil
}

func (l *fakeLock) Release(context.Context) error {
	l.events = append(l.events, fmt.Sprintf("release after %d steps", len(l.steps.calls)))
	return l.releaseErr
}

// TestTheLockCoversTheWholeRun covers two migrators of one database started
// together, as two API server pods are: each holds the lock from before it
// reads anything until it is done, so the second sees what the first did
// rather than racing it to the same migration.
func TestTheLockCoversTheWholeRun(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2, 3})
	lock := &fakeLock{steps: steps, checksLeft: -1}

	require.NoError(t, Up(context.Background(), steps, &fakeRecord{}, Options{Lock: lock}))
	assert.Equal(t, []string{"acquire after 0 steps", "release after 2 steps"}, lock.events)

	failing := newFakeSteps([]int64{1}, []int64{1, 2})
	failing.failUp[2] = true
	lock = &fakeLock{steps: failing, checksLeft: -1}
	require.Error(t, Up(context.Background(), failing, &fakeRecord{}, Options{Lock: lock}))
	assert.Equal(t, "release after 2 steps", lock.events[len(lock.events)-1], "the lock is held until the rollback is done")
}

// TestNoLockNoMigration covers a lock that cannot be acquired: nothing is
// read, backed up or migrated.
func TestNoLockNoMigration(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	backups := 0
	lock := &fakeLock{steps: steps, acquireErr: errors.New("timed out")}

	err := Up(context.Background(), steps, &fakeRecord{}, Options{Lock: lock, Backup: countingBackup(&backups)})

	assert.ErrorContains(t, err, "nothing was migrated")
	assert.Empty(t, steps.calls)
	assert.Zero(t, backups)
}

// TestALockThatCannotBeReleasedDoesNotFailTheRun covers a release that fails
// after a run that migrated: the lock expires on its own, so the run still
// succeeds.
func TestALockThatCannotBeReleasedDoesNotFailTheRun(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	lock := &fakeLock{steps: steps, releaseErr: errors.New("connection reset"), checksLeft: -1}

	assert.NoError(t, Up(context.Background(), steps, &fakeRecord{}, Options{Lock: lock}))
}

// TestALostLockStopsTheRun covers a migrator that loses the lock part way,
// as one cut off from the database past its lease does while another takes
// over: it makes no further change, rolls nothing back, records nothing, and
// says where it left the database.
func TestALostLockStopsTheRun(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2, 3, 4})
	record := &fakeRecord{}
	// the backup and migration 2 pass their checks, migration 3's fails
	lock := &fakeLock{steps: steps, checksLeft: 2}

	err := Up(context.Background(), steps, record, Options{Lock: lock, Backup: countingBackup(new(int))})

	require.ErrorIs(t, err, ErrLockLost)
	assert.Contains(t, err.Error(), "stopped with the database at version 2, having started at version 1")
	assert.Contains(t, err.Error(), "/2026/10/20-171732.15")
	assert.Equal(t, []string{"up 2"}, steps.calls)
	assert.Nil(t, record.failure)
}

// TestALostLockStopsTheBackup covers a lock lost before the backup: nothing
// is backed up or migrated.
func TestALostLockStopsTheBackup(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	backups := 0
	lock := &fakeLock{steps: steps, checksLeft: 0}

	err := Up(context.Background(), steps, &fakeRecord{}, Options{Lock: lock, Backup: countingBackup(&backups)})

	require.ErrorIs(t, err, ErrLockLost)
	assert.Zero(t, backups)
	assert.Empty(t, steps.calls)
}

// TestALostLockStopsTheRollback covers a lock lost while rolling a failed
// migration back: the rollback stops where it is, rather than undoing what
// another migrator may be relying on, and the failure is not recorded under
// the other's lock.  The error keeps what recovering needs: that the schema
// may be incomplete and why, the clean-up's error, and the backup.
func TestALostLockStopsTheRollback(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2, 3})
	steps.failUp[3] = true
	steps.failDown[3] = true
	record := &fakeRecord{}
	// the backup, migrations 2 and 3 and the clean-up pass their checks, the
	// down of 2 fails
	lock := &fakeLock{steps: steps, checksLeft: 4}

	err := Up(context.Background(), steps, record, Options{Lock: lock, Backup: countingBackup(new(int))})

	require.ErrorIs(t, err, ErrLockLost)
	require.ErrorIs(t, err, ErrMigrationFailed)
	for _, want := range []string{
		"its rollback stopped",
		"the schema may be incomplete",
		"migration 3 may have left changes behind",
		"migrations 2 applied in this run are still applied",
		"failed to clean up migration 3",
		"/2026/10/20-171732.15",
	} {
		assert.Contains(t, err.Error(), want)
	}
	assert.Equal(t, []string{"up 2", "up 3", "clean up 3"}, steps.calls)
	assert.Nil(t, record.failure)
}

// TestALostLockBeforeTheRollbackSaysSo covers a lock lost right after a
// migration failed, before anything was rolled back: the failed migration
// may have left changes, and the error says so with the backup.
func TestALostLockBeforeTheRollbackSaysSo(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	steps.failUp[2] = true
	// the backup and migration 2 pass their checks, the clean-up's fails
	lock := &fakeLock{steps: steps, checksLeft: 2}

	err := Up(context.Background(), steps, &fakeRecord{}, Options{Lock: lock, Backup: countingBackup(new(int))})

	require.ErrorIs(t, err, ErrLockLost)
	assert.Contains(t, err.Error(), "migration 2 may have left changes behind")
	assert.Contains(t, err.Error(), "/2026/10/20-171732.15")
	assert.Equal(t, []string{"up 2"}, steps.calls)
}

// TestALostLockStopsTheFailureRecord covers a lock lost after a rollback,
// just before the failure would be recorded: the record is not written under
// another migrator's lock, and the error says the failure went unrecorded.
func TestALostLockStopsTheFailureRecord(t *testing.T) {
	steps := newFakeSteps([]int64{1}, []int64{1, 2})
	steps.failUp[2] = true
	record := &fakeRecord{}
	// migration 2 and its clean-up pass their checks, the record's fails
	lock := &fakeLock{steps: steps, checksLeft: 2}

	err := Up(context.Background(), steps, record, Options{Lock: lock})

	require.ErrorIs(t, err, ErrLockLost)
	assert.Contains(t, err.Error(), "failed to record the failure")
	assert.Nil(t, record.failure)
}

// recordingLock records what a run did with the migration lock.
type recordingLock struct {
	acquireErr, releaseErr error
	events                 []string
}

func (l *recordingLock) Acquire(context.Context) error {
	l.events = append(l.events, "acquire")
	return l.acquireErr
}
func (l *recordingLock) Check(context.Context) error { return nil }
func (l *recordingLock) Release(context.Context) error {
	l.events = append(l.events, "release")
	return l.releaseErr
}

// TestWithLockHoldsTheLockAroundTheChange covers the goose commands other
// than a plain up, such as a down run by hand: the change runs only while the
// lock is held, and the lock is given up afterwards whether it succeeded or
// not.
func TestWithLockHoldsTheLockAroundTheChange(t *testing.T) {
	lock := &recordingLock{}
	err := WithLock(context.Background(), lock, nil, func() error {
		lock.events = append(lock.events, "change")
		return errors.New("down failed")
	})
	if err == nil || err.Error() != "down failed" {
		t.Errorf("the change's error was not returned: %v", err)
	}
	if strings.Join(lock.events, ",") != "acquire,change,release" {
		t.Errorf("events = %v", lock.events)
	}

	t.Run("no change without the lock", func(t *testing.T) {
		lock := &recordingLock{acquireErr: errors.New("held by another migrator")}
		changed := false
		err := WithLock(context.Background(), lock, nil, func() error { changed = true; return nil })
		if err == nil || changed {
			t.Errorf("changed without the lock (err %v)", err)
		}
	})

	t.Run("a lock that cannot be released does not fail a change that was made", func(t *testing.T) {
		lock := &recordingLock{releaseErr: errors.New("connection reset")}
		var logged []string
		err := WithLock(context.Background(), lock, func(format string, args ...interface{}) {
			logged = append(logged, fmt.Sprintf(format, args...))
		}, func() error { return nil })
		if err != nil {
			t.Errorf("a made change failed: %v", err)
		}
		if len(logged) != 1 || !strings.Contains(logged[0], "expires on its own") {
			t.Errorf("logged = %v", logged)
		}
	})
}
