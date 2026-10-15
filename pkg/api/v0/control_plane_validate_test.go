package v0

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gorm "gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

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
// accepts anything, so the hooks are exercised directly: both the create and
// the update path have to reject, since a tier can be set on either.
func TestTierHooksRejectAnUnknownValue(t *testing.T) {
	unknown := "banana"
	known := ControlPlaneTierProduction

	assert.Error(
		t, (&ControlPlaneDefinition{Tier: &unknown}).beforeCreate(nil),
		"a create carrying an unknown tier must be rejected",
	)
	// an update reads the values being written off the statement, so it is
	// called with one rather than a bare nil - which shape it is decides
	// which object is checked, covered by TestTierIsValidatedOnThePatchShape
	unknownUpdate := &ControlPlaneDefinition{Tier: &unknown}
	assert.Error(
		t, unknownUpdate.beforeUpdate(fullReplace(unknownUpdate)),
		"an update carrying an unknown tier must be rejected",
	)

	knownUpdate := &ControlPlaneDefinition{Tier: &known}
	assert.NoError(t, (&ControlPlaneDefinition{Tier: &known}).beforeCreate(nil))
	assert.NoError(t, knownUpdate.beforeUpdate(fullReplace(knownUpdate)))
	assert.NoError(t, (&ControlPlaneDefinition{}).beforeCreate(nil))
}

// fullReplace is the call shape a PUT reaches GORM as: one object, written
// whole, so the hook's receiver is the incoming value.
func fullReplace(object *ControlPlaneDefinition) *gorm.DB {
	return &gorm.DB{Statement: &gorm.Statement{Model: object, Dest: object}}
}

// TestTierIsValidatedOnThePatchShape covers which object the update hook reads.
//
// A PATCH reaches GORM as Model(stored).Updates(incoming), so the hook's
// receiver is the row as stored and the values being written are a separate
// object. Checking the receiver asks whether the tier already recorded is
// valid, which is the wrong question twice over: a bad tier is accepted
// because the stored one is good, and once stored, every later update is
// refused because the stored one is bad - including the one correcting it.
func TestTierIsValidatedOnThePatchShape(t *testing.T) {
	storedWith := func(tier string) *ControlPlaneDefinition {
		return &ControlPlaneDefinition{Tier: &tier}
	}
	// the call shape the generated PATCH handler uses
	patch := func(stored, incoming *ControlPlaneDefinition) *gorm.DB {
		return &gorm.DB{Statement: &gorm.Statement{Model: stored, Dest: incoming}}
	}

	t.Run("an invalid tier is rejected even when the stored one is valid", func(t *testing.T) {
		stored := storedWith(ControlPlaneTierProduction)
		err := stored.beforeUpdate(patch(stored, &ControlPlaneDefinition{Tier: util.Ptr("banana")}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "banana")
	})

	t.Run("a correcting update is accepted even when the stored one is invalid", func(t *testing.T) {
		stored := storedWith("banana")
		assert.NoError(
			t, stored.beforeUpdate(patch(stored, &ControlPlaneDefinition{Tier: util.Ptr(ControlPlaneTierProduction)})),
			"an update that fixes a bad stored tier must not be refused by it",
		)
	})

	t.Run("a patch that does not mention the tier is left alone", func(t *testing.T) {
		stored := storedWith("banana")
		assert.NoError(
			t, stored.beforeUpdate(patch(stored, &ControlPlaneDefinition{AuthEnabled: util.Ptr(true)})),
			"a patch setting something else is not held to a tier it is not writing",
		)
	})

	t.Run("a full replace reads the incoming object, which is the receiver", func(t *testing.T) {
		incoming := storedWith("banana")
		assert.Error(
			t, incoming.beforeUpdate(fullReplace(incoming)),
			"under PUT the receiver is what is being written",
		)
	})
}
