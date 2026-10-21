package installer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// generateInstaller writes a module's installer into a scratch directory and
// returns it.
func generateInstaller(t *testing.T) string {
	t.Helper()

	t.Chdir(t.TempDir())
	require.NoError(t, GenInstaller(
		&gen.Generator{
			Module:     true,
			ModulePath: "example.com/widget-module",
			ApiObjectGroups: []gen.ApiObjectGroup{{
				ControllerDomain: "Widget",
				ControllerName:   "widget-controller",
				ReconciledObjects: []gen.ReconciledObject{
					{Name: "WidgetInstance", Versions: []string{"v0"}},
				},
				ApiObjects: []*gen.ApiObject{
					{TypeName: "WidgetInstance", Version: "v0", Reconciler: true},
				},
			}},
		},
		&sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"},
	))
	generated, err := os.ReadFile(filepath.Join("pkg", "installer", "v0", "installer_gen.go"))
	require.NoError(t, err)

	return string(generated)
}

// TestAModuleConnectsToTheMessageBrokerOverTLS checks that a module's API
// server and controller get what they verify the control plane's message
// broker with: its CA's certificate, copied into the module's namespace and
// mounted where the copied config secrets point them.  Their credentials
// come in those config secrets.
func TestAModuleConnectsToTheMessageBrokerOverTLS(t *testing.T) {
	source := generateInstaller(t)

	for _, secret := range []string{"controller-config", installer.ThreeportApiConfigSecret, installer.NatsCACertSecretName} {
		assert.Contains(t, source, `copySecret(
		i.KubeClient,
		*i.KubeRestMapper,
		"`+secret+`",`, "secret %s is not copied into the module's namespace", secret)
	}

	assert.Contains(t, source, "tpAuthVolumes = append(tpAuthVolumes, tp_installer.NatsClientCAVolume())")
	assert.Contains(t, source, "tpAuthVolumeMounts = append(tpAuthVolumeMounts, tp_installer.NatsClientCAVolumeMount())")

	// the CA's certificate is added before either deployment takes the volumes
	added := strings.Index(source, "tp_installer.NatsClientCAVolume()")
	for _, use := range []*regexp.Regexp{
		regexp.MustCompile(`apiVolumes := tpAuthVolumes`),
		regexp.MustCompile(`"volumes":\s+tpAuthVolumes`),
	} {
		at := use.FindStringIndex(source)
		require.NotNil(t, at, "expected the generated installer to use %s", use)
		assert.Less(t, added, at[0], "the CA's certificate is added after %s", use)
	}
}
