package v0

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
)

// TestBlueGreenClientConfig checks tptctl rewrites its config and the controller does not.
func TestBlueGreenClientConfig(t *testing.T) {
	rejectChild, updateConfig := blueGreenOptions(true)
	require.True(t, rejectChild)
	require.True(t, updateConfig)

	rejectChild, updateConfig = blueGreenOptions(false)
	require.False(t, rejectChild)
	require.False(t, updateConfig)
}

// TestDirectChildConnectionRefused checks a connection to a child returns the parent error.
func TestDirectChildConnectionRefused(t *testing.T) {
	// a connection aimed at the child itself is refused
	parent := uint(1)
	instance := &api.ControlPlaneInstance{ParentControlPlaneInstanceID: &parent}
	err := api.ChildControlPlaneRequest(instance)
	require.ErrorIs(t, err, api.ErrRequestMustBeMadeToParent)
}
