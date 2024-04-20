package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"reflect"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// GenerateKey generates a random 32-byte key for use in encryption
// (32 bytes is the maximum key size for AES-256).
func GenerateKey() (string, error) {

	// creates a new byte array the size of our key
	key := make([]byte, 32)

	// populate our key with a cryptographically secure
	// random sequence
	_, err := rand.Read(key)
	if err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}

	// encode our key in base64 and return it as a string
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt encrypts a string using AES-GCM.
func Encrypt(key, text string) (string, error) {

	// decode the key from base64
	decodedKey, err := util.Base64Decode(key)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %w", err)
	}

	// creates a new AES cipher using our 32 byte key
	c, err := aes.NewCipher([]byte(decodedKey))
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// configure Galois/Counter mode,
	// which provides both authentication and encryption
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return "", fmt.Errorf("failed to configure galois counter mode: %w", err)
	}

	// creates a new byte array the size of the nonce
	nonce := make([]byte, gcm.NonceSize())

	// populate nonce with a random and unique value, which is
	// required for GCM to be secure
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// encode our nonce and ciphertext in base64 and return
	// them as a string
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(text), nil)), nil

}

// Decrypt decrypts a string using AES-GCM.
func Decrypt(key, ciphertext string) (string, error) {

	// decode the ciphertext from base64
	decodedCipherText, err := util.Base64Decode(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// decode the key from base64
	decodedKey, err := util.Base64Decode(key)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %w", err)
	}

	// create a new AES cipher using our 32 byte key
	c, err := aes.NewCipher([]byte(decodedKey))
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// configure Galois/Counter mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return "", fmt.Errorf("failed to configure galois counter mode: %w", err)
	}

	// get the nonce size
	nonceSize := gcm.NonceSize()
	if len(decodedCipherText) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// extract the nonce from the ciphertext
	nonce, decodedCipherText := decodedCipherText[:nonceSize], decodedCipherText[nonceSize:]

	// decrypt the ciphertext
	plaintext, err := gcm.Open(nil, []byte(nonce), []byte(decodedCipherText), nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	// return the plaintext as a string
	return string(plaintext), nil
}

// IsEncrypted attempts to decrypt a value.  If decryption fails it returns
// false to indicate the value provided is not encrypted.  If decryption is
// successful it returns true to indicate the value is encrypted.
func IsEncrypted(key, value string) bool {
	_, err := Decrypt(key, value)
	if err != nil {
		return false
	}

	return true
}

// IsEncryptedField takes an instance of an API object and the field name as a
// string and returns whether that field has the `encrypt:"true"` tag.
func IsEncryptedField(obj interface{}, fieldName string) (bool, error) {
	objVal := reflect.ValueOf(obj)

	// dereference if object is a pointer
	if objVal.Kind() == reflect.Ptr {
		objVal = objVal.Elem()
	}

	if objVal.Kind() == reflect.Struct && objVal.FieldByName(fieldName).IsValid() {
		fieldVal, ok := objVal.Type().FieldByName(fieldName)
		if !ok {
			return false, fmt.Errorf("field %s does not exist", fieldName)
		}
		tagValue := fieldVal.Tag.Get("encrypt")
		return tagValue == "true", nil
	}

	return false, nil
}

// RedactEncryptedValues takes an API object, redacts the value on any
// encrypted fields and returns the object with encrypted fields redacted.
func RedactEncryptedValues(obj interface{}) interface{} {
	objVal := reflect.ValueOf(obj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {
			fieldVal.Elem().SetString("[encrypted value redacted]")
		}
	}

	return obj
}

// DecryptValues takes and API object and the encryption key, decrypts any
// encrypted fields and returns the object with encrypted values decrypted.
func DecryptValues(obj interface{}, encryptionKey string) (interface{}, error) {
	objVal := reflect.ValueOf(obj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {
			underlyingVal, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return obj, fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}

			decryptedVal, err := Decrypt(encryptionKey, underlyingVal)
			if err != nil {
				return obj, fmt.Errorf("failed to decrypt value in field %s: %w", field.Name, err)
			}

			fieldVal.Elem().SetString(decryptedVal)
		}
	}

	return obj, nil
}

// EncryptStringMap encrypts a map of strings using AES-GCM.
func EncryptStringMap(key string, input map[string]string) (map[string]string, error) {
	encryptedMap := make(map[string]string)
	for k, v := range input {
		encryptedVal, err := Encrypt(key, v)
		if err != nil {
			return input, err
		}
		encryptedMap[k] = encryptedVal
	}
	return encryptedMap, nil
}

// DecryptStringMap encrypts a map of strings using AES-GCM.
func DecryptStringMap(key string, input map[string]string) (map[string]string, error) {
	decryptedMap := make(map[string]string)
	for k, v := range input {
		decryptedVal, err := Decrypt(key, v)
		if err != nil {
			return input, err
		}
		decryptedMap[k] = decryptedVal
	}
	return decryptedMap, nil
}
