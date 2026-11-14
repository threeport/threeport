package v0

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestMatchRestoredSourceUsesTheSourceNamespace covers a backup row found by namespace.
func TestMatchRestoredSourceUsesTheSourceNamespace(t *testing.T) {
	// the backup names the namespace it was taken from
	namespace := "primary-05"
	name := "primary-06"
	got, err := matchRestoredSource([]api.ControlPlaneInstance{
		{Name: util.Ptr("genesis-11"), Namespace: util.Ptr("genesis-11")},
		{Name: &name, Namespace: &namespace},
	}, "primary-05", "primary-06")
	require.NoError(t, err)
	require.Equal(t, "primary-06", util.DerefString(got.Name))
}

// TestMatchRestoredSourceUsesTheStoredName covers a backup row found by its old name.
func TestMatchRestoredSourceUsesTheStoredName(t *testing.T) {
	// a renamed source keeps its old name in the backup
	name := "primary-05"
	got, err := matchRestoredSource([]api.ControlPlaneInstance{
		{Name: &name, Namespace: util.Ptr("somewhere-else")},
	}, "primary-05", "primary-06")
	require.NoError(t, err)
	require.Equal(t, "primary-05", util.DerefString(got.Name))
}
