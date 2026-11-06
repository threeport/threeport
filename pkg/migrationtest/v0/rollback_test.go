package migrationtest

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	goose "github.com/pressly/goose/v3"
)

func noop(context.Context, *sql.DB) error { return nil }

// TestConventionViolations covers the convention a failed migration's
// rollback relies on.  A migration with no down migration cannot be rolled
// back past, and one that runs in a transaction would be trusted to have
// undone itself on failing, which CockroachDB does not do for every schema
// change - so each is reported, and one that follows both rules is not.
func TestConventionViolations(t *testing.T) {
	violations := ConventionViolations(goose.Migrations{
		{Version: 1, UpFnNoTxContext: noop, DownFnNoTxContext: noop},
		{Version: 2, UpFnNoTxContext: noop},
		{Version: 3, UseTx: true, UpFnContext: func(context.Context, *sql.Tx) error { return nil }, DownFnContext: func(context.Context, *sql.Tx) error { return nil }},
	})

	if len(violations) != 2 {
		t.Fatalf("violations = %v, want one for each of migrations 2 and 3", violations)
	}
	if !strings.Contains(violations[0], "migration 2 has no down migration") {
		t.Errorf("violation = %q", violations[0])
	}
	if !strings.Contains(violations[1], "migration 3 runs in a transaction") ||
		!strings.Contains(violations[1], "AddMigrationNoTxContext") {
		t.Errorf("violation = %q", violations[1])
	}
}
