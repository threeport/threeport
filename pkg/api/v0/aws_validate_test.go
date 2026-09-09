package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupAwsProviderValidateDB returns an in-memory sqlite DB with the AWS
// provider tables migrated plus the AttachedObjectReference table that the
// relationship hooks dispatcher consults on every update. Also plants a
// fresh encryption key in the env, because AccessKeyID and SecretAccessKey
// are encrypt-tagged and the encrypt hook reads the key on every write.
func setupAwsProviderValidateDB(t *testing.T) *gorm.DB {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&AwsProvider{},
		&AttachedObjectReference{},
	))
	return db
}

// createAwsProvider inserts a provider and returns it re-read from the
// database, so the caller holds the row as the update hooks will see it.
// Pass nil for both keys to get a provider carrying no credentials.
func createAwsProvider(t *testing.T, db *gorm.DB, accessKeyID, secretAccessKey *string) AwsProvider {
	t.Helper()
	created := &AwsProvider{
		Name:            util.Ptr("test-aws-provider"),
		AccountID:       util.Ptr("555555555555"),
		DefaultRegion:   util.Ptr("us-east-1"),
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}
	require.NoError(t, db.Create(created).Error)

	var loaded AwsProvider
	require.NoError(t, db.First(&loaded, *created.ID).Error)
	return loaded
}

// TestAwsProviderPatchRejectsUnpairedAccessKey covers the patch path: a
// provider carrying neither credential gets one of the pair added, which
// leaves the row half configured.
func TestAwsProviderPatchRejectsUnpairedAccessKey(t *testing.T) {
	db := setupAwsProviderValidateDB(t)
	loaded := createAwsProvider(t, db, nil, nil)

	patch := &AwsProvider{AccessKeyID: util.Ptr("AKIAEXAMPLE")}
	err := db.Model(&loaded).Updates(patch).Error

	require.Error(t, err, "adding only the access key id must be rejected")
	assert.ErrorContains(t, err, awsAccessKeyPairingMessage)
}

// TestAwsProviderPatchRejectsUnpairedSecretAccessKey covers the mirror
// case, so the check is not passing for one field by accident.
func TestAwsProviderPatchRejectsUnpairedSecretAccessKey(t *testing.T) {
	db := setupAwsProviderValidateDB(t)
	loaded := createAwsProvider(t, db, nil, nil)

	patch := &AwsProvider{SecretAccessKey: util.Ptr("secret-value")}
	err := db.Model(&loaded).Updates(patch).Error

	require.Error(t, err, "adding only the secret access key must be rejected")
	assert.ErrorContains(t, err, awsAccessKeyPairingMessage)
}

// TestAwsProviderReplaceRejectsClearedSecretAccessKey is the regression
// test for the replace path. tx.Statement.Changed reports false under the
// Save shape, so a replace that cleared one of the pair used to reach the
// database. IsFieldChanged loads the committed row and catches it.
func TestAwsProviderReplaceRejectsClearedSecretAccessKey(t *testing.T) {
	db := setupAwsProviderValidateDB(t)
	loaded := createAwsProvider(t, db, util.Ptr("AKIAEXAMPLE"), util.Ptr("secret-value"))

	// a full replace that drops the secret while keeping the access key
	loaded.SecretAccessKey = nil
	err := db.Save(&loaded).Error

	require.Error(t, err, "clearing one key on a replace must be rejected")
	assert.ErrorContains(t, err, awsAccessKeyPairingMessage)
}

// TestAwsProviderReplaceAcceptsRotatedAccessKey confirms the replace path
// still admits a valid change: rotating the access key while the secret
// stays in place leaves both halves set.
func TestAwsProviderReplaceAcceptsRotatedAccessKey(t *testing.T) {
	db := setupAwsProviderValidateDB(t)
	loaded := createAwsProvider(t, db, util.Ptr("AKIAEXAMPLE"), util.Ptr("secret-value"))

	loaded.AccessKeyID = util.Ptr("AKIAROTATED")
	assert.NoError(t, db.Save(&loaded).Error, "rotating one key with the other set must be allowed")
}

// TestAwsProviderPatchAcceptsBothKeysTogether confirms a patch supplying
// the whole pair passes.
func TestAwsProviderPatchAcceptsBothKeysTogether(t *testing.T) {
	db := setupAwsProviderValidateDB(t)
	loaded := createAwsProvider(t, db, nil, nil)

	patch := &AwsProvider{
		AccessKeyID:     util.Ptr("AKIAEXAMPLE"),
		SecretAccessKey: util.Ptr("secret-value"),
	}
	assert.NoError(t, db.Model(&loaded).Updates(patch).Error, "supplying both keys must be allowed")
}

// TestAwsProviderUpdateSkipsWhenNeitherKeyChanges pins the early return.
// An update touching no credential field must not be rejected, whatever
// the stored pair looks like.
func TestAwsProviderUpdateSkipsWhenNeitherKeyChanges(t *testing.T) {
	db := setupAwsProviderValidateDB(t)
	loaded := createAwsProvider(t, db, nil, nil)

	patch := &AwsProvider{DefaultRegion: util.Ptr("us-west-2")}
	require.NoError(t, db.Model(&loaded).Updates(patch).Error)

	var stored AwsProvider
	require.NoError(t, db.First(&stored, *loaded.ID).Error)
	assert.Equal(t, "us-west-2", *stored.DefaultRegion, "the unrelated field must persist")
}
