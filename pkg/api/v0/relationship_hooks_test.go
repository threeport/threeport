package v0

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// testHolder exercises every relationship-tagged FK shape that
// production types use: one of each Relationship value plus an
// untagged FK as the negative control. The FKs all point at other
// testHolder rows (self-referential) so the test only has to migrate
// one table beyond AttachedObjectReference.
type testHolder struct {
	ID                *uint `gorm:"primaryKey"`
	Name              *string
	DeletedAt         gorm.DeletedAt
	DeletionScheduled *time.Time
	RequiresFK        *uint `relationship:"requires"`
	OwnsFK            *uint `relationship:"owns"`
	MarriesFK         *uint `relationship:"marries"`
	DescribesFK       *uint `relationship:"describes"`
	UntaggedFK        *uint
	owner             *OwnerRef `gorm:"-"`
}

const (
	testHolderFQT       = "test.local/v0.TestHolder"
	testRequiresTgtFQT  = "test.local/v0.RequiresTarget"
	testOwnsTgtFQT      = "test.local/v0.OwnsTarget"
	testMarriesTgtFQT   = "test.local/v0.MarriesTarget"
	testDescribesTgtFQT = "test.local/v0.DescribesTarget"
)

func (h *testHolder) GetFullyQualifiedType() string { return testHolderFQT }

func (h *testHolder) GetOwner() *OwnerRef { return h.owner }

// RelationshipTaggedForeignKeys gives each FK its own synthetic
// ObjectType. Production types do this naturally (different FKs point
// at different real types); reusing one type would make AfterUpdate's
// per-(attacher, target-type) sync collide across FKs.
func (h *testHolder) RelationshipTaggedForeignKeys() []RelationshipTaggedForeignKey {
	return []RelationshipTaggedForeignKey{
		{FieldName: "RequiresFK", ObjectID: h.RequiresFK, ObjectType: testRequiresTgtFQT, Relationship: RelationshipRequires},
		{FieldName: "OwnsFK", ObjectID: h.OwnsFK, ObjectType: testOwnsTgtFQT, Relationship: RelationshipOwns},
		{FieldName: "MarriesFK", ObjectID: h.MarriesFK, ObjectType: testMarriesTgtFQT, Relationship: RelationshipMarries},
		{FieldName: "DescribesFK", ObjectID: h.DescribesFK, ObjectType: testDescribesTgtFQT, Relationship: RelationshipDescribes},
	}
}

func (h *testHolder) BeforeUpdate(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsBeforeUpdate(tx, h)
}

func (h *testHolder) AfterCreate(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsAfterCreate(tx, h)
}

func (h *testHolder) AfterUpdate(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsAfterUpdate(tx, h)
}

func (h *testHolder) BeforeDelete(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsBeforeDelete(tx, h)
}

// setupRelTestDB returns an in-memory sqlite db with the test holder
// table and AttachedObjectReference migrated.
func setupRelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testHolder{}, &AttachedObjectReference{}))
	return db
}

func uintPtr(v uint) *uint    { return &v }
func strPtr(s string) *string { return &s }

// withControlPlaneCaller returns a DB session whose context carries a
// CallerIdentity with OU=controlplane. The BeforeUpdate and
// BeforeDelete hooks use this to bypass the owned/married guard.
func withControlPlaneCaller(db *gorm.DB) *gorm.DB {
	ctx := lib.WithCaller(context.Background(), lib.CallerIdentity{OrganizationalUnit: auth.OUControlPlane})
	return db.WithContext(ctx)
}

// seedHolder creates a row with the given FKs already set. Returns the
// persisted row so the caller can use *holder.ID in subsequent
// references.
func seedHolder(t *testing.T, db *gorm.DB, name string, fks ...func(*testHolder)) *testHolder {
	t.Helper()
	h := &testHolder{Name: strPtr(name)}
	for _, apply := range fks {
		apply(h)
	}
	require.NoError(t, db.Create(h).Error)
	require.NotNil(t, h.ID, "Create should populate ID")
	return h
}

// deleteHolderByID loads the row first and then deletes it. gorm's
// Delete(&Model{}, id) shorthand emits a DELETE without loading the
// row, so BeforeDelete fires with the receiver's ID still nil and the
// blocking check returns early. Loading then deleting matches the
// handler pattern in production (First → Delete on a populated row).
func deleteHolderByID(t *testing.T, db *gorm.DB, id uint) error {
	t.Helper()
	var loaded testHolder
	if err := db.First(&loaded, id).Error; err != nil {
		return err
	}
	return db.Delete(&loaded).Error
}

// countAORs returns the number of AttachedObjectReference rows whose
// attacher is the given holder. Used to assert AfterCreate/AfterUpdate
// sync behavior.
func countAORs(t *testing.T, db *gorm.DB, attacherID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.
		Model(&AttachedObjectReference{}).
		Where("attached_object_type = ? AND attached_object_id = ?", testHolderFQT, attacherID).
		Count(&n).Error)
	return n
}

// TestRelationshipHooks_AfterCreate_NilFKs verifies that creating a
// row with no FKs set produces no AOR rows.
func TestRelationshipHooks_AfterCreate_NilFKs(t *testing.T) {
	db := setupRelTestDB(t)

	h := seedHolder(t, db, "no-fks")

	assert.Equal(t, int64(0), countAORs(t, db, *h.ID))
}

// TestRelationshipHooks_AfterCreate_OneAORPerSetFK creates AORs only
// for the FKs that are non-nil, one per set FK, with the correct
// relationship tag on each.
func TestRelationshipHooks_AfterCreate_OneAORPerSetFK(t *testing.T) {
	db := setupRelTestDB(t)

	target := seedHolder(t, db, "target")
	h := seedHolder(t, db, "holder",
		func(h *testHolder) { h.RequiresFK = target.ID },
		func(h *testHolder) { h.OwnsFK = target.ID },
		// MarriesFK and DescribesFK left nil
	)

	var refs []AttachedObjectReference
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", testHolderFQT, *h.ID).
		Order("relationship").
		Find(&refs).Error)

	require.Len(t, refs, 2, "two FKs were set, expect two AORs")
	for _, ref := range refs {
		require.NotNil(t, ref.Relationship)
	}
	assert.Equal(t, RelationshipOwns, *refs[0].Relationship)
	assert.Equal(t, RelationshipRequires, *refs[1].Relationship)
}

// TestRelationshipHooks_AfterCreate_AllFourRelationships verifies one
// AOR per relationship tag, each carrying its own value.
func TestRelationshipHooks_AfterCreate_AllFourRelationships(t *testing.T) {
	db := setupRelTestDB(t)

	target := seedHolder(t, db, "target")
	h := seedHolder(t, db, "holder",
		func(h *testHolder) { h.RequiresFK = target.ID },
		func(h *testHolder) { h.OwnsFK = target.ID },
		func(h *testHolder) { h.MarriesFK = target.ID },
		func(h *testHolder) { h.DescribesFK = target.ID },
	)

	relationships := map[Relationship]bool{}
	var refs []AttachedObjectReference
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", testHolderFQT, *h.ID).
		Find(&refs).Error)
	require.Len(t, refs, 4)
	for _, ref := range refs {
		require.NotNil(t, ref.Relationship)
		relationships[*ref.Relationship] = true
	}
	assert.True(t, relationships[RelationshipRequires])
	assert.True(t, relationships[RelationshipOwns])
	assert.True(t, relationships[RelationshipMarries])
	assert.True(t, relationships[RelationshipDescribes])
}

// TestRelationshipHooks_BeforeUpdate_FKImmutability walks every
// relationship FK through gorm-reachable transitions under a PATCH
// (Model+Updates). The guard uses tx.Statement.Changed to detect
// mutations against the loaded row.
//
// Tagged FKs: nil->set allowed, set->other rejected, set->nil
// silently dropped by gorm so the FK stays at its pre value. The
// untagged FK is a negative control, freely mutable.
func TestRelationshipHooks_BeforeUpdate_FKImmutability(t *testing.T) {
	type transition struct {
		name          string
		preFK         *uint
		inbound       func(target1ID, target2ID uint) *testHolder
		wantErrSubstr string // empty = expect success
	}

	// The matrix is the cartesian product of (tagged FK field) × (transition).
	// Spelled out per-field rather than via reflection so the test reads as
	// a literal table.

	t.Run("RequiresFK", func(t *testing.T) {
		// run the shared transition matrix against the RequiresFK field
		runFKMatrix(t, "RequiresFK",
			// setter: write v into RequiresFK; lets runFKMatrix populate the field generically
			func(h *testHolder, v *uint) { h.RequiresFK = v },
			// getter: read RequiresFK back; lets runFKMatrix verify post-update state generically
			func(h *testHolder) *uint { return h.RequiresFK },
			// immutable=true: relationship-tagged FK, so set->other transitions must be rejected
			true,
		)
	})
	t.Run("OwnsFK", func(t *testing.T) {
		runFKMatrix(t, "OwnsFK",
			func(h *testHolder, v *uint) { h.OwnsFK = v },
			func(h *testHolder) *uint { return h.OwnsFK },
			true,
		)
	})
	t.Run("MarriesFK", func(t *testing.T) {
		runFKMatrix(t, "MarriesFK",
			func(h *testHolder, v *uint) { h.MarriesFK = v },
			func(h *testHolder) *uint { return h.MarriesFK },
			true,
		)
	})
	t.Run("DescribesFK", func(t *testing.T) {
		runFKMatrix(t, "DescribesFK",
			func(h *testHolder, v *uint) { h.DescribesFK = v },
			func(h *testHolder) *uint { return h.DescribesFK },
			true,
		)
	})
	t.Run("UntaggedFK_freelyMutable", func(t *testing.T) {
		runFKMatrix(t, "UntaggedFK",
			func(h *testHolder, v *uint) { h.UntaggedFK = v },
			func(h *testHolder) *uint { return h.UntaggedFK },
			false,
		)
	})
	_ = transition{}
}

// TestRelationshipHooks_BeforeUpdate_FKImmutability_FullReplace is
// the PUT counterpart to ...FKImmutability: same FK-transition
// matrix, but the update happens via gorm.Save instead of Updates.
// Under Save, Model and Dest are the same object so the guard can't
// rely on tx.Statement.Changed; it compares inbound values against
// the committed row directly.
//
// Tagged FKs: nil->set allowed, set->other and set->nil rejected,
// set->same allowed. The untagged FK is a negative control, freely
// mutable.
func TestRelationshipHooks_BeforeUpdate_FKImmutability_FullReplace(t *testing.T) {
	cases := []struct {
		name         string
		preSet       bool   // RequiresFK set before the replace
		newTarget    string // "", "same", or "other": the relationship FK in the replace
		untaggedOnly bool   // change UntaggedFK set->other, leave RequiresFK nil
		wantRejected bool
	}{
		{name: "tagged_clear_to_set", preSet: false, newTarget: "other", wantRejected: false},
		{name: "tagged_set_to_other", preSet: true, newTarget: "other", wantRejected: true},
		{name: "tagged_set_to_clear", preSet: true, newTarget: "", wantRejected: true},
		{name: "tagged_set_to_same", preSet: true, newTarget: "same", wantRejected: false},
		{name: "untagged_set_to_other", untaggedOnly: true, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRelTestDB(t)
			t1 := seedHolder(t, db, "target-1")
			t2 := seedHolder(t, db, "target-2")

			existing := &testHolder{Name: strPtr("orig")}
			if tc.preSet {
				existing.RequiresFK = t1.ID
			}
			if tc.untaggedOnly {
				existing.UntaggedFK = t1.ID
			}
			require.NoError(t, db.Create(existing).Error)

			// a full replace carries the existing id and every field; a nil
			// RequiresFK on this payload is an explicit clear, not an omission
			replacement := &testHolder{ID: existing.ID, Name: strPtr("replaced")}
			switch tc.newTarget {
			case "same":
				replacement.RequiresFK = t1.ID
			case "other":
				replacement.RequiresFK = t2.ID
			}
			if tc.untaggedOnly {
				replacement.UntaggedFK = t2.ID
			}

			err := db.Save(replacement).Error

			if tc.wantRejected {
				require.Error(t, err, "full-replace %s should be rejected", tc.name)
				assert.Contains(t, err.Error(), "immutable once set")
				return
			}
			require.NoError(t, err, "full-replace %s should be allowed", tc.name)
		})
	}
}

// runFKMatrix exercises gorm.Updates(struct) transitions for one FK:
//   - nil -> set:    allowed
//   - set -> other:  rejected on tagged FKs
//   - set -> nil:    gorm drops the nil; FK preserved at pre value
//   - untouched FKs: allowed regardless of state
//
// For an untagged FK (immutable=false), set -> other is allowed too.
func runFKMatrix(
	t *testing.T,
	fkName string,
	setFK func(*testHolder, *uint),
	getFK func(*testHolder) *uint,
	immutable bool,
) {
	cases := []struct {
		name            string
		preFK           *uint
		inboundFK       *uint
		changeName      bool
		wantRejected    bool
		wantPreservedFK bool
	}{
		{name: "nil_to_set", preFK: nil, inboundFK: uintPtr(99), wantRejected: false},
		{name: "set_to_other", preFK: uintPtr(99), inboundFK: uintPtr(100), wantRejected: immutable},
		{name: "set_to_nil_gorm_drops_silently", preFK: uintPtr(99), inboundFK: nil, changeName: true, wantRejected: false, wantPreservedFK: true},
		{name: "untouched_with_name_change", preFK: uintPtr(99), inboundFK: uintPtr(99), changeName: true, wantRejected: false},
		{name: "nil_untouched_with_name_change", preFK: nil, inboundFK: nil, changeName: true, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRelTestDB(t)
			// targets the FKs point at must exist - the AfterCreate
			// hook inserts AOR rows referencing them by id, but the
			// AOR table doesn't enforce a FK, so we just need ids that
			// look real. Two distinct values are needed for set_to_other.
			t1 := seedHolder(t, db, "target-1")
			t2 := seedHolder(t, db, "target-2")
			_ = t2 // only used through the inbound

			// rebase the inbound values onto real target ids
			pre := tc.preFK
			if pre != nil {
				pre = t1.ID
			}
			in := tc.inboundFK
			switch {
			case tc.name == "set_to_other":
				in = t2.ID
			case in != nil:
				in = t1.ID
			}

			existing := &testHolder{Name: strPtr("orig")}
			setFK(existing, pre)
			require.NoError(t, db.Create(existing).Error)

			var loaded testHolder
			require.NoError(t, db.First(&loaded, *existing.ID).Error)

			// build the inbound payload; for the untouched cases
			// flip Name so the UPDATE has at least one column to set
			inbound := &testHolder{}
			setFK(inbound, in)
			if tc.changeName {
				inbound.Name = strPtr("renamed")
			}

			err := db.Model(&loaded).Updates(inbound).Error

			if tc.wantRejected {
				require.Error(t, err, "FK %s transition %s should be rejected", fkName, tc.name)
				assert.Contains(t, err.Error(), "immutable once set", "rejection should cite immutability")
				return
			}
			require.NoError(t, err, "FK %s transition %s should be allowed", fkName, tc.name)

			// confirm gorm silently dropped the nil from the SET clause
			// by reloading and checking the FK is unchanged
			if tc.wantPreservedFK {
				var post testHolder
				require.NoError(t, db.First(&post, *existing.ID).Error)
				postFK := getFK(&post)
				require.NotNil(t, postFK, "gorm should have dropped nil from SET, leaving FK unchanged")
				assert.Equal(t, *pre, *postFK, "FK should remain at pre-update value")
			}
		})
	}
}

// plantIncomingAOR adds a single AOR row representing "other ->
// target via rel". Used by ownership tests to fabricate incoming
// references without standing up a second attacher type.
func plantIncomingAOR(t *testing.T, db *gorm.DB, target, other *testHolder, rel Relationship) {
	t.Helper()
	r := rel
	require.NoError(t, db.Create(&AttachedObjectReference{
		ObjectType:         strPtr(testHolderFQT),
		ObjectID:           target.ID,
		AttachedObjectType: strPtr(testHolderFQT),
		AttachedObjectID:   other.ID,
		Relationship:       &r,
	}).Error)
}

// TestRelationshipHooks_BeforeUpdate_IncomingRelationshipGuards walks
// every (incoming relationship, caller OU) combination. Production
// rule: owns/marries block external updates; requires/describes never
// block. Control-plane callers bypass all guards. The owned/married
// rejection message cites the owning object so external callers can
// trace lifecycle.
func TestRelationshipHooks_BeforeUpdate_IncomingRelationshipGuards(t *testing.T) {
	cases := []struct {
		name              string
		incoming          Relationship
		controlPlane      bool
		wantRejected      bool
		wantErrSubstrings []string
	}{
		{
			name:              "owns blocks external",
			incoming:          RelationshipOwns,
			wantRejected:      true,
			wantErrSubstrings: []string{"owned by", "tear down the owner first"},
		},
		{
			name:         "owns allows control-plane",
			incoming:     RelationshipOwns,
			controlPlane: true,
		},
		{
			name:              "marries blocks external",
			incoming:          RelationshipMarries,
			wantRejected:      true,
			wantErrSubstrings: []string{"owned by"},
		},
		{
			name:         "marries allows control-plane",
			incoming:     RelationshipMarries,
			controlPlane: true,
		},
		{
			name:     "requires does not block external updates",
			incoming: RelationshipRequires,
		},
		{
			name:     "describes does not block external updates",
			incoming: RelationshipDescribes,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRelTestDB(t)
			target := seedHolder(t, db, "target")
			other := seedHolder(t, db, "other")
			plantIncomingAOR(t, db, target, other, tc.incoming)

			var loaded testHolder
			require.NoError(t, db.First(&loaded, *target.ID).Error)

			useDB := db
			if tc.controlPlane {
				useDB = withControlPlaneCaller(db)
			}
			err := useDB.Model(&loaded).Updates(&testHolder{Name: strPtr("renamed")}).Error

			if tc.wantRejected {
				require.Error(t, err)
				for _, sub := range tc.wantErrSubstrings {
					assert.Contains(t, err.Error(), sub)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestRelationshipHooks_AfterUpdate_InsertsAORForNewlySetFK exercises
// the sync logic: an initial nil->set transition (allowed by
// BeforeUpdate) must add an AOR row for the FK.
func TestRelationshipHooks_AfterUpdate_InsertsAORForNewlySetFK(t *testing.T) {
	db := setupRelTestDB(t)
	target := seedHolder(t, db, "target")
	holder := seedHolder(t, db, "holder")
	require.Equal(t, int64(0), countAORs(t, db, *holder.ID))

	var loaded testHolder
	require.NoError(t, db.First(&loaded, *holder.ID).Error)
	require.NoError(t, db.Model(&loaded).Updates(&testHolder{RequiresFK: target.ID}).Error)

	assert.Equal(t, int64(1), countAORs(t, db, *holder.ID),
		"AfterUpdate should have inserted an AOR for the newly-set FK")
}

// TestRelationshipHooks_AfterUpdate_IdempotentWhenAORExists confirms
// no duplicate AOR is created if an update touches a field unrelated
// to FKs (sync should detect AOR already covers the FK state).
func TestRelationshipHooks_AfterUpdate_IdempotentWhenAORExists(t *testing.T) {
	db := setupRelTestDB(t)
	target := seedHolder(t, db, "target")
	holder := seedHolder(t, db, "holder", func(h *testHolder) { h.RequiresFK = target.ID })
	require.Equal(t, int64(1), countAORs(t, db, *holder.ID), "create should plant one AOR")

	var loaded testHolder
	require.NoError(t, db.First(&loaded, *holder.ID).Error)
	require.NoError(t, db.Model(&loaded).Updates(&testHolder{Name: strPtr("renamed")}).Error)

	assert.Equal(t, int64(1), countAORs(t, db, *holder.ID),
		"AfterUpdate should not insert a duplicate AOR when state is already in sync")
}

// TestRelationshipHooks_BeforeDelete_BlockingMatrix covers each
// incoming-relationship × caller-OU combination. Production rules:
//   - requires always blocks
//   - owns / marries block external, allow control-plane
//   - describes never blocks
//
// The "no incoming" case is broken out separately because it also
// asserts outgoing-AOR cleanup, which the blocking matrix doesn't.
func TestRelationshipHooks_BeforeDelete_BlockingMatrix(t *testing.T) {
	cases := []struct {
		name         string
		incoming     Relationship
		controlPlane bool
		wantBlocked  bool
	}{
		{name: "requires blocks external", incoming: RelationshipRequires, wantBlocked: true},
		{name: "requires blocks control-plane", incoming: RelationshipRequires, controlPlane: true, wantBlocked: true},
		{name: "owns blocks external", incoming: RelationshipOwns, wantBlocked: true},
		{name: "owns allows control-plane", incoming: RelationshipOwns, controlPlane: true},
		{name: "marries blocks external", incoming: RelationshipMarries, wantBlocked: true},
		{name: "marries allows control-plane", incoming: RelationshipMarries, controlPlane: true},
		{name: "describes never blocks external", incoming: RelationshipDescribes},
		{name: "describes never blocks control-plane", incoming: RelationshipDescribes, controlPlane: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRelTestDB(t)
			target := seedHolder(t, db, "target")
			other := seedHolder(t, db, "other")
			plantIncomingAOR(t, db, target, other, tc.incoming)

			useDB := db
			if tc.controlPlane {
				useDB = withControlPlaneCaller(db)
			}
			err := deleteHolderByID(t, useDB, *target.ID)

			if tc.wantBlocked {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot be deleted")
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestRelationshipHooks_BeforeDelete_CleansUpOutgoingAORs verifies
// the outgoing-reference cleanup path: when nothing blocks delete,
// the row's own AORs (where it's the attacher) are removed in the
// same transaction so they don't dangle.
func TestRelationshipHooks_BeforeDelete_CleansUpOutgoingAORs(t *testing.T) {
	db := setupRelTestDB(t)
	target := seedHolder(t, db, "target")
	holder := seedHolder(t, db, "holder", func(h *testHolder) { h.RequiresFK = target.ID })
	require.Equal(t, int64(1), countAORs(t, db, *holder.ID))

	require.NoError(t, deleteHolderByID(t, db, *holder.ID))
	assert.Equal(t, int64(0), countAORs(t, db, *holder.ID),
		"BeforeDelete should clean up outgoing AOR rows in the same transaction")
}

// testCollider is a probe type for the AfterUpdate AOR-sync
// edge case where two relationship-tagged FKs point at the SAME
// target type. Production has this shape on
// ObservabilityStackDefinition (Grafana / KubePrometheus / Loki /
// Promtail all "owns" HelmWorkloadDefinition). The hook's count
// query filters by (object_type, attached_object_type,
// attached_object_id) - it does NOT filter by object_id - so all FKs
// that share an ObjectType collapse onto the same count.
type testCollider struct {
	ID        *uint `gorm:"primaryKey"`
	Name      *string
	GrafanaID *uint `relationship:"owns"`
	LokiID    *uint `relationship:"owns"`
}

const testColliderFQT = "test.local/v0.TestCollider"
const testSharedTargetFQT = "test.local/v0.HelmWorkloadDefinition"

func (c *testCollider) GetFullyQualifiedType() string { return testColliderFQT }

func (c *testCollider) RelationshipTaggedForeignKeys() []RelationshipTaggedForeignKey {
	return []RelationshipTaggedForeignKey{
		{FieldName: "GrafanaID", ObjectID: c.GrafanaID, ObjectType: testSharedTargetFQT, Relationship: RelationshipOwns},
		{FieldName: "LokiID", ObjectID: c.LokiID, ObjectType: testSharedTargetFQT, Relationship: RelationshipOwns},
	}
}

func (c *testCollider) BeforeUpdate(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsBeforeUpdate(tx, c)
}
func (c *testCollider) AfterCreate(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsAfterCreate(tx, c)
}
func (c *testCollider) AfterUpdate(tx *gorm.DB) error {
	return processRelationshipTaggedFieldsAfterUpdate(tx, c)
}

// TestRelationshipHooks_AfterUpdate_SharedTargetTypeCollision verifies
// that two relationship-tagged foreign keys on the same struct that
// share a target type each get their own reference row. The
// after-update sync's count must filter by object_id so it doesn't
// conflate the foreign keys.
func TestRelationshipHooks_AfterUpdate_SharedTargetTypeCollision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testCollider{}, &AttachedObjectReference{}))

	// initial create with only Grafana set; AfterCreate should insert 1 AOR
	obj := &testCollider{Name: strPtr("stack"), GrafanaID: uintPtr(1)}
	require.NoError(t, db.Create(obj).Error)

	var aors []AttachedObjectReference
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", testColliderFQT, *obj.ID).
		Find(&aors).Error)
	require.Len(t, aors, 1, "AfterCreate inserts one AOR for the single set FK")

	// now set Loki (nil -> value, allowed by BeforeUpdate). The sync
	// SHOULD insert a second AOR pointing at Loki's HWD id, but the
	// count query conflates the two FKs under (object_type=HWD,
	// attached_object_type=Collider, attached_object_id=stack), sees
	// the Grafana row, and skips the Loki insert.
	var loaded testCollider
	require.NoError(t, db.First(&loaded, *obj.ID).Error)
	require.NoError(t, db.Model(&loaded).Updates(&testCollider{LokiID: uintPtr(2)}).Error)

	aors = nil
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", testColliderFQT, *obj.ID).
		Find(&aors).Error)
	require.Len(t, aors, 2, "each set FK should have its own AOR row; shared ObjectType must not conflate the sync count")

	// confirm each AOR points at its own FK target id
	gotObjectIDs := []uint{*aors[0].ObjectID, *aors[1].ObjectID}
	assert.ElementsMatch(t, []uint{1, 2}, gotObjectIDs,
		"AORs must carry the per-FK object_id, not be merged into one")
}

// TestRelationshipHooks_AfterCreate_MultipleSharedTargetFKs covers the
// initial-create path on the same shared-target-type shape. AfterCreate
// already iterates per FK without a shared count so it always behaved
// correctly here; the test pins that to prevent regressions.
func TestRelationshipHooks_AfterCreate_MultipleSharedTargetFKs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testCollider{}, &AttachedObjectReference{}))

	obj := &testCollider{Name: strPtr("stack"), GrafanaID: uintPtr(1), LokiID: uintPtr(2)}
	require.NoError(t, db.Create(obj).Error)

	var aors []AttachedObjectReference
	require.NoError(t, db.
		Where("attached_object_type = ? AND attached_object_id = ?", testColliderFQT, *obj.ID).
		Find(&aors).Error)
	require.Len(t, aors, 2)
	gotObjectIDs := []uint{*aors[0].ObjectID, *aors[1].ObjectID}
	assert.ElementsMatch(t, []uint{1, 2}, gotObjectIDs)
}
