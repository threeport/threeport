package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestModuleImagePrecedesBlueGreen checks the module image is recorded before the backup.
func TestModuleImagePrecedesBlueGreen(t *testing.T) {
	var steps []string
	err := recordModuleThenInstall(
		func() error {
			steps = append(steps, "module")
			return nil
		},
		func() error {
			steps = append(steps, "install")
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"module", "install"}, steps)
}
