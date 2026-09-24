package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGcpServiceAccountEmail covers reading the account a credential
// authenticates as, which is what a managed cluster has to authorize.
func TestGcpServiceAccountEmail(t *testing.T) {
	t.Run("returns the client email", func(t *testing.T) {
		email, err := GcpServiceAccountEmail(
			`{"type":"service_account","project_id":"a-project","client_email":"threeport@a-project.iam.gserviceaccount.com"}`,
		)
		require.NoError(t, err)
		assert.Equal(t, "threeport@a-project.iam.gserviceaccount.com", email)
	})

	// authorizing an empty subject would bind nothing while looking like it
	// bound something
	t.Run("credentials with no client email are an error", func(t *testing.T) {
		_, err := GcpServiceAccountEmail(`{"type":"service_account","project_id":"a-project"}`)
		require.Error(t, err)
	})

	t.Run("malformed credentials are an error", func(t *testing.T) {
		_, err := GcpServiceAccountEmail(`{"type":`)
		require.Error(t, err)
	})
}
