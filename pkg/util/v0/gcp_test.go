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

	// an authorized_user credential authenticates as whoever owns its refresh
	// token. Taking a client_email that happens to be alongside it would name an
	// account that makes no request - and in a cluster role binding, hand that
	// account access while leaving the caller without it
	t.Run("an authorized user credential is rejected even carrying an email", func(t *testing.T) {
		_, err := GcpServiceAccountEmail(
			`{"type":"authorized_user","refresh_token":"a-token","client_email":"someone-else@example.com"}`,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authorized_user")
	})

	t.Run("credentials with no type are rejected", func(t *testing.T) {
		_, err := GcpServiceAccountEmail(`{"client_email":"threeport@a-project.iam.gserviceaccount.com"}`)
		require.Error(t, err)
	})
}
