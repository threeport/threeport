package controlplane

import (
	"errors"
	"fmt"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
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

// getOrCreate returns the object that already exists, or creates it.
//
// Onboarding a parent copies fourteen objects into the child's API, one
// after another, with no record of how far it got. The control-plane
// controller retries a failed reconcile, so a failure at the eleventh runs
// the first ten again - and they already exist. What that produces depends
// on the object: a duplicate row where nothing stops one, or a conflict that
// fails the reconcile at the same place every time it is retried, which is a
// bootstrap that cannot finish without someone editing the database by hand.
//
// Treating "already there" as success makes the sequence resumable without
// recording anything. The objects are looked up the way they are named, so a
// retry recognises its own earlier work.
//
// Only a missing object leads to a create. Any other failure of the lookup is
// returned as it is: a child API that is unreachable, or refusing requests,
// must not be mistaken for one that has nothing in it - that reading would
// turn an outage into fourteen creates against an API that cannot take them.
func getOrCreate[T any](
	description string,
	get func() (*T, error),
	create func() (*T, error),
) (*T, error) {
	existing, err := get()
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look for an existing %s: %w", description, err)
	}

	created, err := create()
	if err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", description, err)
	}

	return created, nil
}
