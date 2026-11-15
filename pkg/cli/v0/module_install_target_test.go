package v0

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestModuleInstallDelegatesOnlyFromARoot checks the root and direct install split.
func TestModuleInstallDelegatesOnlyFromARoot(t *testing.T) {
	rootID := uint(1)
	parent := uint(9)
	root := &api.ControlPlaneInstance{Common: api.Common{ID: &rootID}, Name: util.Ptr("root")}
	child := &api.ControlPlaneInstance{
		Common:                       api.Common{ID: util.Ptr(uint(2))},
		Name:                         util.Ptr("primary"),
		ParentControlPlaneInstanceID: &parent,
	}

	// a root naming another control plane records the install for the controller
	delegates, err := moduleInstallDelegates(root, "primary")
	require.NoError(t, err)
	require.True(t, delegates)

	// a root with no other name installs on itself
	delegates, err = moduleInstallDelegates(root, "")
	require.NoError(t, err)
	require.False(t, delegates)
	delegates, err = moduleInstallDelegates(root, "root")
	require.NoError(t, err)
	require.False(t, delegates)

	// a child connection is rejected, including an install of itself
	_, err = moduleInstallDelegates(child, "")
	require.ErrorIs(t, err, api.ErrRequestMustBeMadeToParent)
	_, err = moduleInstallDelegates(child, "primary")
	require.ErrorIs(t, err, api.ErrRequestMustBeMadeToParent)
	_, err = moduleInstallDelegates(child, "other")
	require.ErrorIs(t, err, api.ErrRequestMustBeMadeToParent)
}

// TestInstallThroughControllerSkipsRejectionInInstallJob asserts the controller's install job
// installs on the child it is connected to instead of being rejected.
func TestInstallThroughControllerSkipsRejectionInInstallJob(t *testing.T) {
	// mark the process as the controller's install job
	t.Setenv(installer.ModuleInstallJobEnv, "true")

	// the call returns before it contacts any API, so no client is needed
	delegated, err := InstallThroughController(nil, "", "", "", "module", "ns", "tag")
	require.NoError(t, err)
	require.False(t, delegated)
}
