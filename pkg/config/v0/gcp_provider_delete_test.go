package v0

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// the cloud account is cleaned up while the API record still exists, so a failure
// leaves something to run the delete again from
func TestDeleteGcpProviderCloudFirst_CleanupFailureKeepsRecord(t *testing.T) {
	cleanupErr := errors.New("permission denied")
	recordDeleted := false

	err := deleteGcpProviderCloudFirst(
		true,
		func() error { return cleanupErr },
		func() error { recordDeleted = true; return nil },
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, cleanupErr)
	assert.False(t, recordDeleted)
}

func TestDeleteGcpProviderCloudFirst_RecordIsDeletedLast(t *testing.T) {
	var order []string

	err := deleteGcpProviderCloudFirst(
		true,
		func() error { order = append(order, "cloud"); return nil },
		func() error { order = append(order, "record"); return nil },
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"cloud", "record"}, order)
}

func TestDeleteGcpProviderCloudFirst_NoCleanupRequested(t *testing.T) {
	recordDeleted := false

	err := deleteGcpProviderCloudFirst(
		false,
		func() error { t.Fatal("cleanup must not run"); return nil },
		func() error { recordDeleted = true; return nil },
	)

	require.NoError(t, err)
	assert.True(t, recordDeleted)
}

// the API refuses a delete while other objects use the provider; the message has
// to say the account is already gone
func TestDeleteGcpProviderCloudFirst_BlockedRecordDeleteSaysAccountIsGone(t *testing.T) {
	blocked := errors.New("blocked by attached objects")

	err := deleteGcpProviderCloudFirst(
		true,
		func() error { return nil },
		func() error { return blocked },
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, blocked)
	assert.Contains(t, err.Error(), "GCP service account was deleted")

	err = deleteGcpProviderCloudFirst(false, func() error { return nil }, func() error { return blocked })
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "service account was deleted")
}
