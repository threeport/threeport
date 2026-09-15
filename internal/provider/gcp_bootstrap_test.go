package provider

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestThreeportServiceAccountRoles_AssertsInstanceAdminRole asserts
// threeportServiceAccountRoles includes roles/compute.instanceAdmin.v1.
func TestThreeportServiceAccountRoles_AssertsInstanceAdminRole(t *testing.T) {
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.instanceAdmin.v1")
}

// TestWrapCreatedServiceAccountError_PreservesOpErr covers rollback success
// and a failed delete of the account this create added.
func TestWrapCreatedServiceAccountError_PreservesOpErr(t *testing.T) {
	opErr := errors.New("failed to grant IAM roles")

	// return the operation error when rollback succeeds
	got := wrapCreatedServiceAccountError(opErr, nil, "sa@example.iam.gserviceaccount.com")
	require.Equal(t, opErr, got)

	// append the delete failure when rollback fails
	got = wrapCreatedServiceAccountError(opErr, errors.New("delete denied"), "sa@example.iam.gserviceaccount.com")
	require.ErrorIs(t, got, opErr)
	assert.Contains(t, got.Error(), "failed to delete newly created service account sa@example.iam.gserviceaccount.com")
	assert.Contains(t, got.Error(), "delete denied")
}
