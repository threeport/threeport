package v0

import (
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Relationship classifies how an AttachedObjectReference relates the base
// object to the attached object. Drives lifecycle behavior (deletion and
// update blocking).
type Relationship string

const (
	RelationshipDescribes Relationship = "describes"
	RelationshipRequires  Relationship = "requires"
	RelationshipOwns      Relationship = "owns"
	RelationshipMarries   Relationship = "marries"
)

// RelationshipTaggedForeignKey describes a *uint ID field on an API type
// tagged with a `relationship:` value. The SDK generates a
// RelationshipTaggedForeignKeys method per type that returns one entry
// per such field, so runtime hooks read the list directly instead of
// walking struct tags via reflection.
type RelationshipTaggedForeignKey struct {
	FieldName    string
	ObjectType   string // e.g. "KubernetesWorkloadInstance"
	Relationship Relationship
	ObjectID     *uint
}

// RelationshipTaggedForeignKeyProvider is implemented by every API type
// with at least one relationship-tagged foreign key.
type RelationshipTaggedForeignKeyProvider interface {
	RelationshipTaggedForeignKeys() []RelationshipTaggedForeignKey
}

// relationshipTaggedForeignKeysFor returns the tagged foreign keys of obj, or nil.
func relationshipTaggedForeignKeysFor(obj interface{}) []RelationshipTaggedForeignKey {
	p, ok := obj.(RelationshipTaggedForeignKeyProvider)
	if !ok {
		return nil
	}
	return p.RelationshipTaggedForeignKeys()
}

// insertAttachedObjectReference creates an AOR row for foreignKey. The
// `owns` and `requires` edges are immutable so this runs only on first set.
func insertAttachedObjectReference(
	tx *gorm.DB,
	foreignKey RelationshipTaggedForeignKey,
	attachedObjectType string,
	attachedObjectID uint,
) error {
	if err := tx.Create(&AttachedObjectReference{
		ObjectID:           foreignKey.ObjectID,
		ObjectType:         &foreignKey.ObjectType,
		AttachedObjectID:   &attachedObjectID,
		AttachedObjectType: &attachedObjectType,
		Relationship:       &foreignKey.Relationship,
	}).Error; err != nil {
		return fmt.Errorf(
			"failed to create attached object reference for %s.%s: %w",
			attachedObjectType, foreignKey.FieldName, err,
		)
	}
	return nil
}

// processRelationshipTaggedFieldsAfterCreate inserts an attached object
// reference for each foreign key that has a value.
func processRelationshipTaggedFieldsAfterCreate(tx *gorm.DB, obj interface{}) error {
	foreignKeys := relationshipTaggedForeignKeysFor(obj)
	if len(foreignKeys) == 0 {
		return nil
	}
	objType := obj.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	objID := util.ObjectID(obj)
	if objID == nil {
		return fmt.Errorf(
			"cannot create attached object reference for %s: ID is nil after create",
			objType,
		)
	}
	for _, foreignKey := range foreignKeys {
		if foreignKey.ObjectID == nil {
			continue
		}
		if err := insertAttachedObjectReference(tx, foreignKey, objType, *objID); err != nil {
			return err
		}
	}
	return nil
}

// processRelationshipTaggedFieldsBeforeUpdate rejects any change to a
// relationship FK that was previously set (set->different, set->clear),
// and rejects updates to a row owned or married by another object.
// Once a relationship FK has been set, the only way to change it is to
// tear down and recreate the holding object.
func processRelationshipTaggedFieldsBeforeUpdate(tx *gorm.DB, obj interface{}) error {
	objType := obj.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	objID := util.ObjectID(obj)
	if objID == nil {
		return nil
	}

	// load the pre-update row so the FK check compares against the
	// committed state, not the inbound payload. obj alone reflects the
	// caller's intent, which doesn't tell us whether each FK was
	// previously set.
	preUpdateObj, err := lib.LoadUpdatedObjFromDB(tx, obj, *objID)
	if err != nil {
		return err
	}

	// reject any change (set->different OR set->clear) to a relationship
	// FK that was previously non-nil. the initial clear->set transition
	// stays allowed; everything after that requires teardown.
	for _, preUpdateForeignKey := range relationshipTaggedForeignKeysFor(preUpdateObj) {
		if preUpdateForeignKey.ObjectID != nil && tx.Statement.Changed(string(preUpdateForeignKey.FieldName)) {
			return util.NewBadRequestError(fmt.Sprintf(
				"%s is immutable once set; tear down and recreate the holder to undo this relationship",
				preUpdateForeignKey.FieldName,
			))
		}
	}

	// look up incoming owns/marries refs to decide whether this row's
	// non-FK fields can be updated by the caller
	var ownedOrMarriedRefs []AttachedObjectReference
	if err := tx.
		Where(
			"object_type = ? AND object_id = ? AND relationship IN ?",
			objType, objID, []Relationship{RelationshipOwns, RelationshipMarries},
		).
		Find(&ownedOrMarriedRefs).Error; err != nil {
		return fmt.Errorf(
			"failed to look up owning attached object references for %s/%d: %w",
			objType, *objID, err,
		)
	}

	// no incoming owns/marries refs means anyone can update this row
	if len(ownedOrMarriedRefs) == 0 {
		return nil
	}

	// the owner/partner controller is allowed to update its owned/married
	// row; any control-plane caller bypasses the block
	if lib.Caller(tx.Statement.Context).OrganizationalUnit == auth.OUControlPlane {
		return nil
	}

	// external caller hit a row that's owned or married; reject the update
	// and tell them which object holds the lifecycle
	owner := "another object"
	if ownedOrMarriedRefs[0].AttachedObjectType != nil && ownedOrMarriedRefs[0].AttachedObjectID != nil {
		owner = fmt.Sprintf("%s/%d", *ownedOrMarriedRefs[0].AttachedObjectType, *ownedOrMarriedRefs[0].AttachedObjectID)
	}
	return util.NewBadRequestError(fmt.Sprintf(
		"object is owned by %s and cannot be updated externally; tear down the owner first",
		owner,
	))
}

// processRelationshipTaggedFieldsAfterUpdate inserts an attached
// object reference for each relationship-tagged foreign key whose
// value lacks one. Foreign-key clears are not handled - the
// before-update hook rejects set->clear.
func processRelationshipTaggedFieldsAfterUpdate(tx *gorm.DB, obj interface{}) error {
	if len(relationshipTaggedForeignKeysFor(obj)) == 0 {
		return nil
	}

	objType := obj.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	objID := util.ObjectID(obj)
	if objID == nil {
		return fmt.Errorf(
			"cannot sync attached object references for %s: ID is nil after update",
			objType,
		)
	}

	// reload from the database so the foreign-key values reflect the
	// just-committed update, not the pre-update snapshot still on obj
	updatedObj, err := lib.LoadUpdatedObjFromDB(tx, obj, *objID)
	if err != nil {
		return err
	}

	for _, updatedObjForeignKey := range relationshipTaggedForeignKeysFor(updatedObj) {
		if updatedObjForeignKey.ObjectID == nil {
			continue
		}

		var count int64
		err := lib.NewCleanSession(tx).
			Model(&AttachedObjectReference{}).
			Where(
				"object_type = ? AND object_id = ? AND attached_object_type = ? AND attached_object_id = ?",
				updatedObjForeignKey.ObjectType, *updatedObjForeignKey.ObjectID, objType, *objID,
			).
			Count(&count).Error
		if err != nil {
			return fmt.Errorf(
				"failed to count attached object references for %s.%s: %w",
				objType, updatedObjForeignKey.FieldName, err,
			)
		}

		if count == 0 {
			if err := insertAttachedObjectReference(tx, updatedObjForeignKey, objType, *objID); err != nil {
				return err
			}
		}
	}
	return nil
}

// processRelationshipTaggedFieldsBeforeDelete rejects deletion when an
// incoming reference blocks it, then removes the row's outgoing references.
func processRelationshipTaggedFieldsBeforeDelete(tx *gorm.DB, obj interface{}) error {
	if err := CheckBlockingAttachedObjectReferences(tx, obj); err != nil {
		return err
	}
	objType := obj.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	objID := util.ObjectID(obj)
	if objID == nil {
		return nil
	}

	// clean up outgoing references where this object is the attacher.
	// Those rows become orphans once this row is gone, so delete them
	// in the same transaction.
	if err := tx.
		Where("attached_object_type = ? AND attached_object_id = ?", objType, objID).
		Delete(&AttachedObjectReference{}).Error; err != nil {
		return fmt.Errorf(
			"failed to clean up outgoing attached object references for %s/%d: %w",
			objType, *objID, err,
		)
	}
	return nil
}
