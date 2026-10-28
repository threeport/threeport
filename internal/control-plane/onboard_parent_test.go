package controlplane

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestDecryptFieldReplacesTheCiphertext covers the one thing onboarding a
// parent does to a secret.
//
// The objects a control plane was built from are copied into the child's own
// API, and the secret-bearing fields among them are stored encrypted. They
// have to arrive at the child as plaintext, because the child encrypts them
// again under its own key. A field that arrives still encrypted is a
// credential the child cannot use, and nothing about the install says so -
// it succeeds, and the child fails later against whatever that credential
// was for.
func TestDecryptFieldReplacesTheCiphertext(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	t.Run("an encrypted value comes back as plaintext", func(t *testing.T) {
		ciphertext, err := encryption.Encrypt(key, "the-secret")
		require.NoError(t, err)
		require.NotEqual(t, "the-secret", ciphertext, "the fixture has to actually be encrypted")

		field := &ciphertext
		require.NoError(t, decryptField(key, &field, "a credential"))
		assert.Equal(t, "the-secret", *field)
	})

	t.Run("a field that is not set is left alone", func(t *testing.T) {
		var field *string
		require.NoError(t, decryptField(key, &field, "a credential"))
		assert.Nil(t, field, "there is nothing to decrypt, which is not a failure")
	})

	t.Run("an empty field is left alone", func(t *testing.T) {
		field := util.Ptr("")
		require.NoError(t, decryptField(key, &field, "a credential"))
		assert.Equal(t, "", *field, "an empty string is not ciphertext")
	})

	t.Run("the wrong key is an error naming the field", func(t *testing.T) {
		ciphertext, err := encryption.Encrypt(key, "the-secret")
		require.NoError(t, err)

		otherKey, err := encryption.GenerateKey()
		require.NoError(t, err)

		field := &ciphertext
		err = decryptField(otherKey, &field, "the aws secret access key")
		require.Error(t, err, "a credential that cannot be decrypted must not be passed along")
		assert.Contains(
			t, err.Error(), "the aws secret access key",
			"the error has to say which field, since one call site decrypts several",
		)
		assert.Equal(t, ciphertext, *field, "a failed decryption leaves the field as it was")
	})

	t.Run("a value is not decrypted twice", func(t *testing.T) {
		ciphertext, err := encryption.Encrypt(key, "the-secret")
		require.NoError(t, err)

		field := &ciphertext
		require.NoError(t, decryptField(key, &field, "a credential"))
		require.Equal(t, "the-secret", *field)

		// calling it again on the now-plaintext field fails rather than
		// silently producing something else: that is the shape a retry takes
		// if a caller ever loses track of whether it has run
		assert.Error(
			t, decryptField(key, &field, "a credential"),
			"plaintext is not ciphertext, and treating it as such must not pass quietly",
		)
	})
}

// TestGetOrCreateMakesTheOnboardSequenceResumable covers what a retry does.
//
// Onboarding a parent copies fourteen objects into the child's API in
// sequence, with no record of how far it got. The control-plane controller
// retries a failed reconcile, so a failure at the eleventh runs the first ten
// again - against an API that already has them.
func TestGetOrCreateMakesTheOnboardSequenceResumable(t *testing.T) {
	type object struct{ name string }

	t.Run("an object that is not there yet is created", func(t *testing.T) {
		created := 0
		result, err := getOrCreate(
			"a thing",
			func() (*object, error) {
				return nil, fmt.Errorf("%w: a thing", client_lib.ErrObjectNotFound)
			},
			func() (*object, error) {
				created++
				return &object{name: "made"}, nil
			},
		)
		require.NoError(t, err)
		assert.Equal(t, "made", result.name)
		assert.Equal(t, 1, created)
	})

	t.Run("an object that is already there is not created again", func(t *testing.T) {
		created := 0
		result, err := getOrCreate(
			"a thing",
			func() (*object, error) { return &object{name: "from the first attempt"}, nil },
			func() (*object, error) {
				created++
				return nil, errors.New("the api would have refused this as a conflict")
			},
		)
		require.NoError(t, err, "the retry has to accept what its earlier attempt left behind")
		assert.Equal(t, "from the first attempt", result.name)
		assert.Zero(t, created, "creating it again is the conflict this exists to avoid")
	})

	t.Run("a lookup that fails for another reason does not become a create", func(t *testing.T) {
		created := 0
		_, err := getOrCreate(
			"a thing",
			func() (*object, error) { return nil, errors.New("connection refused") },
			func() (*object, error) {
				created++
				return &object{}, nil
			},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Zero(
			t, created,
			"an unreachable api is not an empty one; reading it that way turns an outage into writes",
		)
	})

	t.Run("a failed create says what it was creating", func(t *testing.T) {
		_, err := getOrCreate(
			"the aws provider",
			func() (*object, error) {
				return nil, fmt.Errorf("%w: a thing", client_lib.ErrObjectNotFound)
			},
			func() (*object, error) { return nil, errors.New("rejected") },
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the aws provider", "fourteen call sites share this error")
		assert.Contains(t, err.Error(), "rejected")
	})
}
