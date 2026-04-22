package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/google/uuid"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// BeforeCreate validates a AWS Provider before persisting to the
// database.
func (a *AwsProvider) BeforeCreate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	isAccessKeyIDSet := false
	isSecretAccessKeySet := false

	createdObj := *a
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

		// check if AccessKeyID is set
		if field.Name == "AccessKeyID" {
			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}

			if underlyingValue != "" {
				isAccessKeyIDSet = true
			}
		}

		// check if SecretAccessKey is set
		if field.Name == "SecretAccessKey" {
			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}

			if underlyingValue != "" {
				isSecretAccessKeySet = true
			}
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

	// validate access & secret access keys
	if isAccessKeyIDSet && !isSecretAccessKeySet ||
		!isAccessKeyIDSet && isSecretAccessKeySet {
		return util.NewBadRequestError(
			"both access key id and secret access key must be set if one of them is provided",
		)
	}

	// generate and set external ID
	uuid := uuid.New().String()
	columnName := ns.ColumnName("", "ExternalId")
	tx.Statement.SetColumn(columnName, uuid)

	return nil
}

// BeforeUpdate validates that no immutable fields are attempting to be changed
// before updates are persisted.
func (a *AwsProvider) BeforeUpdate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	updatedObj := tx.Statement.Dest.(*AwsProvider)
	objVal := reflect.ValueOf(updatedObj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)

		// skip nil fields
		if !util.IsNonNilPtr(fieldVal) {
			continue
		}

		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" && tx.Statement.Changed(field.Name) {
			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}
			encryptedVal, err := encryption.Encrypt(encryptionKey, underlyingValue)
			if err != nil {
				return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
			}
			// use gorm to get column name from field name
			ns := schema.NamingStrategy{}
			columnName := ns.ColumnName("", field.Name)
			tx.Statement.SetColumn(columnName, encryptedVal)
		}
	}

	return nil
}
