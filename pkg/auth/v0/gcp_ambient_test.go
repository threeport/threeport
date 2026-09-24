package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAmbientServiceAccountEmail_OffGcp covers resolving the ambient identity
// where there is none.
//
// A control plane inside GCP has no stored key to read an account from, so the
// metadata server names it instead. Everywhere else there is no metadata server,
// and the caller has to hear that rather than carry on with an empty account -
// which is how this failed before: an empty email built a resource path that
// named nothing, and GCP answered 404.
//
// This runs off GCP, which is what makes it the case worth pinning: the error
// path is the one a developer machine and CI both take.
func TestAmbientServiceAccountEmail_OffGcp(t *testing.T) {
	email, err := AmbientServiceAccountEmail(context.Background())

	require.Error(t, err)
	assert.Empty(t, email, "no account may be returned when none could be resolved")
}
