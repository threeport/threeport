package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupSecretDefinitionDB returns an in-memory sqlite DB with the secret
// tables migrated plus the AttachedObjectReference table the relationship
// hooks dispatcher consults on every update. Plants an encryption key
// because the tagged-field dispatcher runs the encrypt hook on the same
// path.
func setupSecretDefinitionDB(t *testing.T) *gorm.DB {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&SecretDefinition{},
		&AttachedObjectReference{},
	))
	return db
}

// TestSecretDefinitionDataNotStoredOnCreate pins the create half of the
// persist:"false" contract. Data reaches the controller through the
// notification payload and must not reach the row.
func TestSecretDefinitionDataNotStoredOnCreate(t *testing.T) {
	db := setupSecretDefinitionDB(t)

	created := &SecretDefinition{
		Definition: Definition{Name: util.Ptr("test-secret")},
		Data:       &datatypes.JSON{},
	}
	*created.Data = datatypes.JSON(`{"password":"hunter2"}`)
	require.NoError(t, db.Create(created).Error)

	var stored SecretDefinition
	require.NoError(t, db.First(&stored, *created.ID).Error)
	assert.Nil(t, stored.Data, "Data must not reach the row on create")
}

// TestSecretDefinitionDataNotStoredOnUpdate covers the update half of
// the same contract. Data is the only persist:"false" field in the tree
// and carries no encrypt tag, so this hook is the only thing keeping a
// secret payload out of the database on either path. Dropping
// ProcessPersistFalseTaggedFields from ProcessCoreTaggedFieldsBeforeUpdate
// fails this test.
func TestSecretDefinitionDataNotStoredOnUpdate(t *testing.T) {
	db := setupSecretDefinitionDB(t)

	created := &SecretDefinition{Definition: Definition{Name: util.Ptr("test-secret")}}
	require.NoError(t, db.Create(created).Error)

	var loaded SecretDefinition
	require.NoError(t, db.First(&loaded, *created.ID).Error)

	patch := &SecretDefinition{Data: &datatypes.JSON{}}
	*patch.Data = datatypes.JSON(`{"password":"hunter2"}`)
	require.NoError(t, db.Model(&loaded).Updates(patch).Error)

	var stored SecretDefinition
	require.NoError(t, db.First(&stored, *created.ID).Error)
	assert.Nil(t, stored.Data, "Data must not reach the row on update either")
}

// TestSecretDefinitionUpdateKeepsOtherFields confirms nulling the tagged
// column does not take its siblings with it.
func TestSecretDefinitionUpdateKeepsOtherFields(t *testing.T) {
	db := setupSecretDefinitionDB(t)

	created := &SecretDefinition{Definition: Definition{Name: util.Ptr("test-secret")}}
	require.NoError(t, db.Create(created).Error)

	var loaded SecretDefinition
	require.NoError(t, db.First(&loaded, *created.ID).Error)

	patch := &SecretDefinition{Definition: Definition{Name: util.Ptr("renamed-secret")}}
	require.NoError(t, db.Model(&loaded).Updates(patch).Error)

	var stored SecretDefinition
	require.NoError(t, db.First(&stored, *created.ID).Error)
	require.NotNil(t, stored.Name)
	assert.Equal(t, "renamed-secret", *stored.Name, "an untagged field must still persist")
	assert.Nil(t, stored.Data, "the tagged field stays nil")
}
