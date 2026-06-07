package v0

// AttachedObjectReferenceRelationshipField is the Go struct field name
// of AttachedObjectReference.Relationship. Used by hooks that need to
// detect changes to the field via tx.Statement.Changed().
const AttachedObjectReferenceRelationshipField = "Relationship"

// AttachedObjectReference is a reference to an attached object.
//
// Four DB indexes are declared in the GORM tags below:
//
//   - idx_attached_object_unique: full-table unique composite across
//     (object_type, object_id, attached_object_type, attached_object_id).
//     Enforces that a given (base, attacher) pair appears in at most one
//     row regardless of relationship kind.
//
//   - idx_aor_marries_base: partial unique composite across
//     (object_type, object_id) where relationship = 'marries' AND
//     deleted_at IS NULL. Enforces that the base side of a marriage
//     appears in at most one *live* marries row (1-to-1 cardinality for
//     the base). The deleted_at predicate keeps soft-deleted rows out
//     of the unique slot so a base can be re-married after teardown.
//
//   - idx_aor_marries_attached: partial unique composite across
//     (attached_object_type, attached_object_id) where relationship =
//     'marries' AND deleted_at IS NULL. Same constraint applied to the
//     attacher side.
//
//   - idx_aor_owns_base: partial unique composite across
//     (object_type, object_id) where relationship = 'owns' AND
//     deleted_at IS NULL. Enforces that an owned base appears in at
//     most one *live* owns row. The attacher side is intentionally
//     unconstrained for owns: an owner may own many bases.
//
// Each participating column repeats the index name in its `uniqueIndex:`
// tag; GORM bundles them by name. The `,where:...` suffix on the
// partial indexes makes them partial indexes: only rows matching the
// predicate are indexed. The "AND deleted_at IS NULL" half of each
// predicate matters because attached object references use gorm
// soft-delete (see Common) - without it, soft-deleted rows would
// continue to occupy the unique slot and block re-attachment until
// cockroach eventually hard-deletes them.
type AttachedObjectReference struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// ObjectType is the kind of the base object being attached to. Read
	// "this attached object attaches to that object": the no-prefix
	// Object* fields name the "that" (anchor) side. The naming is
	// directional because the attached object is the side that can
	// determine the base object's lifecycle, depending on the type of
	// relationship (see below). Stored as a fully qualified type name
	// in the form "<api-namespace>/<version>.<TypeName>".
	ObjectType *string `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique;uniqueIndex:idx_aor_marries_base,where:relationship = 'marries' AND deleted_at IS NULL;uniqueIndex:idx_aor_owns_base,where:relationship = 'owns' AND deleted_at IS NULL"`

	// ObjectID is the database ID of the base object.
	ObjectID *uint `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique;uniqueIndex:idx_aor_marries_base,where:relationship = 'marries' AND deleted_at IS NULL;uniqueIndex:idx_aor_owns_base,where:relationship = 'owns' AND deleted_at IS NULL"`

	// AttachedObjectType is the kind of the object doing the attaching;
	// the side that can determine the base object's lifecycle. Stored
	// as a fully qualified type name in the form
	// "<api-namespace>/<version>.<TypeName>".
	AttachedObjectType *string `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique;uniqueIndex:idx_aor_marries_attached,where:relationship = 'marries' AND deleted_at IS NULL"`

	// AttachedObjectID is the database ID of the attaching object.
	AttachedObjectID *uint `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique;uniqueIndex:idx_aor_marries_attached,where:relationship = 'marries' AND deleted_at IS NULL"`

	// Relationship classifies this reference and drives lifecycle behavior
	// via gorm hooks and generated code that reveals information about a
	// type's foreign keys:
	//   - "describes": informational; does not block delete or update of the base.
	//   - "requires": blocks any caller from deleting the base while this
	//     reference exists.
	//   - "owns": blocks both delete and update of the base for any caller
	//     except the controller registered for the attached object's type,
	//     identified by its mTLS peer common name.
	//     An owned base has at most one owner (enforced by the partial
	//     index idx_aor_owns_base above); an owner may own many bases.
	//   - "marries": enforces 1-to-1 cardinality between base and attacher
	//     via the partial indexes above; blocks both delete and update of
	//     the base for any caller except the partner's controller.
	Relationship *Relationship `json:",omitempty" validate:"optional" gorm:"default:'describes'"`
}
