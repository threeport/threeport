package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadConfigIfPresent covers the config states tptctl version has to
// tell apart: no file, a readable file and a file that cannot be parsed.
func TestLoadConfigIfPresent(t *testing.T) {
	// viper holds the loaded config globally, so leave it clean for other tests
	t.Cleanup(viper.Reset)

	t.Run("a missing file is reported as absent, not as an error", func(t *testing.T) {
		viper.Reset()
		path := filepath.Join(t.TempDir(), "config.yaml")

		loaded, err := LoadConfigIfPresent(path)

		require.NoError(t, err)
		assert.False(t, loaded)
	})

	t.Run("a readable file is loaded", func(t *testing.T) {
		viper.Reset()
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("CurrentControlPlane: genesis-00\n"), 0o600))

		loaded, err := LoadConfigIfPresent(path)

		require.NoError(t, err)
		assert.True(t, loaded)
		assert.Equal(t, "genesis-00", viper.GetString("CurrentControlPlane"))
	})

	t.Run("a file that cannot be parsed is an error", func(t *testing.T) {
		viper.Reset()
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("CurrentControlPlane: [unclosed\n"), 0o600))

		loaded, err := LoadConfigIfPresent(path)

		require.Error(t, err)
		assert.False(t, loaded)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("the path comes from the environment when no flag is given", func(t *testing.T) {
		viper.Reset()
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("CurrentControlPlane: from-env\n"), 0o600))
		t.Setenv(ThreeportConfigEnvKey, path)

		loaded, err := LoadConfigIfPresent("")

		require.NoError(t, err)
		assert.True(t, loaded)
		assert.Equal(t, "from-env", viper.GetString("CurrentControlPlane"))
	})
}
