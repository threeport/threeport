package v0

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// returning includes the columns an upsert mutates so the in-memory struct
	// matches the persisted row
	assert.Contains(t, sql, `RETURNING "id","count","last_observed_time","event_time","created_at","updated_at"`,
		"create must return the upserted count and last_observed_time:\n%s", sql)
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

// TestEventCreateReloadsCountOnConflict covers a second create of the same
// event refreshing Count and LastObservedTime on the in-memory struct.
func TestEventCreateReloadsCountOnConflict(t *testing.T) {
	// open an in-memory event table
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err, "opening sqlite")
	require.NoError(t, db.AutoMigrate(&Event{}), "migrating events")

	// insert the first occurrence
	first := dedupTestEvent()
	require.NoError(t, db.Create(first).Error, "inserting the first occurrence")
	require.NotNil(t, first.Count)
	assert.Equal(t, uint(1), *first.Count)

	firstObserved := *first.EventTime
	time.Sleep(time.Millisecond)

	// upsert a repeat and check the dest struct
	repeat := dedupTestEvent()
	require.NoError(t, db.Create(repeat).Error, "upserting a repeat")

	require.NotNil(t, repeat.ID)
	assert.Equal(t, *first.ID, *repeat.ID, "a repeat must keep the original row")
	require.NotNil(t, repeat.Count)
	assert.Equal(t, uint(2), *repeat.Count, "the create dest must carry the incremented count")
	require.NotNil(t, repeat.LastObservedTime)
	assert.True(t, repeat.LastObservedTime.After(firstObserved),
		"the create dest must carry the newest sighting")
	require.NotNil(t, repeat.EventTime)
	assert.True(t, repeat.EventTime.Equal(firstObserved),
		"event_time must stay the first observation")
}

// TestEventCreateColumnListsMatchSchema covers the ON CONFLICT and RETURNING
// column strings staying aligned with Event's unique index and upsert assignments.
func TestEventCreateColumnListsMatchSchema(t *testing.T) {
	stmt := dryRunDB(t).Create(dedupTestEvent()).Statement
	require.NotNil(t, stmt.Schema)

	// ON CONFLICT columns must be idx_events_dedup
	idx := stmt.Schema.LookIndex("idx_events_dedup")
	require.NotNil(t, idx, "Event must declare uniqueIndex idx_events_dedup")
	var indexCols []string
	for _, f := range idx.Fields {
		indexCols = append(indexCols, f.DBName)
	}
	var conflictCols []string
	for _, c := range eventDedupColumns {
		conflictCols = append(conflictCols, c.Name)
	}
	assert.Equal(t, indexCols, conflictCols,
		"eventDedupColumns must match idx_events_dedup")

	// RETURNING must include the primary key, every upserted column, and event_time
	onConflict, ok := stmt.Clauses["ON CONFLICT"].Expression.(clause.OnConflict)
	require.True(t, ok, "create must attach ON CONFLICT")
	returning, ok := stmt.Clauses["RETURNING"].Expression.(clause.Returning)
	require.True(t, ok, "create must attach RETURNING")
	returned := map[string]bool{}
	for _, c := range returning.Columns {
		returned[c.Name] = true
	}
	require.True(t, returned["id"], "RETURNING must include id")
	require.True(t, returned["event_time"],
		"RETURNING must include event_time so a repeat keeps first observed")
	for _, assignment := range onConflict.DoUpdates {
		assert.True(t, returned[assignment.Column.Name],
			"RETURNING must include upserted column %s", assignment.Column.Name)
	}
}
