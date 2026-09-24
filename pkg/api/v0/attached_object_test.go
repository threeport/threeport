package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAORTestDB returns an in-memory sqlite db with just the
// AttachedObjectReference table migrated. The partial-unique indexes
// declared on the type's GORM tags are translated to sqlite's
// CREATE UNIQUE INDEX ... WHERE syntax (sqlite 3.8+); AutoMigrate
// emits them at the same time as the table.
func setupAORTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AttachedObjectReference{}))
	return db
}

// newAOR returns an AttachedObjectReference with stable pointer-string
// values so tests can read like "object X attaches to base Y via rel".
func newAOR(baseType string, baseID uint, attacherType string, attacherID uint, rel Relationship) *AttachedObjectReference {
	bt, at, r := baseType, attacherType, rel
	bid, aid := baseID, attacherID
	return &AttachedObjectReference{
		ObjectType:         &bt,
		ObjectID:           &bid,
		AttachedObjectType: &at,
		AttachedObjectID:   &aid,
		Relationship:       &r,
	}
}

// TestAOR_OwnsSingleOwnerConstraint exercises idx_aor_owns_base: an
// owned base appears in at most one owns row. The attacher side is
// intentionally unconstrained (an owner can own many bases), so the
// table covers both the rejection and the legal cases.
func TestAOR_OwnsSingleOwnerConstraint(t *testing.T) {
	const (
		baseType   = "threeport.io/v0.Workload"
		baseID     = uint(42)
		baseTypeB  = "threeport.io/v0.Workload"
		baseIDB    = uint(43)
		ownerType1 = "threeport.io/v0.OwnerA"
		ownerType2 = "threeport.io/v0.OwnerB"
	)

	cases := []struct {
		name    string
		seed    *AttachedObjectReference
		insert  *AttachedObjectReference
		wantErr bool
	}{
		{
			name:   "single owner is allowed",
			seed:   newAOR(baseType, baseID, ownerType1, 1, RelationshipOwns),
			insert: nil,
		},
		{
			name:    "second owner on same base is rejected",
			seed:    newAOR(baseType, baseID, ownerType1, 1, RelationshipOwns),
			insert:  newAOR(baseType, baseID, ownerType2, 2, RelationshipOwns),
			wantErr: true,
		},
		{
			name:   "non-owns row on same base is allowed (partial index excludes it)",
			seed:   newAOR(baseType, baseID, ownerType1, 1, RelationshipOwns),
			insert: newAOR(baseType, baseID, ownerType2, 2, RelationshipDescribes),
		},
		{
			name:   "same owner owns a different base too",
			seed:   newAOR(baseType, baseID, ownerType1, 1, RelationshipOwns),
			insert: newAOR(baseTypeB, baseIDB, ownerType1, 1, RelationshipOwns),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAORTestDB(t)
			require.NoError(t, db.Create(tc.seed).Error)
			if tc.insert == nil {
				return
			}
			err := db.Create(tc.insert).Error
			if tc.wantErr {
				require.Error(t, err, "DB partial unique index should reject second owner")
				assert.Contains(t, err.Error(), "UNIQUE")
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestAOR_MarriesOneToOne exercises idx_aor_marries_base and
// idx_aor_marries_attached. Both sides of the marries relationship
// are constrained to appear in at most one marries row, enforcing
// 1-to-1 cardinality.
func TestAOR_MarriesOneToOne(t *testing.T) {
	const (
		baseType    = "threeport.io/v0.KubernetesWorkloadInstance"
		baseIDA     = uint(10)
		baseIDB     = uint(11)
		partnerType = "threeport.io/v0.GatewayInstance"
		partnerIDA  = uint(20)
		partnerIDB  = uint(21)
	)

	cases := []struct {
		name    string
		seed    *AttachedObjectReference
		insert  *AttachedObjectReference
		wantErr bool
	}{
		{
			name:   "single marriage is allowed",
			seed:   newAOR(baseType, baseIDA, partnerType, partnerIDA, RelationshipMarries),
			insert: nil,
		},
		{
			name:    "second partner on same base is rejected (idx_aor_marries_base)",
			seed:    newAOR(baseType, baseIDA, partnerType, partnerIDA, RelationshipMarries),
			insert:  newAOR(baseType, baseIDA, partnerType, partnerIDB, RelationshipMarries),
			wantErr: true,
		},
		{
			name:    "same partner married to a different base is rejected (idx_aor_marries_attached)",
			seed:    newAOR(baseType, baseIDA, partnerType, partnerIDA, RelationshipMarries),
			insert:  newAOR(baseType, baseIDB, partnerType, partnerIDA, RelationshipMarries),
			wantErr: true,
		},
		{
			name:   "non-marries row sharing base is allowed (partial index excludes it)",
			seed:   newAOR(baseType, baseIDA, partnerType, partnerIDA, RelationshipMarries),
			insert: newAOR(baseType, baseIDA, partnerType, partnerIDB, RelationshipDescribes),
		},
		{
			name:   "different base + different partner is allowed",
			seed:   newAOR(baseType, baseIDA, partnerType, partnerIDA, RelationshipMarries),
			insert: newAOR(baseType, baseIDB, partnerType, partnerIDB, RelationshipMarries),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAORTestDB(t)
			require.NoError(t, db.Create(tc.seed).Error)
			if tc.insert == nil {
				return
			}
			err := db.Create(tc.insert).Error
			if tc.wantErr {
				require.Error(t, err, "DB partial unique index should reject")
				assert.Contains(t, err.Error(), "UNIQUE")
				return
			}
			require.NoError(t, err)
		})
	}
}

// createValidAOR is the seed helper for hook tests; it inserts an
// AttachedObjectReference with stable values and returns the reloaded
// row, mirroring the PATCH handler's flow which loads existing before
// calling Updates.
func createValidAOR(t *testing.T, db *gorm.DB, rel Relationship) AttachedObjectReference {
	t.Helper()
	objType := "threeport.io/v0.TestBase"
	objID := uint(1)
	attachedType := "threeport.io/v0.TestAttacher"
	attachedID := uint(2)
	relCopy := rel
	existing := &AttachedObjectReference{
		ObjectType:         &objType,
		ObjectID:           &objID,
		AttachedObjectType: &attachedType,
		AttachedObjectID:   &attachedID,
		Relationship:       &relCopy,
	}
	require.NoError(t, db.Create(existing).Error)

	var loaded AttachedObjectReference
	require.NoError(t, db.First(&loaded, *existing.ID).Error)
	return loaded
}

// TestAttachedObjectReference_beforeUpdate_RejectsPatchRelationship
// pins the canonical PATCH-shape immutability check: changing
// Relationship via Updates() must be rejected because the relationship
// lifecycle (owns, marries, requires, describes) determines blocking
// behavior of existing references, and silently widening or narrowing
// it would change those guarantees post-create.
func TestAttachedObjectReference_beforeUpdate_RejectsPatchRelationship(t *testing.T) {
	db := setupAORTestDB(t)
	loaded := createValidAOR(t, db, RelationshipRequires)

	newRel := RelationshipOwns
	patch := &AttachedObjectReference{Relationship: &newRel}
	err := db.Model(&loaded).Updates(patch).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "AttachedObjectReference.Relationship is immutable")
}

// TestAttachedObjectReference_beforeUpdate_RejectsPutRelationship is
// the regression guard for the GORM Statement.Changed PUT-blind spot.
// Under Save() the receiver IS the inbound row, so Statement.Changed
// reports nothing changed even when the caller flips Relationship.
// The fix routes through IsFieldChanged, which loads the committed row
// and diffs it against the inbound values. This test fails against
// the pre-fix code and passes against the fixed code.
func TestAttachedObjectReference_beforeUpdate_RejectsPutRelationship(t *testing.T) {
	db := setupAORTestDB(t)
	loaded := createValidAOR(t, db, RelationshipRequires)

	full := loaded
	newRel := RelationshipOwns
	full.Relationship = &newRel
	err := db.Save(&full).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "AttachedObjectReference.Relationship is immutable")
}

// TestAttachedObjectReference_beforeUpdate_AllowsNoOpUpdate confirms a
// patch that doesn't touch Relationship passes through. This pins the
// "no spurious rejection" branch; without it a regression that
// always-returns-error on the field check would still pass the
// rejection tests above but break legitimate updates.
func TestAttachedObjectReference_beforeUpdate_AllowsNoOpUpdate(t *testing.T) {
	db := setupAORTestDB(t)
	loaded := createValidAOR(t, db, RelationshipRequires)

	// PATCH with no Relationship field present in the payload
	patch := &AttachedObjectReference{}
	err := db.Model(&loaded).Updates(patch).Error

	require.NoError(t, err, "no-op update should pass; Relationship absent from patch must not trigger rejection")
}

// TestAttachedObjectReference_beforeUpdate_AllowsSameRelationshipPut
// confirms a PUT that writes the SAME Relationship value back passes
// through. Without the IsFieldChanged fix, a naive "any nil-vs-non-nil
// diff" check could fire even when the caller is sending the same
// value, breaking idempotent writes.
func TestAttachedObjectReference_beforeUpdate_AllowsSameRelationshipPut(t *testing.T) {
	db := setupAORTestDB(t)
	loaded := createValidAOR(t, db, RelationshipRequires)

	full := loaded
	// reassign with the same value via a fresh pointer (simulates the
	// generated handler which binds the request body into a new struct)
	sameRel := RelationshipRequires
	full.Relationship = &sameRel
	err := db.Save(&full).Error

	require.NoError(t, err, "PUT writing the same Relationship back should pass; same value must not be flagged as changed")
}

// TestAOR_OwnsConstraintAfterSoftDelete verifies the deleted_at IS
// NULL clause in idx_aor_owns_base lets a soft-deleted owns row drop
// out of the unique slot so the base can be re-owned after the
// original owner is torn down. Without the clause, the soft-deleted
// row would continue to occupy the slot until cockroach TTL eventually
// hard-deleted it.
func TestAOR_OwnsConstraintAfterSoftDelete(t *testing.T) {
	db := setupAORTestDB(t)

	r1 := newAOR("threeport.io/v0.Workload", 1, "threeport.io/v0.OwnerA", 1, RelationshipOwns)
	require.NoError(t, db.Create(r1).Error)

	// soft-delete (BeforeDelete cleanup of outgoing AORs uses the same
	// call shape; AOR rows are soft-deleted via Common.DeletedAt)
	require.NoError(t, db.Delete(r1).Error)

	// new owner for the same base must be allowed now that the prior
	// owns row is soft-deleted out of the partial unique slot
	r2 := newAOR("threeport.io/v0.Workload", 1, "threeport.io/v0.OwnerB", 2, RelationshipOwns)
	require.NoError(t, db.Create(r2).Error,
		"re-owning a base whose prior owns row is soft-deleted must be allowed")
}

// TestAOR_MarriesConstraintAfterSoftDelete mirrors the owns test for
// the marries 1-to-1 indexes. Both sides (base and attacher) get
// their own partial unique index; both must let soft-deleted rows
// fall out so the base or partner can be re-married after teardown.
func TestAOR_MarriesConstraintAfterSoftDelete(t *testing.T) {
	cases := []struct {
		name   string
		setup  func() (initial, replacement *AttachedObjectReference)
		reason string
	}{
		{
			name: "base side - same base, different partner",
			setup: func() (*AttachedObjectReference, *AttachedObjectReference) {
				return newAOR("threeport.io/v0.KubernetesWorkloadInstance", 1, "threeport.io/v0.GatewayInstance", 10, RelationshipMarries),
					newAOR("threeport.io/v0.KubernetesWorkloadInstance", 1, "threeport.io/v0.GatewayInstance", 11, RelationshipMarries)
			},
			reason: "idx_aor_marries_base must let soft-deleted rows out of the slot",
		},
		{
			name: "attacher side - same partner, different base",
			setup: func() (*AttachedObjectReference, *AttachedObjectReference) {
				return newAOR("threeport.io/v0.KubernetesWorkloadInstance", 1, "threeport.io/v0.GatewayInstance", 10, RelationshipMarries),
					newAOR("threeport.io/v0.KubernetesWorkloadInstance", 2, "threeport.io/v0.GatewayInstance", 10, RelationshipMarries)
			},
			reason: "idx_aor_marries_attached must let soft-deleted rows out of the slot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAORTestDB(t)
			initial, replacement := tc.setup()
			require.NoError(t, db.Create(initial).Error)
			require.NoError(t, db.Delete(initial).Error)
			require.NoError(t, db.Create(replacement).Error, tc.reason)
		})
	}
}
