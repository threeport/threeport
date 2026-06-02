package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupEventJoinTestDB returns an in-memory sqlite db with Event and
// AttachedObjectReference tables migrated. Used to exercise the
// polymorphic JOIN shape the events handler relies on.
func setupEventJoinTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&api.Event{}, &api.AttachedObjectReference{}))
	return db
}

// TestEventsJoin_ExcludesSoftDeletedAOR verifies the JOIN shape used
// by GetEventsJoinAttachedObjectReferences applies
// apiserver_lib.LiveRowsFilter to keep soft-deleted AOR rows out of
// the result. Without the explicit filter, the raw .Joins() form
// would surface stale AORs that don't represent the current state.
func TestEventsJoin_ExcludesSoftDeletedAOR(t *testing.T) {
	db := setupEventJoinTestDB(t)
	fullyQualifiedEventType := (&api.Event{}).GetFullyQualifiedType()

	// create two events, each with its own AOR linking back to a synthetic
	// KubernetesWorkloadInstance subject. Going through Create fires the Event
	// afterCreate hook which inserts the matching AOR.
	now := time.Now()
	e1 := &api.Event{
		Reason:              util.Ptr("R1"),
		Note:                util.Ptr("n1"),
		Type:                util.Ptr("Normal"),
		Count:               util.Ptr(uint(1)),
		EventTime:           &now,
		LastObservedTime:    &now,
		ReportingController: util.Ptr("test"),
		ObjectType:          util.Ptr("threeport.io/v0.KubernetesWorkloadInstance"),
		ObjectID:            util.Ptr(uint(1)),
	}
	e2 := &api.Event{
		Reason:              util.Ptr("R2"),
		Note:                util.Ptr("n2"),
		Type:                util.Ptr("Normal"),
		Count:               util.Ptr(uint(1)),
		EventTime:           &now,
		LastObservedTime:    &now,
		ReportingController: util.Ptr("test"),
		ObjectType:          util.Ptr("threeport.io/v0.KubernetesWorkloadInstance"),
		ObjectID:            util.Ptr(uint(2)),
	}
	require.NoError(t, db.Create(e1).Error)
	require.NoError(t, db.Create(e2).Error)

	// confirm baseline: the JOIN returns both events when nothing is
	// soft-deleted.
	runJoin := func() []api.Event {
		var rows []api.Event
		require.NoError(t, JoinEventsToAttachedObjectReferences(
			db.Model(&api.Event{}),
			fullyQualifiedEventType,
		).Find(&rows).Error)
		return rows
	}
	require.Len(t, runJoin(), 2, "baseline: both events visible with their live AORs")

	// soft-delete e1's AOR directly. e1's event row stays live, but its
	// AOR should drop out of the JOIN.
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", fullyQualifiedEventType, *e1.ID).
		Delete(&api.AttachedObjectReference{}).Error)

	rows := runJoin()
	require.Len(t, rows, 1, "soft-deleted AOR must be excluded from the JOIN")
	assert.Equal(t, *e2.ID, *rows[0].ID, "remaining event is e2 (whose AOR is still live)")
}

// TestEventsJoin_CrossTypeFilterSurfacesAllMatches pins the intended
// (Cartesian-product) behavior of applyObjectIdFilter. When a bare
// kind resolves to multiple FQTs (e.g. "Widget" registered in both
// core and a module, each with a record named "foo"), every matching
// (type, id) pair surfaces - the caller asked about "Widget/foo"
// without disambiguating the namespace, so all "Widget/foo"s come
// back. Callers narrow via objectnamespace / objectversion up front,
// not at this filter layer.
func TestEventsJoin_CrossTypeFilterSurfacesAllMatches(t *testing.T) {
	db := setupEventJoinTestDB(t)
	fullyQualifiedEventType := (&api.Event{}).GetFullyQualifiedType()
	now := time.Now()

	// build a small fixture: three events about widgets, spanning two
	// FQTs and including an id that exists under both. The filter
	// should surface every event whose subject is in the resolved
	// (type, id) cross-product.
	subjects := []struct {
		objectType string
		objectID   uint
	}{
		{"threeport.io/v0.Widget", 42},
		{"example.com/v0.Widget", 99},
		{"threeport.io/v0.Widget", 99}, // same id under a different type - intentionally included
	}
	for i, s := range subjects {
		require.NoError(t, db.Create(&api.Event{
			Reason: util.Ptr(fmt.Sprintf("R%d", i)),
			Note:   util.Ptr("n"), Type: util.Ptr("Normal"),
			Count: util.Ptr(uint(1)), EventTime: &now, LastObservedTime: &now,
			ReportingController: util.Ptr("test"),
			ObjectType:          util.Ptr(s.objectType),
			ObjectID:            util.Ptr(s.objectID),
		}).Error)
	}

	// Caller resolution yielded types from both Widget registrations
	// and ids from each (name lookup hit "foo" under each FQT).
	types := []string{"threeport.io/v0.Widget", "example.com/v0.Widget"}
	ids := []uint{42, 99}

	var rows []api.Event
	require.NoError(t, JoinEventsToAttachedObjectReferences(
		db.Model(&api.Event{}),
		fullyQualifiedEventType,
	).
		Where("v0_attached_object_references.object_type IN ?", types).
		Where("v0_attached_object_references.object_id IN ?", ids).
		Find(&rows).Error)

	assert.Len(t, rows, 3, "Cartesian filter surfaces every event whose subject is one of the resolved (type, id) pairs")
}
