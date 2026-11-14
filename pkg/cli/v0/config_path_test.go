package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetermineThreeportConfigPathDropsPastedTilde covers a trailing tilde on
// THREEPORT_CONFIG when the config file itself has no tilde.
func TestDetermineThreeportConfigPathDropsPastedTilde(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	t.Setenv(ThreeportConfigEnvKey, path+"~")
	assert.Equal(t, path, DetermineThreeportConfigPath(""))
	assert.Equal(t, path, DetermineThreeportConfigPath(path+"~"))

	// a file that really ends in a tilde stays that path
	tildePath := path + "~"
	require.NoError(t, os.WriteFile(tildePath, []byte("y"), 0o644))
	assert.Equal(t, tildePath, DetermineThreeportConfigPath(tildePath))
}
