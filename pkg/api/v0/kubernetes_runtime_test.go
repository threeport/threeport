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

// setupKubernetesRuntimeValidateDB returns an in-memory sqlite DB with
// the kubernetes runtime tables migrated plus the AttachedObjectReference
// table that the relationship hooks dispatcher needs to consult on every
// update. Also plants a fresh encryption key in the env so the encrypt
// hooks that fire on the instance types can read it.
func setupKubernetesRuntimeValidateDB(t *testing.T) *gorm.DB {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&KubernetesRuntimeDefinition{},
		&KubernetesRuntimeInstance{},
		&AttachedObjectReference{},
	))
	return db
}

func createValidKRD(t *testing.T, db *gorm.DB, provider string, ha bool) KubernetesRuntimeDefinition {
	t.Helper()
	existing := &KubernetesRuntimeDefinition{
		Definition:       Definition{Name: util.Ptr("test-krd")},
		InfraProvider:    util.Ptr(provider),
		HighAvailability: util.Ptr(ha),
	}
	require.NoError(t, db.Create(existing).Error)

	var loaded KubernetesRuntimeDefinition
	require.NoError(t, db.First(&loaded, *existing.ID).Error)
	return loaded
}

func createValidKRI(t *testing.T, db *gorm.DB, location string) KubernetesRuntimeInstance {
	t.Helper()
	// KubernetesRuntimeInstance needs a parent definition (FK is NOT NULL).
	parent := createValidKRD(t, db, "kind", false)
	existing := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: util.Ptr("test-kri")},
		Location:                      util.Ptr(location),
		KubernetesRuntimeDefinitionID: parent.ID,
	}
	require.NoError(t, db.Create(existing).Error)

	var loaded KubernetesRuntimeInstance
	require.NoError(t, db.First(&loaded, *existing.ID).Error)
	return loaded
}

// TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPatchInfraProvider
// pins the canonical PATCH-shape immutability check: changing
// InfraProvider via Updates() must be rejected.
func TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPatchInfraProvider(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRD(t, db, "kind", false)

	patch := &KubernetesRuntimeDefinition{InfraProvider: util.Ptr("eks")}
	err := db.Model(&loaded).Updates(patch).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "infra provider cannot be changed")
}

// TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPutInfraProvider
// is the regression guard for the GORM Statement.Changed PUT-blind
// spot. Under Save() the receiver IS the inbound row, so Statement.
// Changed reports nothing changed even when the caller flips
// InfraProvider. The fix routes through IsFieldChanged, which loads
// the committed row and diffs it against the inbound values. This
// test fails against the pre-fix code and passes against the fixed
// code.
func TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPutInfraProvider(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRD(t, db, "kind", false)

	full := loaded
	full.InfraProvider = util.Ptr("eks")
	err := db.Save(&full).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "infra provider cannot be changed")
}

// TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPatchHighAvailability
// is the PATCH-shape immutability check for HighAvailability.
func TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPatchHighAvailability(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRD(t, db, "kind", false)

	patch := &KubernetesRuntimeDefinition{HighAvailability: util.Ptr(true)}
	err := db.Model(&loaded).Updates(patch).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "high availability cannot be changed")
}

// TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPutHighAvailability
// is the PUT-shape parallel and the regression guard for the same bug
// as the InfraProvider PUT test.
func TestKubernetesRuntimeDefinition_beforeUpdate_RejectsPutHighAvailability(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRD(t, db, "kind", false)

	full := loaded
	full.HighAvailability = util.Ptr(true)
	err := db.Save(&full).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "high availability cannot be changed")
}

// TestKubernetesRuntimeDefinition_beforeUpdate_AllowsMutableFields
// confirms changes to non-immutable fields (e.g. NodeSize) pass.
func TestKubernetesRuntimeDefinition_beforeUpdate_AllowsMutableFields(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRD(t, db, "kind", false)

	patch := &KubernetesRuntimeDefinition{NodeSize: util.Ptr("Large")}
	err := db.Model(&loaded).Updates(patch).Error

	require.NoError(t, err, "non-immutable field changes should pass")
}

// TestKubernetesRuntimeInstance_beforeCreate_RejectsComponentSlot rejects a
// create that names a component workload.
func TestKubernetesRuntimeInstance_beforeCreate_RejectsComponentSlot(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	parent := createValidKRD(t, db, "kind", false)
	id := uint(9)
	cases := []KubernetesRuntimeInstance{
		{IngressComponentInstanceID: &id},
		{CertificatesComponentInstanceID: &id},
		{SecretsComponentInstanceID: &id},
		{DatabaseComponentInstanceID: &id},
	}
	for _, slot := range cases {
		instance := slot
		instance.Instance = Instance{Name: util.Ptr("test-kri")}
		instance.Location = util.Ptr("Local")
		instance.KubernetesRuntimeDefinitionID = parent.ID
		err := db.Create(&instance).Error

		require.Error(t, err)
		assert.Contains(t, err.Error(), "component slots are set when the runtime is reconciled")
	}
}

// TestKubernetesRuntimeInstance_beforeUpdate_RejectsPatchLocation
// pins the PATCH-shape Location immutability check.
func TestKubernetesRuntimeInstance_beforeUpdate_RejectsPatchLocation(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRI(t, db, "Local")

	patch := &KubernetesRuntimeInstance{Location: util.Ptr("NorthAmerica:NewYork")}
	err := db.Model(&loaded).Updates(patch).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be moved")
}

// TestKubernetesRuntimeInstance_beforeUpdate_RejectsPutLocation
// is the PUT-shape regression guard for the same bug.
func TestKubernetesRuntimeInstance_beforeUpdate_RejectsPutLocation(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRI(t, db, "Local")

	full := loaded
	full.Location = util.Ptr("NorthAmerica:NewYork")
	err := db.Save(&full).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be moved")
}

// TestKubernetesRuntimeInstance_beforeUpdate_RejectsPutLocationClear
// confirms a PUT that nulls Location (set->clear under Save) is
// rejected just like a value change. The hook used to dereference
// k.Location in the error message and would panic when the inbound
// pointer was nil; this test pins the nil-safe form.
func TestKubernetesRuntimeInstance_beforeUpdate_RejectsPutLocationClear(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)
	loaded := createValidKRI(t, db, "Local")

	full := loaded
	full.Location = nil
	err := db.Save(&full).Error

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be moved")
}
