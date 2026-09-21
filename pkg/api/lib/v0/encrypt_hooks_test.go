package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/threeport/threeport/pkg/encryption/v0"
)

// testSecret carries one *string-encrypt and one *[]string-encrypt
// field - the two shapes the hook supports. Its gorm hooks call
// straight into ProcessEncryptTaggedFields so the test exercises
// the production wiring.
type testSecret struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	Token *string   `encrypt:"true"`
	Env   *[]string `gorm:"serializer:json" encrypt:"true"`
}

func (t *testSecret) EncryptedFields() []EncryptedField {
	return []EncryptedField{
		{Name: "Token", Value: t.Token},
		{Name: "Env", Value: t.Env},
	}
}

func (t *testSecret) BeforeCreate(tx *gorm.DB) error {
	return ProcessEncryptTaggedFields(tx, t)
}

func (t *testSecret) BeforeUpdate(tx *gorm.DB) error {
	return ProcessEncryptTaggedFields(tx, t)
}

// setupEncryptTestDB stands up an in-memory sqlite db and plants a
// fresh encryption key in the env so the hook can read it.
func setupEncryptTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("ENCRYPTION_KEY", key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testSecret{}))
	return db, key
}

// testTokenPtr returns a fresh *string. Going through a function
// keeps the assertion-time local out of the pointer chain so the
// hook's SetColumn (which writes back via reflect) can't mutate it
// into ciphertext before the comparison runs.
func testTokenPtr(s string) *string { return &s }

// TestEncryptHookUpdatesEncryptsFromNil covers the nil -> value
// update transition: gorm fires the hook on Statement.Model (the
// loaded row, nil here), so the dest redirect must steer the hook
// at the inbound payload instead.
func TestEncryptHookUpdatesEncryptsFromNil(t *testing.T) {
	db, key := setupEncryptTestDB(t)

	existing := &testSecret{Name: "test"}
	require.NoError(t, db.Create(existing).Error)
	require.Nil(t, existing.Token, "Token starts nil")

	var loaded testSecret
	require.NoError(t, db.First(&loaded, existing.ID).Error)

	inbound := &testSecret{Token: testTokenPtr("first-secret-value")}
	require.NoError(t, db.Model(&loaded).Updates(inbound).Error)

	var stored testSecret
	require.NoError(t, db.First(&stored, existing.ID).Error)
	require.NotNil(t, stored.Token, "Token should be persisted, not nil")
	assert.NotEqual(t, "first-secret-value", *stored.Token, "Token must not be stored as plaintext after Update")

	decrypted, err := encryption.Decrypt(key, *stored.Token)
	require.NoError(t, err, "stored Token should be valid ciphertext")
	assert.Equal(t, "first-secret-value", decrypted, "ciphertext should decrypt to the inbound plaintext")
}

// TestEncryptHookUpdatesEncryptsValueToOther verifies a value->other
// update encrypts the inbound value; without the dest redirect the
// hook would re-encrypt the loaded (stale) value.
func TestEncryptHookUpdatesEncryptsValueToOther(t *testing.T) {
	db, key := setupEncryptTestDB(t)

	existing := &testSecret{Name: "test", Token: testTokenPtr("first-secret")}
	require.NoError(t, db.Create(existing).Error)

	var loaded testSecret
	require.NoError(t, db.First(&loaded, existing.ID).Error)

	inbound := &testSecret{Token: testTokenPtr("second-secret-value")}
	require.NoError(t, db.Model(&loaded).Updates(inbound).Error)

	var stored testSecret
	require.NoError(t, db.First(&stored, existing.ID).Error)
	require.NotNil(t, stored.Token)

	decrypted, err := encryption.Decrypt(key, *stored.Token)
	require.NoError(t, err)
	assert.Equal(t, "second-secret-value", decrypted, "stored value should be ciphertext of inbound, not the prior value")
	assert.NotEqual(t, "first-secret", decrypted, "must not still have the old value")
}

// TestEncryptHookCreateEncrypts pins the create path: Model == Dest
// so the redirect is a no-op and the hook encrypts the inbound value.
func TestEncryptHookCreateEncrypts(t *testing.T) {
	db, key := setupEncryptTestDB(t)

	obj := &testSecret{Name: "test", Token: testTokenPtr("create-time-secret")}
	require.NoError(t, db.Create(obj).Error)

	var stored testSecret
	require.NoError(t, db.First(&stored, obj.ID).Error)
	require.NotNil(t, stored.Token)
	assert.NotEqual(t, "create-time-secret", *stored.Token, "Token must not be stored as plaintext after Create")

	decrypted, err := encryption.Decrypt(key, *stored.Token)
	require.NoError(t, err)
	assert.Equal(t, "create-time-secret", decrypted)
}

// TestEncryptHookUpdatesPreservesNilWhenNotChanged confirms an
// untouched encrypted field stays nil through an unrelated update.
func TestEncryptHookUpdatesPreservesNilWhenNotChanged(t *testing.T) {
	db, _ := setupEncryptTestDB(t)

	existing := &testSecret{Name: "test"}
	require.NoError(t, db.Create(existing).Error)

	var loaded testSecret
	require.NoError(t, db.First(&loaded, existing.ID).Error)

	inbound := &testSecret{Name: "renamed"}
	require.NoError(t, db.Model(&loaded).Updates(inbound).Error)

	var stored testSecret
	require.NoError(t, db.First(&stored, existing.ID).Error)
	assert.Equal(t, "renamed", stored.Name)
	assert.Nil(t, stored.Token, "Token left nil should stay nil when update doesn't touch it")
}

// TestEncryptHookEncryptsEnvSliceValues exercises the *[]string
// KEY=VALUE path: keys stay plaintext, values round-trip through
// encrypt/decrypt, across both create and update.
func TestEncryptHookEncryptsEnvSliceValues(t *testing.T) {
	db, key := setupEncryptTestDB(t)

	plain := []string{"DB_PASSWORD=hunter2", "API_KEY=abc-123"}
	plainCopy := append([]string{}, plain...)
	obj := &testSecret{Name: "test", Env: &plainCopy}
	require.NoError(t, db.Create(obj).Error)

	var stored testSecret
	require.NoError(t, db.First(&stored, obj.ID).Error)
	require.NotNil(t, stored.Env)
	require.Len(t, *stored.Env, len(plain))
	for i, entry := range *stored.Env {
		wantKey, wantValue, _ := strings.Cut(plain[i], "=")
		gotKey, gotCipher, ok := strings.Cut(entry, "=")
		require.True(t, ok, "stored entry %d should still be KEY=VALUE shape", i)
		assert.Equal(t, wantKey, gotKey, "key portion must remain plaintext")
		assert.NotEqual(t, wantValue, gotCipher, "value portion must be ciphertext, not plaintext")

		decValue, err := encryption.Decrypt(key, gotCipher)
		require.NoError(t, err, "stored value portion should be valid ciphertext")
		assert.Equal(t, wantValue, decValue, "ciphertext should decrypt to the inbound plaintext")
	}

	// replace one entry and confirm the new value round-trips
	var loaded testSecret
	require.NoError(t, db.First(&loaded, obj.ID).Error)
	updateEnv := []string{"DB_PASSWORD=new-pass", "API_KEY=xyz-789"}
	inbound := &testSecret{Env: &updateEnv}
	require.NoError(t, db.Model(&loaded).Updates(inbound).Error)

	require.NoError(t, db.First(&stored, obj.ID).Error)
	require.NotNil(t, stored.Env)
	require.Len(t, *stored.Env, 2)
	_, dbPass, _ := strings.Cut((*stored.Env)[0], "=")
	dec, err := encryption.Decrypt(key, dbPass)
	require.NoError(t, err)
	assert.Equal(t, "new-pass", dec, "updated value portion should decrypt to the new plaintext")
}

// TestEncryptHookRejectsRedactedPlaceholder verifies the hook rejects
// a payload carrying the redacted-value placeholder. Accepting it
// would silently store the marker string as plaintext.
func TestEncryptHookRejectsRedactedPlaceholder(t *testing.T) {
	db, _ := setupEncryptTestDB(t)

	obj := &testSecret{Name: "test", Token: testTokenPtr(encryption.RedactedValuePlaceholder)}
	err := db.Create(obj).Error
	require.Error(t, err, "create with redacted placeholder must fail")
	assert.Contains(t, err.Error(), "redacted placeholder", "error should explain the placeholder rejection")
}

// TestRedactEncryptedValuesPreservesEnvKeys covers the KEY=VALUE
// slice shape: redaction must leave the KEY= prefix intact on each
// entry so a tptctl get -> tptctl replace round trip still hands the
// encrypt hook a well-formed KEY=VALUE payload rather than a bare
// placeholder that fails the KEY=VALUE parse.
func TestRedactEncryptedValuesPreservesEnvKeys(t *testing.T) {
	// seed a slice-shaped encrypted field with two KEY=VALUE entries
	env := []string{"DB_PASSWORD=cipher1", "API_KEY=cipher2"}
	obj := &testSecret{Env: &env}

	// redact in place; the returned interface is the same object
	_ = RedactEncryptedValues(obj)

	// each entry retains its KEY= prefix and the value is the placeholder
	require.Len(t, env, 2)
	assert.Equal(t, "DB_PASSWORD="+encryption.RedactedValuePlaceholder, env[0], "KEY= prefix must survive redaction on first entry")
	assert.Equal(t, "API_KEY="+encryption.RedactedValuePlaceholder, env[1], "KEY= prefix must survive redaction on second entry")
}

// TestRedactedEnvRoundTripsToPlaceholderError drives the whole round
// trip the KEY= prefix exists for. Redaction is what tptctl get prints
// when no encryption key is available, and sending that document back
// unedited is tptctl replace. The hook has to reject it with the
// placeholder error, which is a bad-request naming the offending entry,
// rather than the KEY=VALUE parse error. That parse error is a plain
// error rather than a *util.HttpError, so the generated handler turns it
// into a 500 and the caller never learns what to fix.
func TestRedactedEnvRoundTripsToPlaceholderError(t *testing.T) {
	db, _ := setupEncryptTestDB(t)

	// what tptctl get prints with no key available. The prefix itself is
	// asserted by TestRedactEncryptedValuesPreservesEnvKeys; this test
	// takes redaction as given and checks only what the replay does, so
	// losing the prefix surfaces here as the wrong error rather than as a
	// failed precondition.
	env := []string{"DB_PASSWORD=hunter2", "API_KEY=abc-123"}
	_ = RedactEncryptedValues(&testSecret{Name: "test", Env: &env})

	// what tptctl replace sends back: the same document, unedited
	err := db.Create(&testSecret{Name: "test", Env: &env}).Error

	require.Error(t, err, "replaying a redacted payload must be rejected")
	assert.ErrorContains(t, err, "redacted placeholder", "the rejection must be the placeholder error")
	assert.ErrorContains(t, err, "Env[0]", "the message must name which entry needs a real value")
	assert.NotContains(t, err.Error(), "not in KEY=VALUE format", "reaching the parse error means the KEY= prefix was lost")
}

// TestRedactEncryptedValuesRedactsStringField pins the *string branch
// contract: a non-nil pointer is replaced with the placeholder in
// place, a nil pointer is left alone.
func TestRedactEncryptedValuesRedactsStringField(t *testing.T) {
	// non-nil *string: underlying value is replaced by the placeholder
	obj := &testSecret{Token: testTokenPtr("plaintext-secret")}
	_ = RedactEncryptedValues(obj)
	require.NotNil(t, obj.Token, "non-nil pointer stays non-nil after redact")
	assert.Equal(t, encryption.RedactedValuePlaceholder, *obj.Token, "value must be replaced with the placeholder")

	// nil *string: pointer stays nil, hook does not deref
	nilObj := &testSecret{}
	_ = RedactEncryptedValues(nilObj)
	assert.Nil(t, nilObj.Token, "nil pointer must be left alone")
}

// TestEncryptHookSkipsAlreadyEncryptedInbound verifies the
// IsEncrypted guard: a caller resubmitting ciphertext is not
// double-encrypted, which would otherwise corrupt the field.
func TestEncryptHookSkipsAlreadyEncryptedInbound(t *testing.T) {
	db, key := setupEncryptTestDB(t)

	existing := &testSecret{Name: "test", Token: testTokenPtr("initial-secret")}
	require.NoError(t, db.Create(existing).Error)

	var afterCreate testSecret
	require.NoError(t, db.First(&afterCreate, existing.ID).Error)
	require.NotNil(t, afterCreate.Token)
	storedCipher := *afterCreate.Token

	var loaded testSecret
	require.NoError(t, db.First(&loaded, existing.ID).Error)
	inbound := &testSecret{Token: testTokenPtr(storedCipher)}
	require.NoError(t, db.Model(&loaded).Updates(inbound).Error)

	var stored testSecret
	require.NoError(t, db.First(&stored, existing.ID).Error)
	require.NotNil(t, stored.Token)
	decrypted, err := encryption.Decrypt(key, *stored.Token)
	require.NoError(t, err, "stored Token should still be single-layer ciphertext")
	assert.Equal(t, "initial-secret", decrypted, "ciphertext should still decrypt to the original plaintext")
}
