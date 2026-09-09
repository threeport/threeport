package v0

import (
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
)

// ProcessCoreTaggedFieldsBeforeCreate runs core tag-triggered behavior on
// an API object before create.
func ProcessCoreTaggedFieldsBeforeCreate(tx *gorm.DB, obj interface{}) error {
	if err := lib.ProcessEncryptTaggedFields(tx, obj); err != nil {
		return err
	}
	return lib.ProcessPersistFalseTaggedFields(tx, obj)
}

// ProcessCoreTaggedFieldsBeforeUpdate runs core tag-triggered behavior on
// an API object before update.
func ProcessCoreTaggedFieldsBeforeUpdate(tx *gorm.DB, obj interface{}) error {
	if err := processRelationshipTaggedFieldsBeforeUpdate(tx, obj); err != nil {
		return err
	}
	if err := lib.ProcessEncryptTaggedFields(tx, obj); err != nil {
		return err
	}
	return lib.ProcessPersistFalseTaggedFields(tx, obj)
}

// ProcessCoreTaggedFieldsBeforeDelete runs core tag-triggered behavior on
// an API object before delete.
func ProcessCoreTaggedFieldsBeforeDelete(tx *gorm.DB, obj interface{}) error {
	return processRelationshipTaggedFieldsBeforeDelete(tx, obj)
}

// ProcessCoreTaggedFieldsAfterCreate runs core tag-triggered behavior on
// an API object after create.
func ProcessCoreTaggedFieldsAfterCreate(tx *gorm.DB, obj interface{}) error {
	return processRelationshipTaggedFieldsAfterCreate(tx, obj)
}

// ProcessCoreTaggedFieldsAfterUpdate runs core tag-triggered behavior on
// an API object after update.
func ProcessCoreTaggedFieldsAfterUpdate(tx *gorm.DB, obj interface{}) error {
	return processRelationshipTaggedFieldsAfterUpdate(tx, obj)
}

// ProcessCoreTaggedFieldsAfterDelete runs core tag-triggered behavior on
// an API object after delete.
func ProcessCoreTaggedFieldsAfterDelete(tx *gorm.DB, obj interface{}) error {
	return nil
}
