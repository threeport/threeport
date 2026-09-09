package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// testTransient has one field tagged persist:"false" and one that
// must persist. The BeforeCreate hook calls straight into
// ProcessPersistFalseTaggedFields so the test exercises the
// production wiring path.
type testTransient struct {
	ID     uint `gorm:"primaryKey"`
	Name   string
	Secret *string `persist:"false"`
}

func (t *testTransient) PersistFalseFields() []PersistFalseField {
	return []PersistFalseField{{Name: "Secret"}}
}

func (t *testTransient) BeforeCreate(tx *gorm.DB) error {
	return ProcessPersistFalseTaggedFields(tx, t)
}

// BeforeUpdate mirrors the update wiring in
// ProcessCoreTaggedFieldsBeforeUpdate so the update branch can be
// exercised here. That wiring lives in pkg/api/v0, which this
// package cannot import, so the test proving the production path
// is TestSecretDefinitionDataNotStoredOnUpdate in pkg/api/v0.
func (t *testTransient) BeforeUpdate(tx *gorm.DB) error {
	return ProcessPersistFalseTaggedFields(tx, t)
}

// testPlain has no persist:"false"-tagged fields and does not
// implement PersistFalseFieldProvider, so the hook must be a
// no-op for it.
type testPlain struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	Value string
}

func (t *testPlain) BeforeCreate(tx *gorm.DB) error {
	return ProcessPersistFalseTaggedFields(tx, t)
}

// setupPersistTestDB stands up an in-memory sqlite db with both
// test types migrated.
func setupPersistTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testTransient{}, &testPlain{}))
	return db
}

// testStringPtr returns a fresh *string so an assertion-time
// local can't be reached through the pointer chain after the
// hook nulls the column.
func testStringPtr(s string) *string { return &s }

// TestPersistFalseHookNullsTaggedFieldOnCreate pins the core
// contract: the inbound persist:"false" field is dropped before
// the row hits the database, while sibling fields persist.
func TestPersistFalseHookNullsTaggedFieldOnCreate(t *testing.T) {
	db := setupPersistTestDB(t)

	obj := &testTransient{Name: "keep-me", Secret: testStringPtr("drop-me")}
	require.NoError(t, db.Create(obj).Error)

	var stored testTransient
	require.NoError(t, db.First(&stored, obj.ID).Error)
	assert.Equal(t, "keep-me", stored.Name, "untagged field must persist")
	assert.Nil(t, stored.Secret, "persist:false field must be nil in the row")
}

// TestPersistFalseHookLeavesUntaggedRowAlone pins the no-op
// path: a type without persist:"false" fields persists every
// field as written.
func TestPersistFalseHookLeavesUntaggedRowAlone(t *testing.T) {
	db := setupPersistTestDB(t)

	obj := &testPlain{Name: "alpha", Value: "beta"}
	require.NoError(t, db.Create(obj).Error)

	var stored testPlain
	require.NoError(t, db.First(&stored, obj.ID).Error)
	assert.Equal(t, "alpha", stored.Name)
	assert.Equal(t, "beta", stored.Value, "non-provider type must keep every field")
}

// TestPersistFalseHookNullsRegardlessOfInboundValue confirms a
// non-nil inbound value still gets dropped: the hook nulls the
// column unconditionally rather than inspecting the value.
func TestPersistFalseHookNullsRegardlessOfInboundValue(t *testing.T) {
	db := setupPersistTestDB(t)

	for _, name := range []string{"with-nil-secret", "with-set-secret"} {
		obj := &testTransient{Name: name}
		if name == "with-set-secret" {
			obj.Secret = testStringPtr("never-stored")
		}
		require.NoError(t, db.Create(obj).Error)

		var stored testTransient
		require.NoError(t, db.First(&stored, obj.ID).Error)
		assert.Nil(t, stored.Secret, "%s: persist:false field must be nil", name)
	}
}

// TestPersistFalseHookWithoutTheHookTheValuePersists is the
// negative control for the update path. Running the update with
// hooks skipped lands the inbound value in the row, which is what
// makes TestPersistFalseHookUpdateLeavesInboundValueUnwritten the
// hook's doing rather than something GORM would have done anyway.
func TestPersistFalseHookWithoutTheHookTheValuePersists(t *testing.T) {
	db := setupPersistTestDB(t)

	seeded := &testTransient{Name: "seed"}
	require.NoError(t, db.Create(seeded).Error)

	var loaded testTransient
	require.NoError(t, db.First(&loaded, seeded.ID).Error)

	inbound := &testTransient{Secret: testStringPtr("new-value")}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&loaded).Updates(inbound).Error)

	var stored testTransient
	require.NoError(t, db.First(&stored, seeded.ID).Error)
	require.NotNil(t, stored.Secret, "with no hook the inbound value reaches the row")
	assert.Equal(t, "new-value", *stored.Secret)
}

// TestPersistFalseHookUpdateLeavesInboundValueUnwritten exercises
// the dest-redirect branch: on an update the hook nulls the
// inbound column, so the value the caller sent is not written and
// whatever the row already held is left alone.
func TestPersistFalseHookUpdateLeavesInboundValueUnwritten(t *testing.T) {
	db := setupPersistTestDB(t)

	seeded := &testTransient{Name: "seed", Secret: testStringPtr("seeded-value")}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(seeded).Error)

	var loaded testTransient
	require.NoError(t, db.First(&loaded, seeded.ID).Error)
	require.NotNil(t, loaded.Secret)

	inbound := &testTransient{Secret: testStringPtr("should-be-dropped")}
	require.NoError(t, db.Model(&loaded).Updates(inbound).Error)

	var stored testTransient
	require.NoError(t, db.First(&stored, seeded.ID).Error)
	assert.NotNil(t, stored.Secret, "loaded row's prior value remains untouched by the hook")
	assert.Equal(t, "seeded-value", *stored.Secret, "hook nulls only the inbound column, leaving the loaded value")
}
