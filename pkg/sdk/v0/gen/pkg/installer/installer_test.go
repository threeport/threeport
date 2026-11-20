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

// TestAModuleConnectsToTheDatabaseAsItsOwnUser checks that the generated
// installer never brings the database's root credentials, or the threeport
// user's, into the module's namespace: the control plane creates the
// module's own user, and the module's API server and migrator connect as it
// with the credentials written to the module's namespace.  The database the
// module used to create, which nothing connected to, is gone.
func TestAModuleConnectsToTheDatabaseAsItsOwnUser(t *testing.T) {
	source := generateInstaller(t)

	for _, gone := range []string{"db-root-cert", "db-threeport-cert", "db-create", "db-init", "cockroach sql", "CREATE DATABASE", "DbInitLocation"} {
		assert.NotContains(t, source, gone)
	}

	created := strings.Index(source, `tp_installer.EnsureModuleDatabaseUser(
		i.KubeClient,
		i.ThreeportNamespace,
		i.ModuleNamespace,
		"widget",
	)`)
	require.NotEqual(t, -1, created, "the module's user is created from the control plane")
	deployed := strings.Index(source, "kube.CreateOrUpdateResource(widgetApiDeploy")
	require.NotEqual(t, -1, deployed)
	assert.Less(t, created, deployed, "the user exists before its pods connect")

	assert.Regexp(t, `"annotations": map\[string\]interface\{\}\{\s+tp_installer.ModuleDatabaseCredsAnnotation: databaseCreds,`, source,
		"rotated credentials replace the pods")
	assert.Contains(t, source, "databaseEnv := tp_installer.ModuleDatabaseEnv()")
	assert.Contains(t, source, "}, databaseEnv...)", "the API server connects as the module's user")
	assert.Regexp(t, `"env": databaseEnv,\s+"image": fmt.Sprintf\(\s+"%s/threeport-widget-database-migrator:%s"`, source,
		"the migrator connects as the module's user")
	assert.Regexp(t, `"name": tp_installer.ModuleDatabaseCredsSecretName,\s+"secret": map\[string\]interface\{\}\{\s+"defaultMode": 420,\s+"items": \[\]interface\{\}\{map\[string\]interface\{\}\{\s+"key":  "ca.crt",\s+"path": "ca.crt",\s+\}\},`, source,
		"only the CA's certificate is mounted")
	assert.Len(t, regexp.MustCompile(`"mountPath": "/etc/threeport/db-certs",\s+"name":\s+tp_installer.ModuleDatabaseCredsSecretName,`).FindAllString(source, -1), 2,
		"the API server and the migrator verify the database with it")
}
