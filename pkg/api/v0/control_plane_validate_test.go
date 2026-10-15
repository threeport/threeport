package v0

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Error(
		t, (&ControlPlaneDefinition{Tier: &unknown}).beforeUpdate(nil),
		"an update carrying an unknown tier must be rejected",
	)

	assert.NoError(t, (&ControlPlaneDefinition{Tier: &known}).beforeCreate(nil))
	assert.NoError(t, (&ControlPlaneDefinition{Tier: &known}).beforeUpdate(nil))
	assert.NoError(t, (&ControlPlaneDefinition{}).beforeCreate(nil))
}
