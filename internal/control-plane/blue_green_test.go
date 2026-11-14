package controlplane

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
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
	t.Cleanup(func() {
		installBlueGreen = oldInstall
		saveControlPlaneInstance = oldSave
		lookupGkeProject = oldProject
	})
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
