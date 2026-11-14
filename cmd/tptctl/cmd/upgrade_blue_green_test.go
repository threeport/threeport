package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpgradeBlueGreenFlags covers the flags the blue-green command accepts.
func TestUpgradeBlueGreenFlags(t *testing.T) {
	// the new name is required, and the image flags are optional
	require.NotNil(t, UpgradeBlueGreenCmd.Flags().Lookup("new-control-plane-name"))
	require.Nil(t, UpgradeBlueGreenCmd.Flags().Lookup("name"))
	require.Nil(t, UpgradeBlueGreenCmd.Flags().Lookup("namespace"))
	require.NotNil(t, UpgradeBlueGreenCmd.Flags().Lookup("image-tag"))
	require.NotNil(t, UpgradeBlueGreenCmd.Flags().Lookup("image-namespace"))
	require.NotNil(t, upgradeControlPlaneCmd.PersistentFlags().Lookup("control-plane-name"))
}
