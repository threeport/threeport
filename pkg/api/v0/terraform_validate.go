package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// BeforeCreate validates a Terraform Instance before persisting to the
// database.
func (t *TerraformInstance) BeforeCreate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	createdObj := *t
	objVal := reflect.ValueOf(&createdObj).Elem()
	objType := objVal.Type()
	ns := schema.NamingStrategy{}
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)

		// skip nil fields
		if !util.IsNonNilPtr(fieldVal) {
			continue
		}

		// encrypt field if encrypt tag is present
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {
			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}

			encryptedVal, err := encryption.Encrypt(encryptionKey, underlyingValue)
			if err != nil {
				return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
			}

			// use gorm to get column name from field name
			columnName := ns.ColumnName("", field.Name)
			tx.Statement.SetColumn(columnName, encryptedVal)
		}
	}

	return nil
}

// BeforeUpdate validates updates for a Terraform instance before persisting
// changes to the database
func (t *TerraformInstance) BeforeUpdate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	createdObj := *t
	objVal := reflect.ValueOf(&createdObj).Elem()
	objType := objVal.Type()
	ns := schema.NamingStrategy{}
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)

		// skip nil fields
		if !util.IsNonNilPtr(fieldVal) {
			continue
		}

		// encrypt field if encrypt tag is present
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {
			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}
			// check to see if the input value is already encrypted - if so skip
			// so the value isn't double-encryped
			if encryption.IsEncrypted(encryptionKey, underlyingValue) {
				continue
			}

			encryptedVal, err := encryption.Encrypt(encryptionKey, underlyingValue)
			if err != nil {
				return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
			}

			// use gorm to get column name from field name
			columnName := ns.ColumnName("", field.Name)
			tx.Statement.SetColumn(columnName, encryptedVal)
		}
	}

	return nil
}
