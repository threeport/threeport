package v0

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"gorm.io/gorm"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// EncryptedField describes an encrypt-tagged field on an API type. The SDK
// generates an EncryptedFields method per type that returns one entry per
// field, so runtime hooks read the list without walking struct tags. Name
// is the Go struct field name; the GORM column is derived via the default
// snake-case naming strategy at write time.
type EncryptedField struct {
	Name  string
	Value interface{} // *string or []string of KEY=VALUE
}

// EncryptedFieldProvider is implemented by every API type with at least one
// encrypt-tagged field.
type EncryptedFieldProvider interface {
	EncryptedFields() []EncryptedField
}

// encryptedFieldsFor returns the encrypt-tagged fields of obj, or nil.
func encryptedFieldsFor(obj interface{}) []EncryptedField {
	p, ok := obj.(EncryptedFieldProvider)
	if !ok {
		return nil
	}
	return p.EncryptedFields()
}

// ProcessEncryptTaggedFields encrypts fields tagged `encrypt:"true"`
// before the row is written, replacing plaintext with ciphertext via
// SetColumn. Fires from both BeforeCreate and BeforeUpdate hooks.
//
// On the update path the hook receiver may not hold the values to
// encrypt: under a PATCH (Model(&loaded).Updates(&patch)) the receiver
// is the loaded DB row whose encrypt-tagged fields hold existing
// ciphertext from disk, and the new plaintext lives in
// tx.Statement.Dest. This function routes through IncomingValues to
// always operate on the inbound values regardless of call shape; under
// a PUT (Save) or a create the redirect is a no-op because the
// receiver and tx.Statement.Dest are already the same object. See
// pkg/api/lib/v0/update_helpers.go for the full call-shape model.
func ProcessEncryptTaggedFields(tx *gorm.DB, obj interface{}) error {
	fields := encryptedFieldsFor(IncomingValues(tx))
	if len(fields) == 0 {
		return nil
	}

	// shared symmetric key for AES-GCM, sourced from the API server's env
	encryptionKey, err := encryption.KeyFromEnv()
	if err != nil {
		return err
	}

	for _, field := range fields {
		switch v := field.Value.(type) {
		case *string:
			// nil pointer means the client didn't send the field; nothing to
			// encrypt, existing DB value (if any) preserved
			if v == nil {
				continue
			}
			enc, err := encryptValue(tx, encryptionKey, *v, string(field.Name))
			if err != nil {
				return err
			}
			if enc != *v {
				tx.Statement.SetColumn(strcase.ToSnake(field.Name), enc)
			}

		case *[]string:
			// nil pointer: nothing to encrypt, existing DB value preserved
			if v == nil || len(*v) == 0 {
				continue
			}
			// same KEY=VALUE handling as []string above; deref and reuse
			encSlice := make([]string, len(*v))
			for j, entry := range *v {
				key, value, ok := strings.Cut(entry, "=")
				if !ok {
					// a bad-request error so the handler answers 400; a
					// plain error here reads as a server fault and the
					// caller never learns which entry to fix
					return util.NewBadRequestError(
						fmt.Sprintf("%s[%d] is not in KEY=VALUE format", field.Name, j),
					)
				}
				encValue, err := encryptValue(tx, encryptionKey, value, fmt.Sprintf("%s[%d]", field.Name, j))
				if err != nil {
					return err
				}
				if encValue == value {
					encSlice[j] = entry
				} else {
					encSlice[j] = key + "=" + encValue
				}
			}
			tx.Statement.SetColumn(strcase.ToSnake(field.Name), encSlice)

		default:
			return fmt.Errorf(
				"encrypt tag on unsupported field type for %s",
				field.Name,
			)
		}
	}
	return nil
}

// encryptValue encrypts plain and returns the ciphertext. Returns plain
// unchanged when already encrypted, and a bad-request error when plain is
// the redacted placeholder. fieldRef appears in error and log messages.
func encryptValue(tx *gorm.DB, encryptionKey, plain, fieldRef string) (string, error) {
	// reject the redacted placeholder; clients send a real value to change
	// the field or omit it to leave existing ciphertext in place
	if plain == encryption.RedactedValuePlaceholder {
		return "", util.NewBadRequestError(
			fmt.Sprintf("%s contains redacted placeholder; provide a real value or omit it", fieldRef),
		)
	}
	// skip if already encrypted; a forgotten decrypt by a controller would
	// otherwise double-encrypt the value
	if encryption.IsEncrypted(encryptionKey, plain) {
		tx.Logger.Info(
			tx.Statement.Context,
			"skipping encryption of %s: value already encrypted (likely a controller resubmitted ciphertext)",
			fieldRef,
		)
		return plain, nil
	}
	enc, err := encryption.Encrypt(encryptionKey, plain)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt %s: %w", fieldRef, err)
	}
	return enc, nil
}

// RedactEncryptedValues takes an API object, replaces the value on any
// encrypt-tagged fields with the redacted placeholder, and returns the
// object. Nil pointer fields are left as-is since there is nothing to
// redact. Slice fields are redacted element-by-element, preserving the
// KEY= prefix on each entry so the round trip back through
// ProcessEncryptTaggedFields still recognizes the KEY=VALUE shape.
func RedactEncryptedValues(obj interface{}) interface{} {
	p, ok := obj.(EncryptedFieldProvider)
	if !ok {
		return obj
	}
	for _, field := range p.EncryptedFields() {
		switch v := field.Value.(type) {
		case *string:
			if v == nil {
				continue
			}
			*v = encryption.RedactedValuePlaceholder
		case *[]string:
			if v == nil {
				continue
			}
			for j, entry := range *v {
				// preserve the KEY= prefix so tptctl get -> tptctl replace
				// round trips leave the encrypt hook a well-formed
				// KEY=VALUE entry to reject or accept
				key, _, ok := strings.Cut(entry, "=")
				if !ok {
					(*v)[j] = encryption.RedactedValuePlaceholder
					continue
				}
				(*v)[j] = key + "=" + encryption.RedactedValuePlaceholder
			}
		}
	}
	return obj
}

// DecryptValues takes an API object and the encryption key, decrypts any
// encrypt-tagged fields and returns the object with values in plaintext.
// Nil pointer fields are left as-is since there is nothing to decrypt.
// Slice fields are decrypted element-by-element, preserving the KEY=
// prefix on each entry (only the value after the first `=` is encrypted on
// write, so only that portion is decrypted on read).
func DecryptValues(obj interface{}, encryptionKey string) (interface{}, error) {
	p, ok := obj.(EncryptedFieldProvider)
	if !ok {
		return obj, nil
	}
	for _, field := range p.EncryptedFields() {
		switch v := field.Value.(type) {
		case *string:
			if v == nil {
				continue
			}
			decryptedVal, err := encryption.Decrypt(encryptionKey, *v)
			if err != nil {
				return obj, fmt.Errorf("failed to decrypt value in field %s: %w", field.Name, err)
			}
			*v = decryptedVal
		case *[]string:
			if v == nil {
				continue
			}
			decrypted, err := encryption.DecryptEnvSlice(*v, encryptionKey)
			if err != nil {
				return obj, fmt.Errorf("field %s: %w", field.Name, err)
			}
			copy(*v, decrypted)
		}
	}
	return obj, nil
}
