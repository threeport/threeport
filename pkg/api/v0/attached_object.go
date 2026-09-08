package v0

// AttachedObjectReferenceRelationshipField is the Go struct field name
// of AttachedObjectReference.Relationship. Used by hooks that need to
// detect changes to the field via tx.Statement.Changed().
const AttachedObjectReferenceRelationshipField = "Relationship"

// AttachedObjectReference is a reference to an attached object.
//
// Four DB indexes are declared in the GORM tags below:
//
//   - idx_attached_object_unique: partial unique composite across
//     (object_type, object_id, attached_object_type, attached_object_id)
//     where deleted_at IS NULL. Enforces that a given (base, attacher)
//     pair appears in at most one *live* row regardless of relationship
//     kind.
//
//   - idx_attached_object_reference_marries_base: partial unique composite across
//     (object_type, object_id) where relationship = 'marries' AND
//     deleted_at IS NULL. Enforces that the base side of a marriage
//     appears in at most one *live* marries row (1-to-1 cardinality for
//     the base). The deleted_at predicate keeps soft-deleted rows out
//     of the unique slot so a base can be re-married after teardown.
//
//   - idx_attached_object_reference_marries_attached: partial unique composite across
//     (attached_object_type, attached_object_id) where relationship =
//     'marries' AND deleted_at IS NULL. Same constraint applied to the
//     attacher side.
//
//   - idx_attached_object_reference_owns_base: partial unique composite across
//     (object_type, object_id) where relationship = 'owns' AND
//     deleted_at IS NULL. Enforces that an owned base appears in at
//     most one *live* owns row. The attacher side is intentionally
//     unconstrained for owns: an owner may own many bases.
//
// Each participating column repeats the index name in its `uniqueIndex:`
// tag; GORM bundles them by name. The `,where:...` suffix makes an index
// partial: only rows matching the predicate are indexed. Every one of
// these carries deleted_at IS NULL because attached object references
// use gorm soft-delete (see Common). Without it a soft-deleted row keeps
// its unique slot until cockroach hard-deletes it, so re-attaching the
// same pair is refused in the meantime and the caller is answered a
// conflict it cannot clear.
type AttachedObjectReference struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// ObjectType is the kind of the base object being attached to. Read
	// "this attached object attaches to that object": the no-prefix
	// Object* fields name the "that" (anchor) side. The naming is
	// directional because the attached object is the side that can
	// determine the base object's lifecycle, depending on the type of
	// relationship (see below). Stored as a fully qualified type name
	// in the form "<api-namespace>/<version>.<TypeName>".
	ObjectType *string `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique,where:deleted_at IS NULL;uniqueIndex:idx_attached_object_reference_marries_base,where:relationship = 'marries' AND deleted_at IS NULL;uniqueIndex:idx_attached_object_reference_owns_base,where:relationship = 'owns' AND deleted_at IS NULL"`

	// ObjectID is the database ID of the base object.
	ObjectID *uint `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique,where:deleted_at IS NULL;uniqueIndex:idx_attached_object_reference_marries_base,where:relationship = 'marries' AND deleted_at IS NULL;uniqueIndex:idx_attached_object_reference_owns_base,where:relationship = 'owns' AND deleted_at IS NULL"`

	// AttachedObjectType is the kind of the object doing the attaching;
	// the side that can determine the base object's lifecycle. Stored
	// as a fully qualified type name in the form
	// "<api-namespace>/<version>.<TypeName>".
	AttachedObjectType *string `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique,where:deleted_at IS NULL;uniqueIndex:idx_attached_object_reference_marries_attached,where:relationship = 'marries' AND deleted_at IS NULL"`

	// AttachedObjectID is the database ID of the attaching object.
	AttachedObjectID *uint `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_attached_object_unique,where:deleted_at IS NULL;uniqueIndex:idx_attached_object_reference_marries_attached,where:relationship = 'marries' AND deleted_at IS NULL"`

	// Relationship classifies this reference:
	//   - "describes": informational; does not block delete or update of the base.
	//   - "requires": blocks any caller from deleting the base while this
	//     reference exists, control plane callers included.
	//   - "owns": blocks both delete and update of the base, except for a
	//     caller whose mTLS certificate carries the control plane
	//     organizational unit. That exemption covers every control plane
	//     component, not only the controller registered for the attached
	//     object's type.
	//     An owned base has at most one owner; an owner may own many bases.
	//   - "marries": enforces 1-to-1 cardinality between base and attacher
	//     and blocks both delete and update of the base under the same
	//     control plane exemption as "owns".
	Relationship *Relationship `json:",omitempty" validate:"optional" gorm:"default:'describes'"`
}
