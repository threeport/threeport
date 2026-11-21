package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// TestRememberedModuleScalesSurviveAStoppedAPI covers a retry that cannot dial the API.
func TestRememberedModuleScalesSurviveAStoppedAPI(t *testing.T) {
	// the first attempt stored the counts before the API was scaled to zero
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "primary-13"}}
	kubeClient := testKubeClient()
	want := []ModuleDeploymentScale{{
		Namespace: "example",
		Name:      "example-controller",
		Replicas:  1,
	}}
	require.NoError(t, cpi.rememberModuleScales(kubeClient, want))

	// the retry reads that record instead of discovering through the stopped API
	got, err := cpi.rememberedModuleScales(kubeClient)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// TestModuleDatabaseNamesSkipTheControlPlaneDatabase covers a module that
// keeps its own database and a core module that does not.
func TestModuleDatabaseNamesSkipTheControlPlaneDatabase(t *testing.T) {
	// a core module and a repeated name are left out
	core := true
	names, err := moduleDatabaseNames([]api.ModuleApi{
		{Name: util.Ptr("threeport"), Core: &core},
		{Name: util.Ptr("my-module")},
		{Name: util.Ptr("")},
		{Name: util.Ptr("my-module")},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"threeport_my_module_api"}, names)
}

// TestModuleDatabaseNamesUseTheRegisteredShortName covers an SDK registration.
// The registered name carries the API namespace, and the database uses the short name.
func TestModuleDatabaseNamesUseTheRegisteredShortName(t *testing.T) {
	// a registered module API name derives the database the installer creates
	names, err := moduleDatabaseNames([]api.ModuleApi{
		{Name: util.Ptr("example.com/my-module-module-api")},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"threeport_my_module_api"}, names)
}

// TestRememberedModuleDatabasesSurviveAStoppedAPI covers a retry that cannot list databases.
func TestRememberedModuleDatabasesSurviveAStoppedAPI(t *testing.T) {
	// the first attempt stored the names before the API was scaled to zero
	kubeClient := testKubeClient()
	require.NoError(t, rememberModuleDatabases(kubeClient, "primary-13", []string{"threeport_my_module_api"}))

	// the retry reads that record, and a missing record is not an empty list
	got, recorded, err := readModuleDatabases(kubeClient, "primary-13")
	require.NoError(t, err)
	require.True(t, recorded)
	require.Equal(t, []string{"threeport_my_module_api"}, got)
	_, recorded, err = readModuleDatabases(kubeClient, "missing")
	require.NoError(t, err)
	require.False(t, recorded)
}

// TestRecordModuleReplicasReplacesAStaleCount covers a module scaled after an earlier backup.
func TestRecordModuleReplicasReplacesAStaleCount(t *testing.T) {
	// the deployment is running at five after an earlier backup stored two
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "example-controller",
			"namespace": "example",
			"annotations": map[string]interface{}{
				replicasBeforeRestoreAnnotation: "2",
			},
		},
		"spec": map[string]interface{}{"replicas": int64(5)},
	}}
	kubeClient := testKubeClient(deployment)
	cpi := &ControlPlaneInstaller{}
	require.NoError(t, cpi.recordModuleReplicas(kubeClient, deployment, 5))

	// the next stop must restore five, not the earlier two
	updated, err := kubeClient.Resource(deploymentGVR).Namespace("example").Get(
		context.Background(), "example-controller", metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, "5", updated.GetAnnotations()[replicasBeforeRestoreAnnotation])
}
