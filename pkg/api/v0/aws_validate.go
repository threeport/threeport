package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/threeport/threeport/pkg/encryption/v0"
)

// BeforeCreate validates a KubernetesRuntimeInstance before persisting to the
// database.
func (a *AwsAccount) BeforeCreate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	createdObj := *a
	objVal := reflect.ValueOf(&createdObj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {
			if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
				underlyingVal := fieldVal.Elem()
				createdVal, ok := underlyingVal.Interface().(string)
				if !ok {
					return fmt.Errorf("%s field tagged for encryption but not a string value", field.Name)
				}
				encryptedVal, err := encryption.Encrypt(encryptionKey, createdVal)
				if err != nil {
					return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
				}
				// use gorm to get column name from field name
				ns := schema.NamingStrategy{}
				columnName := ns.ColumnName("", field.Name)
				tx.Statement.SetColumn(columnName, encryptedVal)
			}
		}
	}

	return nil
}

// BeforeUpdate validates that no immutable fields are attempting to be changed
// before updates are persisted.
func (a *AwsAccount) BeforeUpdate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	updatedObj := tx.Statement.Dest.(AwsAccount)
	objVal := reflect.ValueOf(&updatedObj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" && tx.Statement.Changed(field.Name) {
			if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
				underlyingVal := fieldVal.Elem()
				updatedVal, ok := underlyingVal.Interface().(string)
				if !ok {
					return fmt.Errorf("%s field tagged for encryption but not a string value", field.Name)
				}
				encryptedVal, err := encryption.Encrypt(encryptionKey, updatedVal)
				if err != nil {
					return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
				}
				// use gorm to get column name from field name
				ns := schema.NamingStrategy{}
				columnName := ns.ColumnName("", field.Name)
				tx.Statement.SetColumn(columnName, encryptedVal)
			}
		}
	}

	return nil
}
