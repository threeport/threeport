package v0

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupEventTestDB returns an in-memory sqlite db with Event and
// AttachedObjectReference migrated. The Event hooks under test
// (beforeCreate + afterCreate) write into the AOR table in the
// same transaction, so both must exist.
func setupEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Event{}, &AttachedObjectReference{}))
	return db
}

// baseEvent returns an Event with all required scalar fields
// populated except the subject fields. Tests set ObjectType /
// ObjectID per-case.
func baseEvent() *Event {
	return &Event{
		Reason:              util.Ptr("Reason"),
		Note:                util.Ptr("note"),
		Type:                util.Ptr("Normal"),
		Count:               util.Ptr(uint(1)),
		EventTime:           nowPtr(),
		LastObservedTime:    nowPtr(),
		ReportingController: util.Ptr("test-controller"),
	}
}

// TestEventBeforeCreate_SubjectValidation covers the three reject
// shapes plus the happy path. The hook is the only place that
// guarantees the AOR row's required columns will be non-nil and
// well-formed - if it fails to fire or relaxes validation, afterCreate
// would either panic or write garbage into the AOR table.
func TestEventBeforeCreate_SubjectValidation(t *testing.T) {
	cases := []struct {
		name        string
		objectType  *string
		objectID    *uint
		wantErr     bool
		wantErrSubs []string
	}{
		{
			name:        "missing ObjectType is rejected",
			objectType:  nil,
			objectID:    util.Ptr(uint(1)),
			wantErr:     true,
			wantErrSubs: []string{"requires ObjectType"},
		},
		{
			name:        "missing ObjectID is rejected",
			objectType:  util.Ptr("threeport.io/v0.Foo"),
			objectID:    nil,
			wantErr:     true,
			wantErrSubs: []string{"requires", "ObjectID"},
		},
		{
			name:        "both subject fields missing rejected",
			wantErr:     true,
			wantErrSubs: []string{"requires"},
		},
		{
			name:        "malformed ObjectType rejected (not fully qualified)",
			objectType:  util.Ptr("KubernetesWorkloadInstance"),
			objectID:    util.Ptr(uint(1)),
			wantErr:     true,
			wantErrSubs: []string{"not a fully qualified type name"},
		},
		{
			name:        "missing version segment rejected",
			objectType:  util.Ptr("threeport.io/Widget"),
			objectID:    util.Ptr(uint(1)),
			wantErr:     true,
			wantErrSubs: []string{"not a fully qualified type name"},
		},
		{
			name:       "well-formed core FQT accepted",
			objectType: util.Ptr("threeport.io/v0.Foo"),
			objectID:   util.Ptr(uint(1)),
		},
		{
			name:       "well-formed module FQT accepted",
			objectType: util.Ptr("example.com/v1.Gadget"),
			objectID:   util.Ptr(uint(7)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupEventTestDB(t)
			e := baseEvent()
			e.ObjectType = tc.objectType
			e.ObjectID = tc.objectID

			err := db.Create(e).Error
			if tc.wantErr {
				require.Error(t, err)
				for _, sub := range tc.wantErrSubs {
					assert.Contains(t, err.Error(), sub)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestEventAfterCreate_InsertsAOR verifies the AOR row created by the
// afterCreate hook: subject fields on the base side, Event identity on
// the attacher side, relationship = describes. This is the on-disk
// source of truth for the event-subject linkage; mis-wiring it breaks
// every downstream query (`tptctl events --for ...`, etc).
func TestEventAfterCreate_InsertsAOR(t *testing.T) {
	db := setupEventTestDB(t)
	e := baseEvent()
	e.ObjectType = util.Ptr("threeport.io/v0.KubernetesWorkloadInstance")
	e.ObjectID = util.Ptr(uint(42))

	require.NoError(t, db.Create(e).Error)
	require.NotNil(t, e.ID, "Create should populate Event.ID")

	var aor AttachedObjectReference
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", "threeport.io/v0.Event", *e.ID).
		First(&aor).Error)

	require.NotNil(t, aor.ObjectType)
	require.NotNil(t, aor.ObjectID)
	require.NotNil(t, aor.AttachedObjectType)
	require.NotNil(t, aor.AttachedObjectID)
	require.NotNil(t, aor.Relationship)

	assert.Equal(t, "threeport.io/v0.KubernetesWorkloadInstance", *aor.ObjectType)
	assert.Equal(t, uint(42), *aor.ObjectID)
	assert.Equal(t, "threeport.io/v0.Event", *aor.AttachedObjectType)
	assert.Equal(t, *e.ID, *aor.AttachedObjectID)
	assert.Equal(t, RelationshipDescribes, *aor.Relationship,
		"events describe their subject; never owns/marries/requires")
}

// TestEventAfterCreate_MultipleEventsSameSubject confirms that two
// events about the same subject produce two distinct AOR rows -
// uniqueness on the attacher (event) side, not on the base
// (subject) side. This is the dedup-by-event-id assumption that
// enrichEventsWithObjectInfo relies on when building its
// per-event aorByEventID map.
func TestEventAfterCreate_MultipleEventsSameSubject(t *testing.T) {
	db := setupEventTestDB(t)

	subjectType := "threeport.io/v0.KubernetesWorkloadInstance"
	subjectID := uint(42)

	e1 := baseEvent()
	e1.ObjectType = util.Ptr(subjectType)
	e1.ObjectID = util.Ptr(subjectID)
	e1.Reason = util.Ptr("FirstReason")
	require.NoError(t, db.Create(e1).Error)

	e2 := baseEvent()
	e2.ObjectType = util.Ptr(subjectType)
	e2.ObjectID = util.Ptr(subjectID)
	e2.Reason = util.Ptr("SecondReason")
	require.NoError(t, db.Create(e2).Error)

	var aors []AttachedObjectReference
	require.NoError(t, db.
		Where("object_type = ? AND object_id = ?", subjectType, subjectID).
		Find(&aors).Error)

	assert.Len(t, aors, 2, "each event gets its own AOR row")
}

// nowPtr returns a *time.Time pointing at the current time. Inline
// helper to keep baseEvent() readable.
func nowPtr() *time.Time {
	t := time.Now()
	return &t
}
