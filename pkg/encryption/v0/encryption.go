package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// GenerateKey generates a random 64-byte key for use in encryption.
func GenerateKey() (string, error) {
	key := make([]byte, 64)

	_, err := rand.Read(key)
	if err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt encrypts a string using AES-GCM.
func Encrypt(key, text string) {

	c, err := aes.NewCipher([]byte(key))
	if err != nil {
		fmt.Println(err)
	}

	// gcm or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// - https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		fmt.Println(err)
	}

	// creates a new byte array the size of the nonce
	// which must be passed to Seal
	nonce := make([]byte, gcm.NonceSize())

	// populates our nonce with a cryptographically secure
	// random sequence
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		fmt.Println(err)
	}

	// here we encrypt our text using the Seal function
	// Seal encrypts and authenticates plaintext, authenticates the
	// additional data and appends the result to dst, returning the updated
	// slice. The nonce must be NonceSize() bytes long and unique for all
	// time, for a given key.
	fmt.Println(gcm.Seal(nonce, nonce, []byte(text), nil))

}

// Decrypt decrypts a string using AES-GCM.
func Decrypt(key, ciphertext string) {

	c, err := aes.NewCipher([]byte(key))
	if err != nil {
		fmt.Println(err)
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		fmt.Println(err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		fmt.Println(err)
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, []byte(nonce), []byte(ciphertext), nil)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(plaintext))

}
