package v0

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestMatchRestoredSourceUsesTheSourceNamespace covers a backup row found by namespace.
func TestMatchRestoredSourceUsesTheSourceNamespace(t *testing.T) {
	// the backup names the namespace it was taken from
	namespace := "primary-05"
	name := "primary-06"
	got, err := matchRestoredSource([]api.ControlPlaneInstance{
		{Name: util.Ptr("genesis-11"), Namespace: util.Ptr("genesis-11")},
		{Name: &name, Namespace: &namespace},
	}, "primary-05", "primary-06")
	require.NoError(t, err)
	require.Equal(t, "primary-06", util.DerefString(got.Name))
}

// TestMatchRestoredSourceUsesTheStoredName covers a backup row found by its old name.
func TestMatchRestoredSourceUsesTheStoredName(t *testing.T) {
	// a renamed source keeps its old name in the backup
	name := "primary-05"
	got, err := matchRestoredSource([]api.ControlPlaneInstance{
		{Name: &name, Namespace: util.Ptr("somewhere-else")},
	}, "primary-05", "primary-06")
	require.NoError(t, err)
	require.Equal(t, "primary-05", util.DerefString(got.Name))
}

// TestEmptyRestoreHasNoControlPlane covers a child database that stores no control plane row.
func TestEmptyRestoreHasNoControlPlane(t *testing.T) {
	// the parent api holds the control plane, so an empty restore is that case
	empty := []api.ControlPlaneInstance{}
	require.ErrorIs(t, missingRestoredControlPlane(nil), errNoRestoredControlPlane)
	require.ErrorIs(t, missingRestoredControlPlane(&empty), errNoRestoredControlPlane)
	require.NoError(t, missingRestoredControlPlane(&[]api.ControlPlaneInstance{{}}))
}

// TestAPIDeploymentStoppedSkipsADial covers a retry that finds the API already scaled to zero.
func TestAPIDeploymentStoppedSkipsADial(t *testing.T) {
	// the API deployment is already at zero replicas
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      ThreeportAPIServiceResourceName,
			"namespace": "primary-13",
		},
		"spec": map[string]interface{}{
			"replicas": int64(0),
		},
	}}
	kubeClient := testKubeClient(deployment)

	// a stopped API must not be dialed for module discovery
	stopped, err := apiDeploymentStopped(kubeClient, "primary-13")
	require.NoError(t, err)
	require.True(t, stopped)
}
