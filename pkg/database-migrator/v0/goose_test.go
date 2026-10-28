package v0

import (
	"context"
	"strings"
	"testing"
	"time"

	goose "github.com/pressly/goose/v3"
)

// TestPendingAndMissing covers which registered migrations an up applies,
// and which it refuses to skip.  A migration that sorts below one already
// applied was merged out of order: never applied, it would never be pending
// again either.
func TestPendingAndMissing(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		registered       []int64
		applied          map[int64]bool
		current          int64
		pending, missing []int64
	}{
		{
			name:       "a new install has every migration pending",
			registered: []int64{1, 2, 3},
			applied:    map[int64]bool{},
			current:    0,
			pending:    []int64{1, 2, 3},
		},
		{
			name:       "migrations newer than the database are pending",
			registered: []int64{1, 2, 3},
			applied:    map[int64]bool{1: true, 2: true},
			current:    2,
			pending:    []int64{3},
		},
		{
			name:       "a migration below the current version that never ran is missing",
			registered: []int64{1, 2, 3, 4},
			applied:    map[int64]bool{1: true, 3: true},
			current:    3,
			pending:    []int64{4},
			missing:    []int64{2},
		},
		{
			name:       "a migration rolled back below the current version is missing too",
			registered: []int64{1, 2, 3},
			applied:    map[int64]bool{1: true, 2: false, 3: true},
			current:    3,
			missing:    []int64{2},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			pending, missing := pendingAndMissing(testCase.registered, testCase.applied, testCase.current)
			if !equalVersions(pending, testCase.pending) {
				t.Errorf("pending = %v, want %v", pending, testCase.pending)
			}
			if !equalVersions(missing, testCase.missing) {
				t.Errorf("missing = %v, want %v", missing, testCase.missing)
			}
		})
	}
}

func equalVersions(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// TestBackupPathAsOf covers the path a backup taken as of a cluster timestamp
// gets in its collection: the timestamp's wall time, in UTC, to the hundredth
// of a second.  test/cockroach confirms it against a real CockroachDB.
func TestBackupPathAsOf(t *testing.T) {
	asOf := time.Date(2026, 10, 23, 22, 50, 43, 987654321, time.UTC)
	path, at, err := backupPathAsOf("1792795843987654321.0000000003")
	if err != nil {
		t.Fatal(err)
	}
	if !at.Equal(asOf) {
		t.Errorf("as of %s, want %s", at, asOf)
	}
	if want := asOf.Format("/2006/01/02-150405.00"); path != want || path != "/2026/10/23-225043.98" {
		t.Errorf("path = %q, want %q", path, want)
	}

	for _, malformed := range []string{"", "now", "1792795843987654321", "-1.0", "1.2.3"} {
		if _, _, err := backupPathAsOf(malformed); err == nil {
			t.Errorf("accepted %q as a cluster timestamp", malformed)
		}
	}
}

// TestCleanUpDoesNotTrustATransaction covers a migration that ran in a
// transaction and failed.  CockroachDB does not undo every schema change a
// failed transaction made, so it is reported as not cleaned up - which points
// the operator at the backup - rather than as rolled back.
func TestCleanUpDoesNotTrustATransaction(t *testing.T) {
	steps := NewGooseSteps(nil, goose.Migrations{{Version: 7, UseTx: true}})
	err := steps.CleanUp(context.Background(), 7)
	if err == nil || !strings.Contains(err.Error(), "ran in a transaction") {
		t.Errorf("a failed transactional migration was taken as cleaned up: %v", err)
	}
}
