package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// savedChildConfig is a config with the saved child and the genesis at
// genesisAddress to refresh it from.
func savedChildConfig(genesisAddress string) *cli.ThreeportConfig {
	return &cli.ThreeportConfig{
		CurrentControlPlane: "genesis",
		ControlPlanes: []cli.ControlPlane{
			{Name: "genesis", APIServer: genesisAddress, Genesis: true},
			{
				Name:          "child",
				APIServer:     "child.example:443",
				CACert:        "ca-child",
				Credentials:   []cli.Credential{{Name: "child", ClientCert: "client-child", ClientKey: "key-child"}},
				EncryptionKey: "saved-key",
			},
		},
	}
}

// useConfigFile points viper at a config file holding cfg, made read-only
// when readOnly is set.
func useConfigFile(t *testing.T, cfg *cli.ThreeportConfig, readOnly bool) {
	t.Helper()

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
	if readOnly {
		require.NoError(t, os.Chmod(path, 0o400))
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	}
}

// TestARefreshThatFallsBackSaysWhy covers #770: a refresh that leaves the
// saved entry in place says why, so a manager that is down, one that doesn't
// have the control plane, an API lookup that fails, and a config that can't
// be written can be told apart.
func TestARefreshThatFallsBackSaysWhy(t *testing.T) {
	t.Run("the manager can't be reached", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		addr := strings.TrimPrefix(server.URL, "http://")
		server.Close()
		cfg := savedChildConfig(addr)

		err := refreshFromManager(cfg, resolveControlPlaneRequest(cfg, "child"), "child")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no configured control plane manages it (genesis: ")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("the manager doesn't have it", func(t *testing.T) {
		addr := strings.TrimPrefix(controlPlaneInstanceServer(t, controlPlaneInstance(1, "genesis", nil)).URL, "http://")
		cfg := savedChildConfig(addr)

		err := refreshFromManager(cfg, resolveControlPlaneRequest(cfg, "child"), "child")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no configured control plane manages it (genesis: ")
		assert.ErrorIs(t, err, client_lib.ErrObjectNotFound)
	})

	t.Run("the manager's API can't build the entry", func(t *testing.T) {
		// the instance server has no definitions to build the entry from
		child := controlPlaneInstance(2, "child", ptrTo(uint(1)))
		child.CACert = ptrTo("ca-child")
		child.ControlPlaneDefinitionID = ptrTo(uint(3))
		child.KubernetesRuntimeInstanceID = ptrTo(uint(4))
		addr := strings.TrimPrefix(controlPlaneInstanceServer(t, controlPlaneInstance(1, "genesis", nil), child).URL, "http://")
		cfg := savedChildConfig(addr)

		err := refreshFromManager(cfg, resolveControlPlaneRequest(cfg, "child"), "child")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get control plane definition associated with the control plane being set")
	})

	t.Run("the config can't be written", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root writes a read-only file")
		}
		cfg := savedChildConfig(refreshAPI(t, "34.1.2.3:443"))
		useConfigFile(t, cfg, true)

		err := refreshFromManager(cfg, resolveControlPlaneRequest(cfg, "child"), "child")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update threeport config: failed to write config to file")
	})

	t.Run("the refresh is written", func(t *testing.T) {
		cfg := savedChildConfig(refreshAPI(t, "34.1.2.3:443"))
		useConfigFile(t, cfg, false)

		require.NoError(t, refreshFromManager(cfg, resolveControlPlaneRequest(cfg, "child"), "child"))
		assert.Equal(t, "34.1.2.3:443", cfg.ControlPlanes[1].APIServer)
	})

	t.Run("nothing to refresh from", func(t *testing.T) {
		cfg := &cli.ThreeportConfig{ControlPlanes: []cli.ControlPlane{{Name: "child", APIServer: "child.example:443"}}}
		assert.NoError(t, refreshFromManager(cfg, resolveControlPlaneRequest(cfg, "child"), "child"))

		genesis := &cli.ThreeportConfig{ControlPlanes: []cli.ControlPlane{{Name: "genesis", Genesis: true}}}
		assert.NoError(t, refreshFromManager(genesis, resolveControlPlaneRequest(genesis, "genesis"), "genesis"))
	})
}

// TestTheFallbackWarningCarriesTheReason covers what the command prints when
// the refresh falls back: the warning names the control plane, the reason,
// and that the saved entry is used.
func TestTheFallbackWarningCarriesTheReason(t *testing.T) {
	addr := strings.TrimPrefix(controlPlaneInstanceServer(t, controlPlaneInstance(1, "genesis", nil)).URL, "http://")
	cfg := savedChildConfig(addr)

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = writer
	refreshErr := refreshStoredControlPlane(cfg, resolveControlPlaneRequest(cfg, "child"), "child")
	os.Stdout = stdout
	require.NoError(t, writer.Close())
	printed, err := io.ReadAll(reader)
	require.NoError(t, err)

	require.NoError(t, refreshErr, "the command goes on with the saved entry")
	assert.Contains(t, string(printed), "Could not refresh control plane child: no configured control plane manages it (genesis: ")
	assert.Contains(t, string(printed), "object not found")
	assert.Contains(t, string(printed), ". Using the saved entry.")
	assert.Equal(t, "child.example:443", cfg.ControlPlanes[1].APIServer)
}
