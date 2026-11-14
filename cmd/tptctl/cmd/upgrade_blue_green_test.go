package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpgradeBlueGreenFlags covers the flags the blue-green command accepts.
func TestUpgradeBlueGreenFlags(t *testing.T) {
	// the new name is required, and the old image flags are gone
	require.NotNil(t, UpgradeBlueGreenCmd.Flags().Lookup("new-control-plane-name"))
	require.Nil(t, UpgradeBlueGreenCmd.Flags().Lookup("name"))
	require.Nil(t, UpgradeBlueGreenCmd.Flags().Lookup("namespace"))
	require.Nil(t, UpgradeBlueGreenCmd.Flags().Lookup("image-tag"))
	require.Nil(t, UpgradeBlueGreenCmd.Flags().Lookup("image-namespace"))
	require.NotNil(t, upgradeControlPlaneCmd.PersistentFlags().Lookup("control-plane-name"))
}
