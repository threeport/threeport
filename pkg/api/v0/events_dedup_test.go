package v0

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// dryRunDB returns a GORM handle that renders SQL without sending it.
// SkipDefaultTransaction is required: DryRun alone still dials for the wrapping transaction.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	// open a postgres-dialect handle that never talks to the database
	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  "postgres://u:p@127.0.0.1:26257/threeport_api?sslmode=disable",
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true},
	)
	require.NoError(t, err, "opening the dry-run handle")
	return db
}

// dedupTestEvent returns one Event with every field the unique index needs.
func dedupTestEvent() *Event {
	now := time.Now()
	return &Event{
		Reason:              strPtr("ScriptFailed"),
		Note:                strPtr("create script failed with exit code 1"),
		Type:                strPtr("Warning"),
		ReportingController: strPtr("MachineWorkloadController"),
		Count:               uintPtr(1),
		EventTime:           &now,
		LastObservedTime:    &now,
		ObjectType:          strPtr("threeport.io/v0.MachineWorkloadInstance"),
		ObjectID:            uintPtr(42),
	}
}

// TestEventCreateUpsertsOnDedupKey covers a create that conflicts on the unique index
// so a repeat updates the existing row instead of inserting another.
func TestEventCreateUpsertsOnDedupKey(t *testing.T) {
	db := dryRunDB(t)

	// render a create
	stmt := db.Create(dedupTestEvent()).Statement
	sql := stmt.SQL.String()

	// conflict target lists the unique index columns
	assert.Contains(t, sql, `ON CONFLICT ("reason","note","type","reporting_controller","object_type","object_id")`,
		"the conflict target must list the index columns:\n%s", sql)
	// conflict target repeats the deleted_at predicate
	assert.Contains(t, sql, "WHERE deleted_at IS NULL DO UPDATE",
		"the conflict target must repeat the index predicate; CockroachDB refuses a partial unique index as an arbiter through ON CONSTRAINT:\n%s", sql)
	// a repeat increments count
	assert.Contains(t, strings.ToLower(sql), "count\"=v0_events.count + 1",
		"a repeat must increment the running count:\n%s", sql)
	// a repeat records the newest sighting
	assert.Contains(t, sql, "excluded.last_observed_time",
		"a repeat must record the newest sighting:\n%s", sql)
	// a repeat leaves event_time as first observed
	assert.NotContains(t, sql, "excluded.event_time",
		"event_time must keep meaning first observed, so a repeat leaves it alone:\n%s", sql)
}

// TestEventCreateWritesSubjectColumns covers the subject columns written on create.
// The unique index spans subject and content, so both live on the event row.
func TestEventCreateWritesSubjectColumns(t *testing.T) {
	db := dryRunDB(t)

	// render a create
	stmt := db.Create(dedupTestEvent()).Statement
	sql := stmt.SQL.String()

	// subject type and id belong on the event row
	assert.Contains(t, sql, "object_type", "subject type belongs on the event row:\n%s", sql)
	assert.Contains(t, sql, "object_id", "subject id belongs on the event row:\n%s", sql)
	// object_name is not stored
	assert.NotContains(t, sql, "object_name",
		"object_name resolves at read time from object_id and must not be stored:\n%s", sql)
}
