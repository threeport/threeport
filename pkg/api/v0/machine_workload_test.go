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

// setupMachineWorkloadValidateDB returns an in-memory sqlite DB with the
// MachineWorkloadDefinition, MachineWorkloadInstance, and
// AttachedObjectReference tables migrated, plus the encryption key set in
// the env so the encrypt hooks can run on writes.
func setupMachineWorkloadValidateDB(t *testing.T) *gorm.DB {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&MachineRuntimeDefinition{},
		&MachineRuntimeInstance{},
		&MachineWorkloadDefinition{},
		&MachineWorkloadInstance{},
		&AttachedObjectReference{},
	))
	return db
}

// createValidMWD inserts a MachineWorkloadDefinition with the given Env and
// returns the persisted row reloaded from DB. The reload mirrors the PATCH
// handler's flow, which loads existing before calling Updates.
func createValidMWD(t *testing.T, db *gorm.DB, env *[]string) MachineWorkloadDefinition {
	t.Helper()
	existing := &MachineWorkloadDefinition{
		Definition:   Definition{Name: util.Ptr("test-mwd")},
		CreateScript: util.Ptr("echo create"),
		DeleteScript: util.Ptr("echo delete"),
		Env:          env,
	}
	require.NoError(t, db.Create(existing).Error)

	var loaded MachineWorkloadDefinition
	require.NoError(t, db.First(&loaded, *existing.ID).Error)
	return loaded
}

// TestMachineWorkloadDefinition_beforeUpdate_RejectsInvalidPayloadEnv is
// the regression guard for the GORM hook receiver bug. On update paths the
// receiver carries the stale loaded row, so a validator that reads from
// the receiver would validate the OLD env and silently accept invalid
// inbound payloads. The fix redirects to Statement.Dest; this test fails
// against the broken pre-fix code and passes against the fixed code.
func TestMachineWorkloadDefinition_beforeUpdate_RejectsInvalidPayloadEnv(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	loaded := createValidMWD(t, db, &[]string{"VALID_KEY=valid_value"})

	payload := &MachineWorkloadDefinition{
		Env: &[]string{"1bad_key=foo"},
	}
	err := db.Model(&loaded).Updates(payload).Error

	require.Error(t, err, "update with invalid env must be rejected")
	assert.Contains(t, err.Error(), "invalid env key", "rejection should cite the env key")
}

// TestMachineWorkloadDefinition_beforeUpdate_AcceptsValidPayloadEnv
// confirms a valid payload Env replaces a valid existing Env successfully.
func TestMachineWorkloadDefinition_beforeUpdate_AcceptsValidPayloadEnv(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	loaded := createValidMWD(t, db, &[]string{"OLD_KEY=old_val"})

	payload := &MachineWorkloadDefinition{
		Env: &[]string{"NEW_KEY=new_val"},
	}
	require.NoError(t, db.Model(&loaded).Updates(payload).Error)
}

// TestMachineWorkloadDefinition_beforeUpdate_SkipsValidationWhenEnvUnchanged
// confirms the tx.Statement.Changed("Env") gate skips validation when the
// payload doesn't touch Env. Without the gate, validation would run
// against the stale loaded ciphertext, which usually passes the regex by
// accident but is a fragile path.
func TestMachineWorkloadDefinition_beforeUpdate_SkipsValidationWhenEnvUnchanged(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	loaded := createValidMWD(t, db, &[]string{"OLD_KEY=old_val"})

	payload := &MachineWorkloadDefinition{
		Shell: util.Ptr("/bin/sh"),
	}
	require.NoError(t, db.Model(&loaded).Updates(payload).Error)
}

// TestMachineWorkloadInstance_beforeUpdate_RejectsInvalidPayloadEnv
// mirrors the Definition regression test for the Instance side. The two
// beforeUpdate methods share the same Statement.Dest redirect pattern,
// so the same regression applies on both sides.
func TestMachineWorkloadInstance_beforeUpdate_RejectsInvalidPayloadEnv(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	def := createValidMWD(t, db, nil)
	mri := &MachineRuntimeInstance{
		Instance:    Instance{Name: util.Ptr("test-mri")},
		Hostname:    util.Ptr("host.example"),
		SSHUser:     util.Ptr("user"),
		SSHPassword: util.Ptr("password"),
	}
	require.NoError(t, db.Create(mri).Error)

	existing := &MachineWorkloadInstance{
		Instance:                    Instance{Name: util.Ptr("test-mwi")},
		MachineRuntimeInstanceID:    mri.ID,
		MachineWorkloadDefinitionID: def.ID,
		Env:                         &[]string{"VALID_KEY=valid_value"},
	}
	require.NoError(t, db.Create(existing).Error)

	var loaded MachineWorkloadInstance
	require.NoError(t, db.First(&loaded, *existing.ID).Error)

	payload := &MachineWorkloadInstance{
		Env: &[]string{"2bad_key=foo"},
	}
	err := db.Model(&loaded).Updates(payload).Error

	require.Error(t, err, "update with invalid env must be rejected")
	assert.Contains(t, err.Error(), "invalid env key")
}

// TestMachineWorkloadDefinition_ShellDefault_AppliedOnInsert pins the
// Shell column default ("/bin/bash"). Downstream controllers read
// Shell after create; a silent regression on the default tag (typo,
// accidental drop, reorder) would shift the runtime contract for any
// workload that omits Shell.
func TestMachineWorkloadDefinition_ShellDefault_AppliedOnInsert(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	// createValidMWD leaves Shell nil, so insert exercises the
	// column default applied by AutoMigrate-managed sqlite.
	loaded := createValidMWD(t, db, &[]string{"VALID_KEY=valid_value"})

	require.NotNil(t, loaded.Shell, "Shell should be populated from the column default")
	assert.Equal(t, "/bin/bash", *loaded.Shell)
}
