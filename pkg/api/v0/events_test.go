package v0

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

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

// TestEventBeforeCreate_SubjectValidation covers subject field validation on create.
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
			db := dryRunDB(t)
			e := baseEvent()
			e.ObjectType = tc.objectType
			e.ObjectID = tc.objectID

			// create with the case's subject fields
			err := db.Create(e).Error
			if tc.wantErr {
				// reject a missing or malformed subject
				require.Error(t, err)
				for _, sub := range tc.wantErrSubs {
					assert.Contains(t, err.Error(), sub)
				}
				return
			}
			// accept a well-formed fully qualified type
			require.NoError(t, err)
		})
	}
}

// TestEventAfterCreate_WritesOnlyTheEventRow covers that create writes the event row alone.
func TestEventAfterCreate_WritesOnlyTheEventRow(t *testing.T) {
	db := dryRunDB(t)

	// capture tables written during create
	var tables []string
	require.NoError(t, db.Callback().Create().After("gorm:create").Register(
		"test:capture_created_tables",
		func(tx *gorm.DB) { tables = append(tables, tx.Statement.Table) },
	))

	e := baseEvent()
	e.ObjectType = util.Ptr("threeport.io/v0.KubernetesWorkloadInstance")
	e.ObjectID = util.Ptr(uint(42))

	// create a well-formed event
	require.NoError(t, db.Create(e).Error)

	// only the event table is written
	assert.Equal(t, []string{"v0_events"}, tables,
		"an event create writes the event row alone")
}

// nowPtr returns a *time.Time pointing at the current time. Inline
// helper to keep baseEvent() readable.
func nowPtr() *time.Time {
	t := time.Now()
	return &t
}
