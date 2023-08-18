package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/threeport/threeport/internal/util"
)

// GenerateKey generates a random 64-byte key for use in encryption.
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
		fmt.Println(err)
	}

	// configure Galois/Counter mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		fmt.Println(err)
	}

	// creates a new byte array the size of the nonce
	nonce := make([]byte, gcm.NonceSize())

	// populates our nonce with a cryptographically secure
	// random sequence
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		fmt.Println(err)
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
		return "", fmt.Errorf("failed to create gcm: %w", err)
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
		return "", fmt.Errorf("failed to open gcm: %w", err)
	}

	// return the plaintext as a string
	return string(plaintext), nil
}
