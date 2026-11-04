package v0

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newBindingCluster returns a fake cluster that can list cluster role bindings.
func newBindingCluster() *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{clusterRoleBindingGVR: "ClusterRoleBindingList"},
	)
}

// bindingInstaller returns an installer for a control plane namespace.
func bindingInstaller(namespace string, gke bool) *ControlPlaneInstaller {
	installer := NewInstaller()
	installer.Opts.Namespace = namespace
	if gke {
		installer.Opts.InfraProvider = "gke"
		installer.Opts.GcpProjectId = "a-project"
	}
	return installer
}

// installBindings creates the bindings the installer writes for every controller.
func installBindings(t *testing.T, cluster *dynamicfake.FakeDynamicClient, installer *ControlPlaneInstaller) {
	t.Helper()
	for _, controller := range installer.Opts.ControllerList {
		for _, binding := range installer.controllerClusterRoleBindings(controller) {
			setManagedByLabel(binding)
			_, err := cluster.Resource(clusterRoleBindingGVR).Create(context.Background(), binding, metav1.CreateOptions{})
			require.NoError(t, err)
		}
	}
}

// bindingNames lists the cluster role bindings in the fake cluster.
func bindingNames(t *testing.T, cluster *dynamicfake.FakeDynamicClient) []string {
	t.Helper()
	list, err := cluster.Resource(clusterRoleBindingGVR).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	names := []string{}
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	return names
}

// legacyBinding builds a binding as installed before it was labeled.
func legacyBinding(name, subjectNamespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata":   map[string]interface{}{"name": name},
		"subjects": []interface{}{map[string]interface{}{
			"kind": "ServiceAccount", "name": "sa", "namespace": subjectNamespace,
		}},
	}}
}

// The installer writes sixteen bindings per control plane: one threeportworkloads
// binding for each of the thirteen controllers and a cluster-admin binding for
// three of them.
func TestInstalledBindingsAreLabeledWithTheirNamespace(t *testing.T) {
	cluster := newBindingCluster()
	installer := bindingInstaller("acme-control-plane-primary", true)
	installBindings(t, cluster, installer)

	list, err := cluster.Resource(clusterRoleBindingGVR).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, len(installer.Opts.ControllerList)+len(clusterAdminControllers()))
	for _, item := range list.Items {
		assert.Equal(t, "acme-control-plane-primary", item.GetLabels()[LabelControlPlaneNamespace], item.GetName())
		assert.True(t, strings.HasPrefix(item.GetName(), "acme-control-plane-primary-"), item.GetName())
	}
}

// Deleting a control plane leaves no binding named for its namespace, and leaves
// the other control planes on the cluster as they were.
func TestDeleteRemovesOnlyThisControlPlanesBindings(t *testing.T) {
	cluster := newBindingCluster()
	// a sibling whose namespace starts with the deleted one's
	for _, namespace := range []string{"foo", "foo-bar", "other"} {
		installBindings(t, cluster, bindingInstaller(namespace, namespace == "foo"))
	}

	require.NoError(t, DeleteControlPlaneClusterRoleBindings(cluster, "foo"))

	remaining := bindingNames(t, cluster)
	perControlPlane := len(ThreeportControllerList) + len(clusterAdminControllers())
	assert.Len(t, remaining, 2*perControlPlane)
	for _, name := range remaining {
		assert.False(t, strings.HasPrefix(name, "foo-") && !strings.HasPrefix(name, "foo-bar-"), "left %s", name)
	}
	assert.Contains(t, remaining, "foo-bar-helm-workload-controller-cluster-admin")
	assert.Contains(t, remaining, "other-helm-workload-controller-cluster-admin")
}

// Bindings made before they were labeled are found by name, and one whose
// subjects are in another namespace is not this control plane's.
func TestDeleteFindsBindingsFromBeforeTheLabel(t *testing.T) {
	cluster := newBindingCluster()
	ctx := context.Background()
	for _, binding := range []*unstructured.Unstructured{
		legacyBinding("foo-aws-controller-threeportworkloads", "foo"),
		legacyBinding("foo-helm-workload-controller-cluster-admin", "foo"),
		legacyBinding("foo-gcp-controller-threeportworkloads", "elsewhere"),
	} {
		_, err := cluster.Resource(clusterRoleBindingGVR).Create(ctx, binding, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	require.NoError(t, DeleteControlPlaneClusterRoleBindings(cluster, "foo"))

	assert.Equal(t, []string{"foo-gcp-controller-threeportworkloads"}, bindingNames(t, cluster))
}

// A control plane with no bindings, or one already cleaned up, is not an error.
func TestDeleteSucceedsWhenThereIsNothingToDelete(t *testing.T) {
	cluster := newBindingCluster()
	installBindings(t, cluster, bindingInstaller("foo", false))

	require.NoError(t, DeleteControlPlaneClusterRoleBindings(cluster, "foo"))
	require.NoError(t, DeleteControlPlaneClusterRoleBindings(cluster, "foo"))
	assert.Empty(t, bindingNames(t, cluster))
}

// A failed delete is reported, and the rest are still attempted.
func TestDeleteReturnsErrorsAfterTryingEveryBinding(t *testing.T) {
	cluster := newBindingCluster()
	installBindings(t, cluster, bindingInstaller("foo", false))
	cluster.PrependReactor("delete", "clusterrolebindings", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.(k8stesting.DeleteAction).GetName() == "foo-aws-controller-threeportworkloads" {
			return true, nil, errors.New("forbidden")
		}
		return false, nil, nil
	})

	err := DeleteControlPlaneClusterRoleBindings(cluster, "foo")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo-aws-controller-threeportworkloads")
	assert.Equal(t, []string{"foo-aws-controller-threeportworkloads"}, bindingNames(t, cluster))
}

// With no namespace the selector would match nothing useful and the names would
// start with a dash, so it is refused.
func TestDeleteRequiresANamespace(t *testing.T) {
	require.Error(t, DeleteControlPlaneClusterRoleBindings(newBindingCluster(), ""))
}
