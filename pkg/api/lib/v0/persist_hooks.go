package v0

import (
	"github.com/iancoleman/strcase"
	"gorm.io/gorm"
)

// PersistFalseField names a field on an API type tagged `persist:"false"`.
// Only the Go field name is needed; the GORM column is derived via the
// default snake-case naming strategy at write time.
type PersistFalseField struct {
	Name string
}

// PersistFalseFieldProvider is implemented by every API type with at
// least one persist-false-tagged field. The SDK generates the method on
// each qualifying type so runtime hooks read the list without walking
// struct tags.
type PersistFalseFieldProvider interface {
	PersistFalseFields() []PersistFalseField
}

// persistFalseFieldsFor returns the persist-false-tagged fields of obj,
// or nil if obj doesn't implement the provider.
func persistFalseFieldsFor(obj interface{}) []PersistFalseField {
	p, ok := obj.(PersistFalseFieldProvider)
	if !ok {
		return nil
	}
	return p.PersistFalseFields()
}

// ProcessPersistFalseTaggedFields nulls every column for a field tagged
// `persist:"false"` before the row is written, so the value never
// reaches the database. The value travels through the notification
// payload to the controller and is then dropped.
//
// On update gorm fires the hook on Statement.Model (the loaded row);
// redirect to Statement.Dest so the inbound write side is operated on
// instead of stale loaded values. Create paths leave Model == Dest.
func ProcessPersistFalseTaggedFields(tx *gorm.DB, obj interface{}) error {
	target := obj
	if dest, ok := tx.Statement.Dest.(PersistFalseFieldProvider); ok && dest != obj {
		target = dest
	}

	for _, field := range persistFalseFieldsFor(target) {
		tx.Statement.SetColumn(strcase.ToSnake(field.Name), nil)
	}

	return nil
}
