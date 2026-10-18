package v0

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	gorm "gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupControlPlaneValidateDB gives the tier hooks a real GORM session.
//
// Every test here goes through one. The hooks read the call shape off the
// statement and the immutability check compares against the row as committed,
// so a hand-built *gorm.DB models neither - it passes while describing a
// situation that does not occur, which is how an earlier version of these
// tests gave false confidence about exactly the bug review found.
func setupControlPlaneValidateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// AttachedObjectReference comes along because the shared relationship
	// hooks read it on every write
	require.NoError(t, db.AutoMigrate(&ControlPlaneDefinition{}, &AttachedObjectReference{}))

	return db
}

// createControlPlaneDefinition inserts a definition at the given tier and
// returns it reloaded, which is what the PATCH handler passes to Updates.
func createControlPlaneDefinition(t *testing.T, db *gorm.DB, name, tier string) ControlPlaneDefinition {
	t.Helper()
	created := &ControlPlaneDefinition{
		Definition: Definition{Name: util.Ptr(name)},
		Tier:       util.Ptr(tier),
	}
	require.NoError(t, db.Create(created).Error)

	var loaded ControlPlaneDefinition
	require.NoError(t, db.First(&loaded, *created.ID).Error)

	return loaded
}

// TestValidateTierRejectsAnUnknownValue covers the API's own check on Tier.
//
// The CLI validates --tier, but that is one door into this field: anything
// reaching the API directly bypasses it. Tier gates production-only behavior -
// database topology, replica counts, backups - so an unrecognised value does
// not fail anywhere, it quietly takes the development branch in every one of
// them. A control plane would then read as configured while behaving as if it
// were not, which is worse than a rejected create.
func TestValidateTierRejectsAnUnknownValue(t *testing.T) {
	for _, tier := range ControlPlaneTiers {
		assert.NoError(t, validateTier(&tier), "%s is a tier", tier)
	}

	assert.NoError(t, validateTier(nil), "an unset tier takes the column default")

	for _, unknown := range []string{"", "Production", "prod", "banana"} {
		err := validateTier(&unknown)
		require.Error(t, err, "%q is not a tier", unknown)
		assert.Contains(t, err.Error(), "must be one of")
		var httpError *util.HttpError
		require.ErrorAs(t, err, &httpError)
		assert.Equal(
			t, http.StatusBadRequest, httpError.StatusCode,
			"a rejected value is the caller's error, not the server's",
		)
	}
}

// TestTierHooksRejectAnUnknownValue covers the wiring rather than the check.
//
// A validation function nothing calls passes its own test while the API still
// accepts anything, so the value is pushed through the database: both the
// create and the update path have to reject it, since a tier can be written
// on either.
func TestTierHooksRejectAnUnknownValue(t *testing.T) {
	t.Run("a create carrying an unknown tier is rejected", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)

		err := db.Create(&ControlPlaneDefinition{
			Definition: Definition{Name: util.Ptr("cpd-bad-create")},
			Tier:       util.Ptr("banana"),
		}).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be one of")
	})

	t.Run("a create at a known tier is accepted", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)

		for _, tier := range ControlPlaneTiers {
			assert.NoError(t, db.Create(&ControlPlaneDefinition{
				Definition: Definition{Name: util.Ptr("cpd-" + tier)},
				Tier:       util.Ptr(tier),
			}).Error, "%s is a tier", tier)
		}
	})

	t.Run("a create with no tier takes the column default", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)

		assert.NoError(t, db.Create(&ControlPlaneDefinition{
			Definition: Definition{Name: util.Ptr("cpd-no-tier")},
		}).Error)
	})
}

// TestTierIsValidatedOnThePatchShape covers which object the update hook reads.
//
// A PATCH reaches GORM as Model(stored).Updates(incoming), so the hook's
// receiver is the row as stored and the values being written are a separate
// object. Checking the receiver asks whether the tier already recorded is
// valid, which is the wrong question: the bad value sails past because the
// stored one is good.
//
// Immutability rejects this request either way, so the message is what
// separates the two readings - reading the receiver reports a tier that
// cannot be changed, when the real complaint is that 'banana' is not a tier.
func TestTierIsValidatedOnThePatchShape(t *testing.T) {
	db := setupControlPlaneValidateDB(t)
	stored := createControlPlaneDefinition(t, db, "cpd-patch-invalid", ControlPlaneTierDevelopment)

	err := db.Model(&stored).Updates(&ControlPlaneDefinition{Tier: util.Ptr("banana")}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of", "the value being written is what is checked")
	assert.Contains(t, err.Error(), "banana")
}

// TestTierCannotBeChangedAfterCreation covers the tier's immutability.
//
// The tier is read at install time to decide replica counts, database
// topology and backups. Rewriting the field afterwards reconfigures none of
// that, so a definition whose tier had been changed would describe a control
// plane that was never built that way - and the field exists precisely to be
// trusted by the things that read it.
//
// Both call shapes are covered because they fail differently: under PATCH the
// incoming values are a separate object, under PUT they are the row itself.
func TestTierCannotBeChangedAfterCreation(t *testing.T) {
	t.Run("a patch that moves the tier is rejected", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)
		stored := createControlPlaneDefinition(t, db, "cpd-patch-change", ControlPlaneTierDevelopment)

		err := db.Model(&stored).Updates(&ControlPlaneDefinition{
			Tier: util.Ptr(ControlPlaneTierProduction),
		}).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be changed after creation")
	})

	t.Run("a replace that moves the tier is rejected", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)
		stored := createControlPlaneDefinition(t, db, "cpd-put-change", ControlPlaneTierDevelopment)
		stored.Tier = util.Ptr(ControlPlaneTierProduction)

		err := db.Save(&stored).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be changed after creation")
	})

	t.Run("a replace carrying the recorded tier is accepted", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)
		stored := createControlPlaneDefinition(t, db, "cpd-put-same", ControlPlaneTierProduction)
		stored.AuthEnabled = util.Ptr(true)

		assert.NoError(
			t, db.Save(&stored).Error,
			"the config layer replaces with the tier carried forward; that must keep working",
		)
	})

	t.Run("a patch restating the same tier is accepted", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)
		stored := createControlPlaneDefinition(t, db, "cpd-patch-same", ControlPlaneTierProduction)

		assert.NoError(t, db.Model(&stored).Updates(&ControlPlaneDefinition{
			Tier: util.Ptr(ControlPlaneTierProduction),
		}).Error, "writing the value it already holds changes nothing")
	})

	t.Run("the reconciler's flag write is left alone", func(t *testing.T) {
		db := setupControlPlaneValidateDB(t)
		stored := createControlPlaneDefinition(t, db, "cpd-reconciled", ControlPlaneTierProduction)

		// the shape control_plane_definition_reconciler_gen.go uses to mark
		// an object reconciled: an update that mentions no tier at all
		assert.NoError(t, db.Model(&stored).Updates(&ControlPlaneDefinition{
			Reconciliation: Reconciliation{Reconciled: util.Ptr(true)},
		}).Error, "a patch that does not write the tier is not changing it")
	})
}
