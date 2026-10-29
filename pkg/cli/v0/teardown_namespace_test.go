package v0

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestTeardownLeavesANamespaceItDidNotCreate covers the interaction between
// tearing down by default and reapplying onto a running control plane.
//
// Tearing down uninstalls the control plane components, and that deletes the
// whole namespace. On a reapply - which is what --control-plane-only does
// against an existing control plane - the namespace holds a running API, a
// database and its data. Removing it because an install failed partway
// through would destroy all of it, and nobody asked for that by not passing a
// flag.
func TestTeardownLeavesANamespaceItDidNotCreate(t *testing.T) {
	teardown := true

	uninstallerFor := func(preexisted bool) *Uninstaller {
		installer := &threeport.ControlPlaneInstaller{}
		installer.Opts.Namespace = "threeport-control-plane"
		installer.Opts.NamespacePreexisted = preexisted
		// --control-plane-only is the path this guard is about, and it also
		// stops the teardown before it reaches cloud infrastructure these
		// tests have none of
		installer.Opts.ControlPlaneOnly = true

		return &Uninstaller{
			cpi:               installer,
			teardownOnFailure: &teardown,
		}
	}

	t.Run("a namespace that was already there is left alone", func(t *testing.T) {
		createErr := errors.New("the install failed partway through")

		err := uninstallerFor(true).cleanOnCreateError("", createErr)
		require.Error(t, err)
		assert.ErrorIs(t, err, createErr, "the original failure is still what happened")
		assert.Contains(
			t, err.Error(), "already present",
			"the operator has to be told the namespace was kept, not left to wonder",
		)
		assert.Contains(t, err.Error(), "threeport-control-plane")
	})

	t.Run("a namespace this install created is not spared", func(t *testing.T) {
		createErr := errors.New("the install failed partway through")

		err := uninstallerFor(false).cleanOnCreateError("", createErr)
		require.Error(t, err)
		assert.NotContains(
			t, err.Error(), "already present",
			"a namespace this install created is exactly what teardown is for",
		)
	})

	t.Run("teardown off still returns without touching anything", func(t *testing.T) {
		off := false
		uninstaller := uninstallerFor(false)
		uninstaller.teardownOnFailure = &off
		createErr := errors.New("the install failed partway through")

		assert.ErrorIs(t, uninstaller.cleanOnCreateError("", createErr), createErr)
	})
}
