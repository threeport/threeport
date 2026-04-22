package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// BeforeCreate encrypts sensitive fields on the OCI provider before persisting
// to the database.
func (o *OciProvider) BeforeCreate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	createdObj := *o
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

// BeforeUpdate encrypts sensitive fields on the OCI provider before updating
// in the database.
func (o *OciProvider) BeforeUpdate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	updatedObj := tx.Statement.Dest.(*OciProvider)
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

// BeforeDelete prevents deletion of an OCI provider that has active runtime instances.
func (o *OciProvider) BeforeDelete(tx *gorm.DB) error {
	// check for active OKE runtime instances using this provider
	var instances []OciOkeKubernetesRuntimeInstance
	if result := tx.Where(
		"oci_provider_id = ?", *o.ID,
	).Find(&instances); result.Error != nil {
		return fmt.Errorf("failed to check for OCI OKE runtime instances: %w", result.Error)
	}

	if len(instances) > 0 {
		return &util.HttpError{
			StatusCode: 409,
			Message: fmt.Sprintf(
				"oci provider has %d active OKE runtime instance(s) - cannot be deleted",
				len(instances),
			),
		}
	}

	return nil
}
