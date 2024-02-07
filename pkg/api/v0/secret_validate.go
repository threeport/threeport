package v0

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// BeforeCreate validates a secret definition before
// persisting to the database.
func (s *SecretDefinition) BeforeCreate(tx *gorm.DB) error {
	createdObj := *s
	objVal := reflect.ValueOf(&createdObj).Elem()
	objType := objVal.Type()
	ns := schema.NamingStrategy{}

	// ensure Data is not persisted
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)

		if field.Name == "Data" {
			persist := field.Tag.Get("persist")
			if persist == "false" {
				columnName := ns.ColumnName("", field.Name)
				tx.Statement.SetColumn(columnName, nil)
			}
		}
	}

	return nil
}
