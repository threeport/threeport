package controlplane

import (
	"fmt"

	"github.com/threeport/threeport/pkg/encryption/v0"
)

// decryptField replaces an encrypted field with its plaintext, in place.
//
// Onboarding a parent copies the objects a control plane was built from into
// the child's own API, and the secret-bearing fields among them are stored
// encrypted. They have to arrive at the child as plaintext, because the child
// encrypts them again under its own key.
//
// Written once because it was written seven times: the same seven lines
// around CertificateKey, ConnectionToken, the AWS access key and secret, and
// the OCI private key. Seven copies of a decision about secrets is seven
// places for one of them to be slightly different - an omitted empty check, a
// field assigned before the error is examined - and the failure that produces
// is a credential that reaches the child unusable, or does not reach it at
// all, long after the install reported success.
//
// A field that is absent or empty is left alone rather than treated as
// ciphertext. There is nothing to decrypt, and an empty string is not a
// failed decryption.
func decryptField(encryptionKey string, field **string, description string) error {
	if field == nil || *field == nil || **field == "" {
		return nil
	}

	decrypted, err := encryption.Decrypt(encryptionKey, **field)
	if err != nil {
		return fmt.Errorf("failed to decrypt %s: %w", description, err)
	}
	*field = &decrypted

	return nil
}
