package controlplane

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestChildBlueGreenInstallsThenClearsFromNamespace checks a child with FromNamespace calls the shared install, then clears the field.
func TestChildBlueGreenInstallsThenClearsFromNamespace(t *testing.T) {
	// the child already names the new deployment, and still records the old namespace
	from := "old-ns"
	name := "new-ns"
	id := uint(7)
	instance := &v0.ControlPlaneInstance{
		Name:          &name,
		Namespace:     &name,
		FromNamespace: &from,
	}
	gke := v0.KubernetesRuntimeInfraProviderGKE
	definition := &v0.KubernetesRuntimeDefinition{InfraProvider: &gke}
	runtimeName := "runtime"
	runtime := &v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &id}, Name: &runtimeName}

	called := false
	oldInstall := installBlueGreen
	oldSave := saveControlPlaneInstance
	oldProject := lookupGkeProject
	oldLookup := lookupControlPlaneByName
	t.Cleanup(func() {
		installBlueGreen = oldInstall
		saveControlPlaneInstance = oldSave
		lookupGkeProject = oldProject
		lookupControlPlaneByName = oldLookup
	})
	lookupControlPlaneByName = func(*http.Client, string, string) (*v0.ControlPlaneInstance, error) {
		return nil, client_lib.ErrObjectNotFound
	}
	lookupGkeProject = func(*http.Client, string, uint) (string, error) {
		return "project-1", nil
	}
	installBlueGreen = func(
		local *cli.ControlPlane,
		_ *http.Client,
		_ string,
		_ string,
		source *cli.ControlPlaneTarget,
		newName string,
	) error {
		called = true
		require.Equal(t, provider.GkeBackupBucketName(runtimeName, "project-1"), local.GKEProviderConfig.BackupBucket)
		require.Equal(t, name, newName)
		require.Equal(t, from, util.DerefString(source.Instance.FromNamespace))
		return nil
	}
	saveControlPlaneInstance = func(_ *http.Client, _ string, saved *v0.ControlPlaneInstance) (*v0.ControlPlaneInstance, error) {
		require.Empty(t, util.DerefString(saved.FromNamespace))
		return saved, nil
	}

	err := installChildBlueGreen(&controller.Reconciler{}, instance, runtime, definition, nil, nil)
	require.NoError(t, err)
	require.True(t, called)
}

// TestChildWithoutFromNamespaceDoesNotInstall checks a child with no source namespace is not a new deployment.
func TestChildWithoutFromNamespaceDoesNotInstall(t *testing.T) {
	name := "child"
	instance := &v0.ControlPlaneInstance{Name: &name, Namespace: &name}
	require.False(t, childBlueGreenRequested(instance))
}

// TestBlueGreenProviderSwitch checks GKE uses tptctl's bucket name and every other provider is refused.
func TestBlueGreenProviderSwitch(t *testing.T) {
	// GKE gets the same bucket name tptctl computes
	gke := v0.KubernetesRuntimeInfraProviderGKE
	local, err := blueGreenLocal(
		&v0.KubernetesRuntimeDefinition{InfraProvider: &gke},
		"runtime",
		"project-1",
	)
	require.NoError(t, err)
	require.Equal(t, provider.GkeBackupBucketName("runtime", "project-1"), local.GKEProviderConfig.BackupBucket)

	eks := v0.KubernetesRuntimeInfraProviderEKS
	_, err = blueGreenLocal(
		&v0.KubernetesRuntimeDefinition{InfraProvider: &eks},
		"runtime",
		"project-1",
	)
	require.ErrorIs(t, err, cli.ErrNoBackupBucket)
}

// TestTakenReplacementNameSkipsInstall checks a name another control plane holds never starts the install.
func TestTakenReplacementNameSkipsInstall(t *testing.T) {
	from := "old-ns"
	name := "taken-ns"
	id := uint(7)
	other := uint(9)
	instance := &v0.ControlPlaneInstance{
		Common:        v0.Common{ID: &id},
		Name:          &name,
		Namespace:     &name,
		FromNamespace: &from,
	}
	gke := v0.KubernetesRuntimeInfraProviderGKE
	definition := &v0.KubernetesRuntimeDefinition{InfraProvider: &gke}
	runtimeName := "runtime"
	runtime := &v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &id}, Name: &runtimeName}

	called := false
	oldInstall := installBlueGreen
	oldLookup := lookupControlPlaneByName
	oldProject := lookupGkeProject
	t.Cleanup(func() {
		installBlueGreen = oldInstall
		lookupControlPlaneByName = oldLookup
		lookupGkeProject = oldProject
	})
	lookupGkeProject = func(*http.Client, string, uint) (string, error) {
		return "project-1", nil
	}
	lookupControlPlaneByName = func(*http.Client, string, string) (*v0.ControlPlaneInstance, error) {
		return &v0.ControlPlaneInstance{Common: v0.Common{ID: &other}}, nil
	}
	installBlueGreen = func(*cli.ControlPlane, *http.Client, string, string, *cli.ControlPlaneTarget, string) error {
		called = true
		return nil
	}

	// install must stop before the source namespace is replaced
	err := installChildBlueGreen(&controller.Reconciler{}, instance, runtime, definition, nil, nil)
	require.ErrorIs(t, err, cli.ErrReplacementNameTaken)
	require.False(t, called)
}

// TestChildBlueGreenIsRequestedOnlyBeforeTheRename checks which instances ask for
// a new deployment: one still named for the old control plane does, and one
// already renamed to its namespace does not, even while its source namespace
// stays recorded for a module install.
func TestChildBlueGreenIsRequestedOnlyBeforeTheRename(t *testing.T) {
	// an instance that names the old control plane but the new namespace
	old, fresh, from := "primary-05", "primary-06", "primary-05"
	requested := &v0.ControlPlaneInstance{Name: &old, Namespace: &fresh, FromNamespace: &from}
	require.True(t, childBlueGreenRequested(requested))

	// the same instance renamed to its namespace, with its source still recorded
	renamed := &v0.ControlPlaneInstance{Name: &fresh, Namespace: &fresh, FromNamespace: &from}
	require.False(t, childBlueGreenRequested(renamed))

	// an instance that records no source namespace
	require.False(t, childBlueGreenRequested(&v0.ControlPlaneInstance{Name: &old, Namespace: &old}))
}

// TestChildBlueGreenWithAModuleInstallKeepsTheSourceRecorded checks that a child
// upgrade carrying a module install request takes the new name but leaves its
// source namespace recorded, so the install can remove it after it succeeds.
func TestChildBlueGreenWithAModuleInstallKeepsTheSourceRecorded(t *testing.T) {
	// a requested upgrade that carries a module install
	from, old, fresh := "primary-05", "primary-05", "primary-06"
	id := uint(7)
	request := "example.com/acme-module-api registry.example v1"
	instance := &v0.ControlPlaneInstance{
		Common:        v0.Common{ID: &id},
		Name:          &old,
		Namespace:     &fresh,
		FromNamespace: &from,
		ModuleInstall: &request,
	}
	gke := v0.KubernetesRuntimeInfraProviderGKE
	definition := &v0.KubernetesRuntimeDefinition{InfraProvider: &gke}
	runtimeName := "runtime"
	runtime := &v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &id}, Name: &runtimeName}

	oldInstall := installBlueGreen
	oldSave := saveControlPlaneInstance
	oldProject := lookupGkeProject
	oldLookup := lookupControlPlaneByName
	t.Cleanup(func() {
		installBlueGreen = oldInstall
		saveControlPlaneInstance = oldSave
		lookupGkeProject = oldProject
		lookupControlPlaneByName = oldLookup
	})
	lookupControlPlaneByName = func(*http.Client, string, string) (*v0.ControlPlaneInstance, error) {
		return nil, client_lib.ErrObjectNotFound
	}
	lookupGkeProject = func(*http.Client, string, uint) (string, error) { return "project-1", nil }
	installBlueGreen = func(*cli.ControlPlane, *http.Client, string, string, *cli.ControlPlaneTarget, string) error {
		return nil
	}
	var saved *v0.ControlPlaneInstance
	saveControlPlaneInstance = func(_ *http.Client, _ string, instance *v0.ControlPlaneInstance) (*v0.ControlPlaneInstance, error) {
		saved = instance
		return instance, nil
	}

	// run the install
	require.NoError(t, installChildBlueGreen(&controller.Reconciler{}, instance, runtime, definition, nil, nil))

	// assert the instance takes the new name and keeps the source recorded
	require.NotNil(t, saved)
	require.Equal(t, fresh, util.DerefString(saved.Name))
	require.Equal(t, from, util.DerefString(saved.FromNamespace))
	require.False(t, childBlueGreenRequested(saved))
}
