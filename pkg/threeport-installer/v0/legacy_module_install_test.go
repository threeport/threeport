package v0

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	k8stesting "k8s.io/client-go/testing"
)

// legacyModuleInstallObjects are the account a module install job ran as in
// v0.7.0-rc.5 and rc.6 in namespace, and its binding to cluster-admin.
func legacyModuleInstallObjects(namespace string) []runtime.Object {
	return []runtime.Object{
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata":   map[string]interface{}{"name": "module-install", "namespace": namespace},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata":   map[string]interface{}{"name": "module-install-" + namespace},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "cluster-admin",
			},
			"subjects": []interface{}{map[string]interface{}{
				"kind": "ServiceAccount", "name": "module-install", "namespace": namespace,
			}},
		}},
	}
}

// legacyModuleInstallLeft reports whether namespace still has the legacy
// module install account and binding.
func legacyModuleInstallLeft(t *testing.T, kubeClient dynamic.Interface, namespace string) (account, binding bool) {
	t.Helper()

	return resourceExists(t, kubeClient, serviceAccountsResource, namespace, "module-install"),
		resourceExists(t, kubeClient, roleBindingsResource, "", "module-install-"+namespace)
}

// TestTheLegacyModuleInstallAccountIsRemoved covers #826: the account module
// install jobs ran as in v0.7.0-rc.5 and rc.6, which nothing creates
// anymore, and its cluster role binding to cluster-admin, which deleting the
// namespace would leave, are removed from this control plane only.  Their
// absence is no error.
func TestTheLegacyModuleInstallAccountIsRemoved(t *testing.T) {
	objects := append(legacyModuleInstallObjects("child"), legacyModuleInstallObjects("threeport-control-plane")...)
	kubeClient := testKubeClient(objects...)
	cpi := NewInstaller(Namespace("child"))

	require.NoError(t, cpi.removeLegacyModuleInstallAccount(kubeClient, controllersTestMapper()))
	account, binding := legacyModuleInstallLeft(t, kubeClient, "child")
	assert.False(t, account)
	assert.False(t, binding)
	account, binding = legacyModuleInstallLeft(t, kubeClient, "threeport-control-plane")
	assert.True(t, account, "another control plane's")
	assert.True(t, binding)

	require.NoError(t, cpi.removeLegacyModuleInstallAccount(kubeClient, controllersTestMapper()), "already gone")
}

// TestInstallingTheControllersRemovesTheLegacyModuleInstallAccount covers
// where the removal runs on a control plane upgraded from v0.7.0-rc.5 or
// rc.6: installing the controllers, which an install, an upgrade, a
// reinstall and a restore all do, so the account doesn't keep cluster-admin
// unused until the control plane is torn down.
func TestInstallingTheControllersRemovesTheLegacyModuleInstallAccount(t *testing.T) {
	kubeClient, cpi := installTestBroker(t, ControlPlaneTierDev, dbRootCertsForTest())
	for _, object := range legacyModuleInstallObjects(cpi.Opts.Namespace) {
		legacy := object.(*unstructured.Unstructured)
		resource := serviceAccountsResource
		namespace := legacy.GetNamespace()
		if legacy.GetKind() == "ClusterRoleBinding" {
			resource = roleBindingsResource
		}
		_, err := kubeClient.Resource(resource).Namespace(namespace).Create(context.Background(), legacy, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	account, binding := legacyModuleInstallLeft(t, kubeClient, cpi.Opts.Namespace)
	require.True(t, account)
	require.True(t, binding)

	require.NoError(t, cpi.InstallThreeportControllers(kubeClient, controllersTestMapper(), nil))
	account, binding = legacyModuleInstallLeft(t, kubeClient, cpi.Opts.Namespace)
	assert.False(t, account)
	assert.False(t, binding)
}

// TestAFailedLegacyModuleInstallRemovalIsReported covers a delete that
// fails for another reason than the object being gone: the install or
// teardown fails, rather than leaving a cluster-admin binding behind
// unnoticed.
func TestAFailedLegacyModuleInstallRemovalIsReported(t *testing.T) {
	kubeClient := testKubeClient(legacyModuleInstallObjects("child")...)
	kubeClient.PrependReactor("delete", "clusterrolebindings", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(roleBindingsResource.GroupResource(), "module-install-child", errors.New("denied"))
	})
	cpi := NewInstaller(Namespace("child"))

	assert.ErrorContains(t, cpi.removeLegacyModuleInstallAccount(kubeClient, controllersTestMapper()),
		"failed to remove the module install ClusterRoleBinding module-install-child left by an earlier version")
	assert.ErrorContains(t, cpi.DeleteControlPlaneRoleBindings(kubeClient, controllersTestMapper()), "module-install-child")
}
