package v0

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The metadata lookup is stood in for here rather than called. Left real, a test
// asserting what happens without an ambient identity passes on a laptop and
// fails on a GCE runner, which has one.

func TestAmbientServiceAccountEmail_Resolved(t *testing.T) {
	restore := SetAmbientServiceAccountEmailForTest(func(context.Context) (string, error) {
		return "threeport@a-project.iam.gserviceaccount.com", nil
	})
	defer restore()

	email, err := AmbientServiceAccountEmail(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "threeport@a-project.iam.gserviceaccount.com", email)
}

// the caller has to hear this rather than carry on with an empty account, which
// is how this failed before: an empty email built a resource path naming nothing
func TestAmbientServiceAccountEmail_Unresolved(t *testing.T) {
	restore := SetAmbientServiceAccountEmailForTest(func(context.Context) (string, error) {
		return "", errors.New("no metadata server")
	})
	defer restore()

	email, err := AmbientServiceAccountEmail(context.Background())
	require.Error(t, err)
	assert.Empty(t, email)
}
