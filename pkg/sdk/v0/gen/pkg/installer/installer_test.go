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
		assert.Contains(t, source, `tp_installer.CopySecret(
		i.KubeClient,
		*i.KubeRestMapper,
		"`+secret+`",`, "secret %s is not copied into the module's namespace", secret)
	}

	// the installer's copy refreshes a module's existing copies, which hold
	// the broker's settings from before it required TLS
	assert.NotContains(t, source, "func copySecret(")

	assert.Contains(t, source, "tpAuthVolumes = append(tpAuthVolumes, tp_installer.ModuleBrokerCAVolume())")
	assert.Contains(t, source, "tpAuthVolumeMounts = append(tpAuthVolumeMounts, tp_installer.ModuleBrokerCAVolumeMount())")
	assert.Contains(t, source, "tpAuthVolumeMounts = append(tpAuthVolumeMounts, tp_installer.ModuleBrokerCASystemVolumeMount())")

	// the CA's certificate is added before either deployment takes the volumes
	added := strings.Index(source, "tp_installer.ModuleBrokerCAVolume()")
	for _, use := range []*regexp.Regexp{
		regexp.MustCompile(`apiVolumes := tpAuthVolumes`),
		regexp.MustCompile(`"volumes":\s+tpAuthVolumes`),
	} {
		at := use.FindStringIndex(source)
		require.NotNil(t, at, "expected the generated installer to use %s", use)
		assert.Less(t, added, at[0], "the CA's certificate is added after %s", use)
	}
}

// TestAModuleInstallsIntoTheConnectedControlPlanesNamespace checks that the
// generated installer reads the namespace of the control plane its API client
// is connected to before it copies anything, and keeps a namespace it was given.
func TestAModuleInstallsIntoTheConnectedControlPlanesNamespace(t *testing.T) {
	source := generateInstaller(t)

	// assert the lookup uses the connected control plane, only for the default
	assert.Contains(t, source, "i.ThreeportNamespace == defaultThreeportNamespace")
	assert.Contains(t, source, "client.GetSelfControlPlaneInstance(i.ApiClient, i.ApiEndpoint)")
	assert.Contains(t, source, "i.ThreeportNamespace = *controlPlane.Namespace")

	// assert the lookup comes before the first secret is copied
	lookup := strings.Index(source, "GetSelfControlPlaneInstance")
	firstCopy := strings.Index(source, "CopySecret(")
	require.NotEqual(t, -1, lookup)
	require.NotEqual(t, -1, firstCopy)
	assert.Less(t, lookup, firstCopy, "the namespace is read after a secret was copied")
}

// TestAModuleRefusesANamespaceAnotherControlPlaneOwns checks the generated installer rejects a taken namespace first.
func TestAModuleRefusesANamespaceAnotherControlPlaneOwns(t *testing.T) {
	source := generateInstaller(t)

	// assert the rejection uses the connected control plane and comes first
	reject := strings.Index(source, "tp_installer.RejectExistingModuleNamespace(")
	create := strings.Index(source, "create namespace")
	require.NotEqual(t, -1, reject)
	require.NotEqual(t, -1, create)
	assert.Less(t, reject, create)
	assert.Regexp(t, regexp.MustCompile(`RejectExistingModuleNamespace\(\s*i\.KubeClient,\s*i\.ModuleNamespace,\s*i\.ThreeportNamespace,\s*apiServerName,`), source)

	// assert the created namespace records that control plane
	assert.Contains(t, source, "tp_installer.ModuleNamespaceOwnerLabel: i.ThreeportNamespace")
}

// TestAModuleSpreadsAndProtectsItsReplicasLikeTheControlPlane checks that the
// generated installer gives the module API server and each controller the
// same pod anti-affinity and disruption budget the control plane gives its own
// components, so a node drain leaves one replica of each serving.
func TestAModuleSpreadsAndProtectsItsReplicasLikeTheControlPlane(t *testing.T) {
	source := generateInstaller(t)

	// assert the API server and the controller each prefer separate nodes
	assert.Equal(t, 2, strings.Count(source, "tp_installer.ComponentAntiAffinity("))

	// assert each applies a budget with the replica count it deploys with
	assert.Equal(t, 2, strings.Count(source, "tp_installer.ApplyComponentDisruptionBudget("))
	assert.Contains(t, source, "i.KubeRestMapper, i.ModuleNamespace, apiServerName, replicas)")

	// assert the budget is applied after the deployment it protects exists
	deployment := strings.Index(source, "failed to create/update widget API deployment")
	budget := strings.Index(source, "tp_installer.ApplyComponentDisruptionBudget(")
	require.NotEqual(t, -1, deployment)
	require.NotEqual(t, -1, budget)
	assert.Less(t, deployment, budget, "the budget is applied before the API deployment")
}
