package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// refreshAPI stands in for the genesis control plane's API: it lists the
// child control plane at newAddress, with the objects its config entry is
// built from.
func refreshAPI(t *testing.T, newAddress string) string {
	t.Helper()

	genesis := controlPlaneInstance(1, "genesis", nil)
	child := controlPlaneInstance(2, "child", ptrTo(uint(1)))
	child.CACert = ptrTo("ca-child")
	child.ClientCert = ptrTo("client-new")
	child.ClientKey = ptrTo("key-new")
	child.ApiServerEndpoint = ptrTo(newAddress)
	child.ControlPlaneDefinitionID = ptrTo(uint(3))
	child.KubernetesRuntimeInstanceID = ptrTo(uint(4))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch {
		case strings.HasPrefix(r.URL.Path, v0.PathControlPlaneInstances):
			switch r.URL.Query().Get("name") {
			case "genesis":
				data = []v0.ControlPlaneInstance{genesis}
			case "child":
				data = []v0.ControlPlaneInstance{child}
			default:
				data = []v0.ControlPlaneInstance{}
			}
		case strings.HasPrefix(r.URL.Path, v0.PathControlPlaneDefinitions):
			data = []v0.ControlPlaneDefinition{{AuthEnabled: ptrTo(true)}}
		case strings.HasPrefix(r.URL.Path, v0.PathKubernetesRuntimeInstances):
			data = []v0.KubernetesRuntimeInstance{{KubernetesRuntimeDefinitionID: ptrTo(uint(5))}}
		case strings.HasPrefix(r.URL.Path, v0.PathKubernetesRuntimeDefinitions):
			data = []v0.KubernetesRuntimeDefinition{{InfraProvider: ptrTo(v0.KubernetesRuntimeInfraProviderGKE)}}
		default:
			t.Errorf("unexpected API call %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Data": data}))
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://")
}

// TestAFailedRefreshIsNotSavedBySelectingTheControlPlane covers #769 through
// tptctl config current-control-plane: when the refreshed entry for a saved
// child can't be written, the command warns that it uses the saved entry and
// then selects it, with a write that succeeds.  That write saves the entry as
// it was, not the address and certificates the failed write had.
func TestAFailedRefreshIsNotSavedBySelectingTheControlPlane(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes a read-only file")
	}

	for name, failWrite := range map[string]bool{
		"the refresh can't be written": true,
		"the refresh is written":       false,
	} {
		t.Run(name, func(t *testing.T) {
			addr := refreshAPI(t, "34.1.2.3:443")
			cfg := &cli.ThreeportConfig{
				CurrentControlPlane: "genesis",
				ControlPlanes: []cli.ControlPlane{
					{Name: "genesis", APIServer: addr, Genesis: true},
					{
						Name:          "child",
						APIServer:     "child.example:443",
						CACert:        "ca-child",
						Credentials:   []cli.Credential{{Name: "child", ClientCert: "client-child", ClientKey: "key-child"}},
						EncryptionKey: "saved-key",
					},
				},
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
			if failWrite {
				require.NoError(t, os.Chmod(path, 0o400))
			}

			// as the command does: refresh the saved entry, then select it
			resolution := resolveControlPlaneRequest(cfg, "child")
			require.NoError(t, refreshStoredControlPlane(cfg, resolution, "child"))
			require.NoError(t, os.Chmod(path, 0o600))
			cfg.SetCurrentControlPlane("child")

			saved := viper.New()
			saved.SetConfigFile(path)
			saved.SetConfigType("yaml")
			require.NoError(t, saved.ReadInConfig())
			var onDisk cli.ThreeportConfig
			require.NoError(t, saved.Unmarshal(&onDisk))
			require.Len(t, onDisk.ControlPlanes, 2)
			assert.Equal(t, "child", onDisk.CurrentControlPlane)

			child := onDisk.ControlPlanes[1]
			inMemory := cfg.ControlPlanes[1]
			if failWrite {
				assert.Equal(t, "child.example:443", child.APIServer, "the saved address stays")
				assert.Equal(t, "client-child", child.Credentials[0].ClientCert, "and the saved certificate")
				assert.Equal(t, "child.example:443", inMemory.APIServer, "in memory too")
				return
			}
			assert.Equal(t, "34.1.2.3:443", child.APIServer, "the refreshed address is saved")
			assert.Equal(t, "client-new", child.Credentials[0].ClientCert)
			assert.Equal(t, "saved-key", child.EncryptionKey, "with the entry's own key")
			assert.Equal(t, "34.1.2.3:443", inMemory.APIServer)
		})
	}
}
