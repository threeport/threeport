package cmd

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config_v0 "github.com/threeport/threeport/pkg/config/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// captureStdout runs the listing and returns what it printed.
//
// The output functions write to os.Stdout through a tabwriter rather than to
// an io.Writer the caller supplies, so the only way to read what a user would
// see is to stand in for os.Stdout.
func captureStdout(t *testing.T, listing func() error) string {
	t.Helper()
	read, write, err := os.Pipe()
	require.NoError(t, err)

	original := os.Stdout
	os.Stdout = write
	listingErr := listing()
	os.Stdout = original
	require.NoError(t, write.Close())

	printed, err := io.ReadAll(read)
	require.NoError(t, err)
	require.NoError(t, listingErr)

	return string(printed)
}

// TestControlPlaneListingsShowTheTier covers the tier being answerable from
// the CLI.
//
// A tier that is stored and never shown leaves the operator with no way to
// ask what a control plane is, which is half of what persisting it was for.
// Both listings that report a definition's configuration are covered: the
// combined one carries the tier through from the definition, so printing it
// there is not a different value, only a different table.
func TestControlPlaneListingsShowTheTier(t *testing.T) {
	t.Run("get control-planes", func(t *testing.T) {
		printed := captureStdout(t, func() error {
			return outputGetv0ControlPlanesCmd(&[]config_v0.ControlPlaneConfig{{
				ControlPlane: config_v0.ControlPlaneValues{
					Name:        util.Ptr("cp-combined"),
					AuthEnabled: util.Ptr(false),
					Tier:        util.Ptr("production"),
					Genesis:     util.Ptr(true),
				},
			}})
		})
		assert.Contains(t, printed, "TIER", "the column has to be headed")
		assert.Contains(t, printed, "production", "and the recorded value printed")
	})

	t.Run("get control-plane-definitions", func(t *testing.T) {
		printed := captureStdout(t, func() error {
			return outputGetv0ControlPlaneDefinitionsCmd(&[]config_v0.ControlPlaneDefinitionConfig{{
				ControlPlaneDefinition: config_v0.ControlPlaneDefinitionValues{
					Name:        util.Ptr("cp-definition"),
					AuthEnabled: util.Ptr(false),
					Tier:        util.Ptr("production"),
				},
			}})
		})
		assert.Contains(t, printed, "TIER")
		assert.Contains(t, printed, "production")
	})

	t.Run("a definition with no tier recorded prints an empty cell", func(t *testing.T) {
		printed := captureStdout(t, func() error {
			return outputGetv0ControlPlaneDefinitionsCmd(&[]config_v0.ControlPlaneDefinitionConfig{{
				ControlPlaneDefinition: config_v0.ControlPlaneDefinitionValues{
					Name:        util.Ptr("cp-no-tier"),
					AuthEnabled: util.Ptr(false),
				},
			}})
		})
		assert.Contains(t, printed, "cp-no-tier", "a nil tier must not stop the row printing")
	})
}
