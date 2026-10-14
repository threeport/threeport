package v0

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// sqlIdent is the shape of a table or column name this hook will interpolate.
var sqlIdent = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ErrMsgExternalUpdateBlocked is the substring present in any 400 error
// returned when an object cannot be updated because it is owned by a
// controller. The client lib detects this fragment to produce ErrObjectOwned;
// both sites must be updated together if this text changes.
const ErrMsgExternalUpdateBlocked = "cannot be updated externally"

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

// AssociationTypesProvider is implemented by API types that list owned
// has-many children with no requires back-reference, as fully qualified type names.
type AssociationTypesProvider interface {
	AssociationTypes() []string
}

// AssociationRequiredByTypesProvider is implemented by API types that list
// dependents which require them. Those children are not owned.
type AssociationRequiredByTypesProvider interface {
	AssociationRequiredByTypes() []string
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
// reference for each foreign key that has a value, then records an owner
// when the create request names one.
func processRelationshipTaggedFieldsAfterCreate(tx *gorm.DB, obj interface{}) error {
	foreignKeys := relationshipTaggedForeignKeysFor(obj)
	if len(foreignKeys) > 0 {
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
	}
	return recordOwnerOnCreate(tx, obj)
}

// ownerReader is the create-request owner. Common implements it.
type ownerReader interface {
	GetOwner() *OwnerRef
}

// recordOwnerOnCreate inserts the owns reference and sets the owner's field in the create transaction.
// It checks the caller organization and the owned row, and does not run the owner's before-update hook.
func recordOwnerOnCreate(tx *gorm.DB, obj interface{}) error {
	reader, ok := obj.(ownerReader)
	if !ok || reader.GetOwner() == nil {
		return nil
	}
	owner := reader.GetOwner()
	if owner.Type == nil || owner.ID == nil || owner.Field == nil {
		return util.NewBadRequestError("owner requires a type, an id, and a field")
	}
	if err := rejectOwnerOutsideCallerNamespace(tx, *owner.Type); err != nil {
		return err
	}

	// the new row is the owned object
	objType := obj.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	objID := util.ObjectID(obj)
	if objID == nil {
		return fmt.Errorf("cannot record owner for %s: ID is nil after create", objType)
	}
	table, err := tableForQualifiedType(*owner.Type)
	if err != nil {
		return err
	}
	column, err := columnForOwnerField(*owner.Field)
	if err != nil {
		return err
	}
	if err := requireOwnerColumn(tx, table, column); err != nil {
		return err
	}
	if err := rejectExternalOwnedUpdate(tx, *owner.Type, *owner.ID); err != nil {
		return err
	}
	if err := tx.Create(&AttachedObjectReference{
		ObjectID:           objID,
		ObjectType:         &objType,
		AttachedObjectID:   owner.ID,
		AttachedObjectType: owner.Type,
		Relationship:       util.Ptr(RelationshipOwns),
	}).Error; err != nil {
		return fmt.Errorf("failed to create owner reference for %s/%d: %w", objType, *objID, err)
	}

	// overwrite a saved id whose child is gone, and block the write while that child is live
	held, err := ownerColumnHoldsOtherLiveChild(tx, table, column, *owner.ID, *objID, tx.Statement.Table)
	if err != nil {
		return err
	}
	predicate := ""
	args := []any{*objID, *owner.ID}
	if held {
		// zero rows makes the create roll back, so two live children cannot share the column
		predicate = fmt.Sprintf(" AND (%s IS NULL OR %s = ?)", column, column)
		args = append(args, *objID)
	}
	result := lib.NewCleanSession(tx).Exec(
		fmt.Sprintf(
			"UPDATE %s SET %s = ? WHERE id = ? AND deleted_at IS NULL%s",
			table, column, predicate,
		),
		args...,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to set %s.%s for owner %d: %w", table, column, *owner.ID, result.Error)
	}
	if result.RowsAffected != 1 {
		return util.NewBadRequestError(fmt.Sprintf(
			"owner update for %s.%s id %d changed %d rows",
			table, column, *owner.ID, result.RowsAffected,
		))
	}
	return nil
}

// ownerColumnHoldsOtherLiveChild reports that the owner column stores a different id whose row is still live.
// A deleted or deletion-scheduled child is not live, so the caller overwrites that saved id.
func ownerColumnHoldsOtherLiveChild(tx *gorm.DB, ownerTable, column string, ownerID, newID uint, childTable string) (bool, error) {
	// read the id the owner column already stores
	var current sql.NullInt64
	row := lib.NewCleanSession(tx).Raw(
		fmt.Sprintf("SELECT %s FROM %s WHERE id = ? AND deleted_at IS NULL", column, ownerTable),
		ownerID,
	).Row()
	if err := row.Scan(&current); err != nil {
		return false, fmt.Errorf("failed to read %s.%s for owner %d: %w", ownerTable, column, ownerID, err)
	}

	// an empty column or this same child needs no live-row check
	if !current.Valid || uint(current.Int64) == newID {
		return false, nil
	}

	// count that saved id only while its row is present and not scheduled for deletion
	var live int64
	err := lib.NewCleanSession(tx).Raw(
		fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE id = ? AND deleted_at IS NULL AND deletion_scheduled IS NULL",
			childTable,
		),
		current.Int64,
	).Scan(&live).Error
	if err != nil {
		return false, fmt.Errorf("failed to read child %d from %s: %w", current.Int64, childTable, err)
	}
	return live > 0, nil
}

// tableForQualifiedType maps a fully qualified type to its database table.
// A threeport.io type drops the namespace. Any other namespace stays.
// The rest of the name matches TableName().
func tableForQualifiedType(objectType string) (string, error) {
	namespace, version, typeName, ok := lib.ParseQualifiedType(objectType)
	if !ok {
		return "", util.NewBadRequestError(fmt.Sprintf("owner type %q is not a fully qualified type", objectType))
	}
	plural := pluralize.NewClient().Pluralize(strcase.ToSnake(typeName), 2, false)
	raw := version + "_" + plural
	if namespace != lib.CoreApiNamespace {
		raw = namespace + "_" + raw
	}
	table := sanitizeIdent(raw)
	if !sqlIdent.MatchString(table) {
		return "", util.NewBadRequestError(fmt.Sprintf("owner type %q does not name a table", objectType))
	}
	return table, nil
}

// columnForOwnerField maps the owner's Go field to its column.
func columnForOwnerField(field string) (string, error) {
	column := sanitizeIdent(strcase.ToSnake(field))
	if !sqlIdent.MatchString(column) {
		return "", util.NewBadRequestError(fmt.Sprintf("owner field %q is not a column name", field))
	}
	if column == "id" {
		return "", util.NewBadRequestError(fmt.Sprintf("owner field %q names the primary key", field))
	}
	return column, nil
}

// sanitizeIdent matches sdk util.TableName without importing that package.
func sanitizeIdent(input string) string {
	input = strings.ToLower(input)
	re := regexp.MustCompile(`[^a-z0-9_]`)
	output := re.ReplaceAllString(input, "_")
	output = strings.Trim(output, "_")
	if len(output) > 63 {
		output = output[:63]
	}
	return output
}

// requireOwnerColumn rejects a table or column the database does not have.
// SQLite has no information_schema, so that driver reads pragma_table_info.
func requireOwnerColumn(tx *gorm.DB, table, column string) error {
	var n int64
	session := lib.NewCleanSession(tx)
	var err error
	if session.Dialector.Name() == "sqlite" {
		err = session.Raw(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?",
			table, column,
		).Scan(&n).Error
	} else {
		err = session.Raw(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?",
			table, column,
		).Scan(&n).Error
	}
	if err != nil {
		return fmt.Errorf("failed to look up %s.%s: %w", table, column, err)
	}
	if n != 1 {
		return util.NewBadRequestError(fmt.Sprintf("owner column %s.%s does not exist", table, column))
	}
	return nil
}

// processRelationshipTaggedFieldsBeforeUpdate rejects any change to a
// relationship FK that was previously set (set->different, set->clear),
// and rejects updates to a row owned or married by another object.
// Once a relationship FK has been set, the only way to change it is to
// tear down and recreate the holding object.
//
// This hook enforces two rules:
//
//  1. Relationship FK immutability: once an FK has been set, it can't
//     be changed or cleared. Only clear->set is allowed; after that
//     the holder must be torn down and recreated.
//  2. Caller authorization: if this row is owned or married by
//     another object via an incoming AOR, only the owner/partner
//     controller (or any control-plane caller) may update it.
func processRelationshipTaggedFieldsBeforeUpdate(tx *gorm.DB, obj interface{}) error {
	objType := obj.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	objID := util.ObjectID(obj)
	if objID == nil {
		return nil
	}

	// We don't reach for lib.IsFieldChanged here because the check
	// needs (a) the "previously non-nil" filter; only FKs that were
	// already set are immutable, and (b) all FKs diffed in a single
	// DB read. Calling IsFieldChanged per FK would re-load the pre
	// row N times under PUT.
	//
	// Read the committed pre-update row and a map of the inbound FK
	// values keyed by Go field name so the loop below can compare
	// each pre FK against its inbound counterpart.
	preUpdateObj, err := lib.LoadObjectFromDB(tx)
	if err != nil {
		return err
	}
	incomingByField := make(map[string]*uint)
	for _, foreignKey := range relationshipTaggedForeignKeysFor(lib.IncomingValues(tx)) {
		incomingByField[foreignKey.FieldName] = foreignKey.ObjectID
	}

	// A nil inbound FK means different things under PUT and PATCH:
	// under PUT (full replace) it's an explicit clear; under PATCH
	// it's "the field was absent from the payload, leave it alone".
	// isReplace lets the loop distinguish the two.
	isReplace := lib.IsFullReplace(tx)

	for _, preUpdateForeignKey := range relationshipTaggedForeignKeysFor(preUpdateObj) {
		// FK wasn't set before this update; clear->set (or stay nil)
		// is allowed, so nothing to enforce here.
		if preUpdateForeignKey.ObjectID == nil {
			continue
		}
		incomingID := incomingByField[preUpdateForeignKey.FieldName]
		// cleared: caller explicitly nulled the FK (only meaningful
		// under PUT, see the isReplace note above).
		cleared := incomingID == nil && isReplace
		// changed: caller set the FK to a different non-nil value.
		changed := incomingID != nil && *incomingID != *preUpdateForeignKey.ObjectID
		if cleared || changed {
			return util.NewBadRequestError(fmt.Sprintf(
				"%s is immutable once set; tear down and recreate the holder to undo this relationship",
				preUpdateForeignKey.FieldName,
			))
		}
	}

	// the rest of this function is the caller-authorization check
	return rejectExternalOwnedUpdate(tx, objType, *objID)
}

// rejectOwnerOutsideCallerNamespace rejects an owner whose type namespace differs from the caller organization.
// The threeport-owner certificate and an authentication-off caller may name any owner.
func rejectOwnerOutsideCallerNamespace(tx *gorm.DB, ownerType string) error {
	caller := lib.Caller(tx.Statement.Context)
	// allow the threeport-owner certificate
	if caller.CommonName == "threeport-owner" {
		return nil
	}
	// allow the authentication-off caller, which has no organization
	if caller.OrganizationalUnit == auth.OUControlPlane && caller.Organization == "" {
		return nil
	}
	organization := caller.Organization
	if organization == "" {
		return util.NewBadRequestError("owner requires a caller organization")
	}
	namespace, _, _, ok := lib.ParseQualifiedType(ownerType)
	if !ok {
		return util.NewBadRequestError(fmt.Sprintf("owner type %q is not a fully qualified type", ownerType))
	}
	if namespace != organization {
		return util.NewBadRequestError(fmt.Sprintf(
			"owner type %s is outside caller organization %s",
			ownerType,
			organization,
		))
	}
	return nil
}

// rejectExternalOwnedUpdate rejects an update when another object owns or marries the row.
// A control-plane caller is allowed through.
func rejectExternalOwnedUpdate(tx *gorm.DB, objType string, objID uint) error {
	var ownedOrMarriedRefs []AttachedObjectReference
	if err := lib.NewCleanSession(tx).
		Where(
			"object_type = ? AND object_id = ? AND relationship IN ?",
			objType, objID, []Relationship{RelationshipOwns, RelationshipMarries},
		).
		Find(&ownedOrMarriedRefs).Error; err != nil {
		return fmt.Errorf(
			"failed to look up owning attached object references for %s/%d: %w",
			objType, objID, err,
		)
	}

	// no incoming owns or marries reference, so any caller may update the row
	if len(ownedOrMarriedRefs) == 0 {
		return nil
	}

	// control-plane callers maintain owned rows
	if lib.Caller(tx.Statement.Context).OrganizationalUnit == auth.OUControlPlane {
		return nil
	}

	// name the object that holds the lifecycle
	owner := "another object"
	if ownedOrMarriedRefs[0].AttachedObjectType != nil && ownedOrMarriedRefs[0].AttachedObjectID != nil {
		owner = fmt.Sprintf("%s/%d", *ownedOrMarriedRefs[0].AttachedObjectType, *ownedOrMarriedRefs[0].AttachedObjectID)
	}
	return util.NewBadRequestError(fmt.Sprintf(
		"object is owned by %s and %s; tear down the owner first",
		owner,
		ErrMsgExternalUpdateBlocked,
	))
}

// processRelationshipTaggedFieldsAfterUpdate inserts an attached
// object reference for each relationship-tagged foreign key whose
// value lacks one. Foreign-key clears are not handled - the
// before-update hook rejects set->clear.
//
// FK clears aren't handled here because the before-update hook
// rejects set->clear before we get here. The only transitions to
// react to are nil->set and (no-op) set->same-value, both of which
// reduce to "ensure an AOR exists for the current value".
func processRelationshipTaggedFieldsAfterUpdate(tx *gorm.DB, obj interface{}) error {
	if len(relationshipTaggedForeignKeysFor(obj)) == 0 {
		// type has no relationship-tagged FKs → nothing to sync
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

	// Reload so the FK values reflect what was just committed. GORM
	// merges the update into the receiver before this hook fires, so
	// the reload is mostly defensive; it keeps the FK reads correct
	// regardless of which call shape (PATCH or PUT) drove the update.
	updatedObj, err := lib.LoadObjectFromDB(tx)
	if err != nil {
		return err
	}

	for _, updatedObjForeignKey := range relationshipTaggedForeignKeysFor(updatedObj) {
		// FK isn't set → no AOR needed for this column
		if updatedObjForeignKey.ObjectID == nil {
			continue
		}

		// Does an AOR already exist linking (FK target → this row)?
		// Use a clean session: we're issuing an unrelated query from
		// inside an update hook, so the surrounding statement's
		// WHERE clauses would otherwise apply.
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
			// No AOR yet; this is either the initial nil->set transition
			// or a sync after a backfill. Insert one now.
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
