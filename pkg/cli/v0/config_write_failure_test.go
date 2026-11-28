package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readOnlyThreeportConfig writes a threeport config file holding cfg, reads
// it into viper, and makes the file read-only, so the next write fails.  It
// returns the file and a func that makes it writable again.
func readOnlyThreeportConfig(t *testing.T, cfg *ThreeportConfig) (string, func()) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only file")
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o600))
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadInConfig())
	viper.Set("ControlPlanes", cfg.ControlPlanes)
	viper.Set("CurrentControlPlane", cfg.CurrentControlPlane)
	require.NoError(t, viper.WriteConfig())

	require.NoError(t, os.Chmod(path, 0o400))
	return path, func() { require.NoError(t, os.Chmod(path, 0o600)) }
}

// savedConfig reads the threeport config file at path.
func savedConfig(t *testing.T, path string) ThreeportConfig {
	t.Helper()

	saved := viper.New()
	saved.SetConfigFile(path)
	saved.SetConfigType("yaml")
	require.NoError(t, saved.ReadInConfig())
	var cfg ThreeportConfig
	require.NoError(t, saved.Unmarshal(&cfg))
	return cfg
}

// TestAFailedConfigWriteIsNotSavedByTheNextOne covers #769: a config update
// whose write fails changes neither the config in memory nor what a later
// write in the same command saves.  The next write saves the config as it
// was before the failed update.
func TestAFailedConfigWriteIsNotSavedByTheNextOne(t *testing.T) {
	saved := func() *ThreeportConfig {
		return &ThreeportConfig{
			CurrentControlPlane: "genesis",
			ControlPlanes: []ControlPlane{
				{Name: "genesis", APIServer: "genesis.example:443", Genesis: true},
				{Name: "child", APIServer: "child.example:443", CACert: "ca-child"},
			},
		}
	}

	for name, update := range map[string]*ControlPlane{
		"an entry refreshed": {Name: "child", APIServer: "other.example:443", CACert: "ca-other"},
		"an entry added":     {Name: "new", APIServer: "new.example:443"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := saved()
			path, writable := readOnlyThreeportConfig(t, cfg)

			require.Error(t, UpdateThreeportConfig(cfg, update))
			assert.Equal(t, saved().ControlPlanes, cfg.ControlPlanes, "the config in memory is as it was")

			// the next write in the same command succeeds
			writable()
			cfg.SetCurrentControlPlane("child")

			onDisk := savedConfig(t, path)
			assert.Equal(t, "child", onDisk.CurrentControlPlane)
			require.Len(t, onDisk.ControlPlanes, 2, "an entry the failed write added is not saved")
			assert.Equal(t, "child.example:443", onDisk.ControlPlanes[1].APIServer, "nor the address it refreshed")
			assert.Equal(t, "ca-child", onDisk.ControlPlanes[1].CACert)
		})
	}

	t.Run("a write that succeeds", func(t *testing.T) {
		cfg := saved()
		path, writable := readOnlyThreeportConfig(t, cfg)
		writable()

		require.NoError(t, UpdateThreeportConfig(cfg, &ControlPlane{Name: "child", APIServer: "other.example:443", CACert: "ca-other"}))
		assert.Equal(t, "other.example:443", cfg.ControlPlanes[1].APIServer, "the config in memory is updated")
		onDisk := savedConfig(t, path)
		assert.Equal(t, "child", onDisk.CurrentControlPlane)
		assert.Equal(t, "other.example:443", onDisk.ControlPlanes[1].APIServer)
	})
}
