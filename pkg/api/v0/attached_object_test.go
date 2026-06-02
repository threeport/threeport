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

// TestAOR_RelationshipImmutable verifies the beforeUpdate hook in
// attached_object_validate.go: silently widening describes -> owns
// would change blocking behavior of an existing reference, so the
// Relationship column is locked once persisted. Unchanged updates
// (Changed returns false) must pass through.
func TestAOR_RelationshipImmutable(t *testing.T) {
	const (
		baseType     = "threeport.io/v0.Workload"
		baseID       = uint(7)
		attacherType = "threeport.io/v0.Pod"
		attacherID   = uint(99)
	)

	cases := []struct {
		name      string
		updateRel *Relationship // nil = no change to Relationship
		wantErr   bool
	}{
		{
			name:      "rel change describes -> owns rejected",
			updateRel: rPtr(RelationshipOwns),
			wantErr:   true,
		},
		{
			name:      "rel change describes -> requires rejected",
			updateRel: rPtr(RelationshipRequires),
			wantErr:   true,
		},
		{
			name:      "unchanged update (no rel field in payload) allowed",
			updateRel: nil,
		},
		{
			name:      "rel same value resolves Changed=false and is allowed",
			updateRel: rPtr(RelationshipDescribes),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAORTestDB(t)
			existing := newAOR(baseType, baseID, attacherType, attacherID, RelationshipDescribes)
			require.NoError(t, db.Create(existing).Error)

			var loaded AttachedObjectReference
			require.NoError(t, db.First(&loaded, *existing.ID).Error)

			inbound := &AttachedObjectReference{}
			if tc.updateRel != nil {
				inbound.Relationship = tc.updateRel
			}

			err := db.Model(&loaded).Updates(inbound).Error
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "immutable")
				return
			}
			require.NoError(t, err)
		})
	}
}

func rPtr(r Relationship) *Relationship { return &r }

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
