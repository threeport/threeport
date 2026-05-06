package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ProcessCoreTaggedFieldsBeforeCreate runs core tag-triggered behavior on
// an API object before create.
func ProcessCoreTaggedFieldsBeforeCreate(tx *gorm.DB, obj interface{}) error {
	return processEncryptTaggedFields(tx, obj, false)
}

// ProcessCoreTaggedFieldsBeforeUpdate runs core tag-triggered behavior on
// an API object before update.
func ProcessCoreTaggedFieldsBeforeUpdate(tx *gorm.DB, obj interface{}) error {
	return processEncryptTaggedFields(tx, obj, true)
	// TODO: enforce tag-driven update blocks (e.g. immutable fields)
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

// processEncryptTaggedFields encrypts struct fields tagged `encrypt:"true"`
// and rejects the redacted placeholder. encrypts tagged fields, provided they are not
// already encrypted.
func processEncryptTaggedFields(tx *gorm.DB, obj interface{}, checkChanged bool) error {
	// shared symmetric key for AES-GCM, sourced from the API server's env
	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	// reflect over the dereferenced struct; column names follow GORM's default
	// naming strategy (CamelCase → snake_case)
	objVal := reflect.ValueOf(obj).Elem()
	objType := objVal.Type()
	ns := schema.NamingStrategy{}

	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		// only fields explicitly tagged for encryption participate
		if field.Tag.Get("encrypt") != "true" {
			continue
		}
		// in BeforeUpdate, leave fields the client didn't modify alone — the
		// existing DB ciphertext is already correct
		if checkChanged && !tx.Statement.Changed(field.Name) {
			continue
		}

		fieldVal := objVal.Field(i)
		columnName := ns.ColumnName("", field.Name)

		switch fieldVal.Kind() {
		case reflect.Ptr:
			// nil pointer means the client didn't send the field; nothing to
			// encrypt, existing DB value (if any) preserved
			if !util.IsNonNilPtr(fieldVal) {
				continue
			}
			plain, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", field.Name, err)
			}
			// reject the redacted placeholder — clients should send a real
			// value to change the field, or omit it to leave it unchanged
			if plain == encryption.RedactedValuePlaceholder {
				return util.NewBadRequestError(
					fmt.Sprintf(
						"field %s contains redacted placeholder; provide a real value or omit the field",
						field.Name,
					),
				)
			}
			// skip if the input is already encrypted — controllers are
			// responsible for decrypting before submitting, but a forgotten
			// decrypt round-trips ciphertext; skipping prevents double-encryption.
			if encryption.IsEncrypted(encryptionKey, plain) {
				continue
			}
			enc, err := encryption.Encrypt(encryptionKey, plain)
			if err != nil {
				return fmt.Errorf("failed to encrypt %s: %w", field.Name, err)
			}
			tx.Statement.SetColumn(columnName, enc)

		case reflect.Slice:
			// nil/empty slice: nothing to encrypt
			if fieldVal.IsNil() || fieldVal.Len() == 0 {
				continue
			}
			slice, ok := fieldVal.Interface().([]string)
			if !ok {
				return fmt.Errorf("encrypt tag on non-[]string slice field %s", field.Name)
			}
			// each entry is KEY=VALUE; encrypt only VALUE so KEYs stay readable
			// for callers that need to introspect them (e.g. env-var names)
			encSlice := make([]string, len(slice))
			for j, entry := range slice {
				parts := strings.SplitN(entry, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("%s[%d] is not in KEY=VALUE format", field.Name, j)
				}
				// reject the redacted placeholder — clients should send a real
				// value to change the field, or omit it to leave it unchanged
				if parts[1] == encryption.RedactedValuePlaceholder {
					return util.NewBadRequestError(
						fmt.Sprintf(
							"%s[%d] contains redacted placeholder; provide a real value or omit the entry",
							field.Name, j,
						),
					)
				}
				// skip if already encrypted — preserves the original entry to
				// avoid double-encryption when a controller round-trips it.
				if encryption.IsEncrypted(encryptionKey, parts[1]) {
					encSlice[j] = entry
					continue
				}
				encValue, err := encryption.Encrypt(encryptionKey, parts[1])
				if err != nil {
					return fmt.Errorf("failed to encrypt %s[%d]: %w", field.Name, j, err)
				}
				encSlice[j] = parts[0] + "=" + encValue
			}
			tx.Statement.SetColumn(columnName, encSlice)
		}
	}
	return nil
}
