package v0

import (
	"testing"

	echo "github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModuleRouter_Route covers the accessor the module api route handler uses
// to snapshot the router before a transaction that may not commit. The
// AfterCreate hook installs a route from inside the transaction, so the
// handler has to be able to put the router back the way it found it.
func TestModuleRouter_Route(t *testing.T) {
	router := ModuleRouter{}

	t.Run("an unregistered path reports absent", func(t *testing.T) {
		handler, ok := router.Route("/v0/absent")

		assert.False(t, ok)
		assert.Nil(t, handler)
	})

	t.Run("a registered path returns its handler", func(t *testing.T) {
		want := "reached"
		var got string
		router.AddRoute("/v0/present", func(c echo.Context) error {
			got = want
			return nil
		})

		handler, ok := router.Route("/v0/present")

		require.True(t, ok)
		require.NotNil(t, handler)
		require.NoError(t, handler(nil), "the handler that comes back must be the one registered")
		assert.Equal(t, want, got)
	})

	t.Run("a removed path reports absent again", func(t *testing.T) {
		router.AddRoute("/v0/transient", func(c echo.Context) error { return nil })
		router.RemoveRoute("/v0/transient")

		_, ok := router.Route("/v0/transient")

		assert.False(t, ok)
	})
}

// TestModuleRouter_RestoreAfterFailedTransaction walks the sequence the
// handler performs when a create does not commit: snapshot, let the hook
// install a route, then restore. A sibling route sharing the path must survive,
// because Path carries no unique index and a blanket RemoveRoute would take it
// out.
func TestModuleRouter_RestoreAfterFailedTransaction(t *testing.T) {
	t.Run("a path the hook introduced is removed", func(t *testing.T) {
		router := ModuleRouter{}

		prior, hadPrior := router.Route("/v0/new")
		router.AddRoute("/v0/new", func(c echo.Context) error { return nil }) // the hook

		// the transaction failed
		if hadPrior {
			router.AddRoute("/v0/new", prior)
		} else {
			router.RemoveRoute("/v0/new")
		}

		_, ok := router.Route("/v0/new")
		assert.False(t, ok, "a route only the failed transaction added must not survive")
	})

	t.Run("a path that was already live keeps its original handler", func(t *testing.T) {
		router := ModuleRouter{}
		var reached string
		router.AddRoute("/v0/shared", func(c echo.Context) error {
			reached = "sibling"
			return nil
		})

		prior, hadPrior := router.Route("/v0/shared")
		router.AddRoute("/v0/shared", func(c echo.Context) error { // the hook overwrites
			reached = "doomed"
			return nil
		})

		// the transaction failed
		if hadPrior {
			router.AddRoute("/v0/shared", prior)
		} else {
			router.RemoveRoute("/v0/shared")
		}

		handler, ok := router.Route("/v0/shared")
		require.True(t, ok, "the sibling's route must survive")
		require.NoError(t, handler(nil))
		assert.Equal(t, "sibling", reached, "the surviving handler must be the original")
	})
}
