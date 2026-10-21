package v0

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	networkpolicytest "github.com/threeport/threeport/pkg/networkpolicytest/v0"
)

const testPolicyControlPlaneNamespace = "threeport-control-plane"

var (
	networkPolicyGVR   = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	testApiServerName  = "threeport-test-api-server"
	testApiServerPods  = map[string]string{"app.kubernetes.io/name": testApiServerName}
	testModuleNsName   = "threeport-test"
	testModuleNsUID    = "0c6b1b0e-5d1a-4c9e-9f5e-2f6a8d4c3b21"
	testOtherModuleNs  = "threeport-other"
	testOtherApiServer = "threeport-other-api-server"
)

// testPolicyMapper maps the kinds a module's database policy touches.
func testPolicyMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}, {Group: "networking.k8s.io", Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)

	return mapper
}

// testModuleNamespace returns a module's namespace as the cluster holds it.
func testModuleNamespace(name, uid string) *unstructured.Unstructured {
	namespace := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name":   name,
			"uid":    uid,
			"labels": map[string]interface{}{namespaceNameLabel: name},
		},
	}}

	return namespace
}

// testApiServerService returns a module API server's service selecting pods
// carrying selector.
func testApiServerService(namespace, name string, selector map[string]interface{}) *unstructured.Unstructured {
	service := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec":       map[string]interface{}{},
	}}
	if selector != nil {
		service.Object["spec"].(map[string]interface{})["selector"] = selector
	}

	return service
}

// getTestPolicy returns a module's database policy from the control plane's
// namespace.
func getTestPolicy(t *testing.T, kubeClient *dynamicfake.FakeDynamicClient, apiServerName string) *unstructured.Unstructured {
	t.Helper()

	policy, err := kubeClient.Resource(networkPolicyGVR).Namespace(testPolicyControlPlaneNamespace).Get(
		context.Background(), ModuleDatabasePolicyName(apiServerName), metav1.GetOptions{},
	)
	require.NoError(t, err)

	return policy
}

// TestAModulesPolicyAdmitsItsApiServerAlone covers what #629 asks of the
// policy: the module's API server pods, from the module's own namespace,
// on the SQL port, and nothing else.
func TestAModulesPolicyAdmitsItsApiServerAlone(t *testing.T) {
	policy := ModuleDatabasePolicy(
		testPolicyControlPlaneNamespace,
		testModuleNamespace(testModuleNsName, testModuleNsUID),
		testApiServerName,
		testApiServerPods,
	)

	networkpolicytest.AssertModuleDatabasePolicy(t, policy, testPolicyControlPlaneNamespace, testModuleNsName, testApiServerPods)
	assert.Equal(t, "threeport-database-access-"+testApiServerName, policy.GetName())
	require.Len(t, policy.GetOwnerReferences(), 1)
	assert.Equal(t, testModuleNsUID, string(policy.GetOwnerReferences()[0].UID), "the owner is the namespace the module was installed into, not one recreated under its name")
}

// TestTheDatabaseAdmitsWhatItsPoliciesAddUpTo covers the policies together,
// the way Kubernetes enforces them: a connection is admitted when any policy
// selecting the database admits it.
func TestTheDatabaseAdmitsWhatItsPoliciesAddUpTo(t *testing.T) {
	policies := []*unstructured.Unstructured{
		testInstaller("dev-0", "kubernetes-workload").databasePolicy(),
		ModuleDatabasePolicy(testPolicyControlPlaneNamespace, testModuleNamespace(testModuleNsName, testModuleNsUID), testApiServerName, testApiServerPods),
		ModuleDatabasePolicy(testPolicyControlPlaneNamespace, testModuleNamespace(testOtherModuleNs, "other-uid"), testOtherApiServer,
			map[string]string{"app.kubernetes.io/name": testOtherApiServer}),
	}
	admitted := func(peer networkpolicytest.Peer) bool {
		for _, policy := range policies {
			ok, err := networkpolicytest.Admits(policy, peer, cockroachSqlPort)
			require.NoError(t, err)
			if ok {
				return true
			}
		}

		return false
	}

	for name, peer := range map[string]networkpolicytest.Peer{
		"the control plane's API server": {Namespace: testPolicyControlPlaneNamespace, PodLabels: apiServerPod},
		"a module's API server":          {Namespace: testModuleNsName, PodLabels: testApiServerPods},
		"another module's API server":    {Namespace: testOtherModuleNs, PodLabels: map[string]string{"app.kubernetes.io/name": testOtherApiServer}},
	} {
		assert.True(t, admitted(peer), "%s reaches the database", name)
	}

	for name, peer := range map[string]networkpolicytest.Peer{
		"a module's API server labels in the other module's namespace": {
			Namespace: testOtherModuleNs, PodLabels: testApiServerPods,
		},
		"a module's API server labels in a namespace labelled as a module's": {
			Namespace:       "someone-elses",
			NamespaceLabels: map[string]string{"threeport.io/module": "true"},
			PodLabels:       testApiServerPods,
		},
		"an unlabelled pod of a module's namespace": {Namespace: testModuleNsName},
		"a control plane controller":                {Namespace: testPolicyControlPlaneNamespace, PodLabels: controllerPod},
		"the message broker":                        {Namespace: testPolicyControlPlaneNamespace, PodLabels: brokerPod},
	} {
		assert.False(t, admitted(peer), "%s must have no path to the database", name)
	}
}

func TestInstallModuleDatabasePolicyIsOwnedByTheNamespaceInTheCluster(t *testing.T) {
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), testModuleNamespace(testModuleNsName, testModuleNsUID))

	require.NoError(t, InstallModuleDatabasePolicy(
		kubeClient, testPolicyMapper(), testPolicyControlPlaneNamespace, testModuleNsName, testApiServerName, testApiServerPods,
	))

	policy := getTestPolicy(t, kubeClient, testApiServerName)
	networkpolicytest.AssertModuleDatabasePolicy(t, policy, testPolicyControlPlaneNamespace, testModuleNsName, testApiServerPods)
	require.Len(t, policy.GetOwnerReferences(), 1)
	assert.Equal(t, testModuleNsUID, string(policy.GetOwnerReferences()[0].UID))
}

func TestInstallModuleDatabasePolicyReplacesAnOldOne(t *testing.T) {
	// a policy left from a namespace of the same name that was deleted and
	// created again, and admitting other pods
	stale := ModuleDatabasePolicy(testPolicyControlPlaneNamespace, testModuleNamespace(testModuleNsName, "old-uid"), testApiServerName,
		map[string]string{"app.kubernetes.io/name": "something-else"})
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), testModuleNamespace(testModuleNsName, testModuleNsUID), stale)

	require.NoError(t, InstallModuleDatabasePolicy(
		kubeClient, testPolicyMapper(), testPolicyControlPlaneNamespace, testModuleNsName, testApiServerName, testApiServerPods,
	))

	policy := getTestPolicy(t, kubeClient, testApiServerName)
	networkpolicytest.AssertModuleDatabasePolicy(t, policy, testPolicyControlPlaneNamespace, testModuleNsName, testApiServerPods)
	assert.Equal(t, testModuleNsUID, string(policy.GetOwnerReferences()[0].UID))
}

func TestInstallModuleDatabasePolicyRefusesWhatItCannotScope(t *testing.T) {
	t.Run("a missing namespace", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

		err := InstallModuleDatabasePolicy(
			kubeClient, testPolicyMapper(), testPolicyControlPlaneNamespace, testModuleNsName, testApiServerName, testApiServerPods,
		)
		assert.ErrorContains(t, err, "failed to get module namespace "+testModuleNsName)
	})

	t.Run("no pod labels, which would admit the whole namespace", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), testModuleNamespace(testModuleNsName, testModuleNsUID))

		err := InstallModuleDatabasePolicy(
			kubeClient, testPolicyMapper(), testPolicyControlPlaneNamespace, testModuleNsName, testApiServerName, nil,
		)
		assert.ErrorContains(t, err, "has no pod labels")
		_, err = kubeClient.Resource(networkPolicyGVR).Namespace(testPolicyControlPlaneNamespace).Get(
			context.Background(), ModuleDatabasePolicyName(testApiServerName), metav1.GetOptions{},
		)
		assert.Error(t, err, "no policy is created")
	})
}

// TestInstallModuleDatabasePoliciesCoversModulesInstalledBefore covers the
// modules whose installer created no policy, which reinstall admits from the
// registry.
func TestInstallModuleDatabasePoliciesCoversModulesInstalledBefore(t *testing.T) {
	cpi := testInstaller("dev-0")

	t.Run("each module's API server is admitted by what its service selects", func(t *testing.T) {
		otherPods := map[string]interface{}{"app.kubernetes.io/name": testOtherApiServer, "tier": "api"}
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(),
			testModuleNamespace(testModuleNsName, testModuleNsUID),
			testApiServerService(testModuleNsName, testApiServerName, map[string]interface{}{"app.kubernetes.io/name": testApiServerName}),
			testModuleNamespace(testOtherModuleNs, "other-uid"),
			testApiServerService(testOtherModuleNs, testOtherApiServer, otherPods),
		)

		require.NoError(t, cpi.InstallModuleDatabasePolicies(kubeClient, testPolicyMapper(), []ModuleApiServer{
			{Namespace: testModuleNsName, Service: testApiServerName},
			{Namespace: testOtherModuleNs, Service: testOtherApiServer},
		}))

		networkpolicytest.AssertModuleDatabasePolicy(t, getTestPolicy(t, kubeClient, testApiServerName),
			testPolicyControlPlaneNamespace, testModuleNsName, testApiServerPods)
		networkpolicytest.AssertModuleDatabasePolicy(t, getTestPolicy(t, kubeClient, testOtherApiServer),
			testPolicyControlPlaneNamespace, testOtherModuleNs, map[string]string{"app.kubernetes.io/name": testOtherApiServer, "tier": "api"})
	})

	t.Run("a module whose namespace or service is gone is skipped", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), testModuleNamespace(testModuleNsName, testModuleNsUID))

		require.NoError(t, cpi.InstallModuleDatabasePolicies(kubeClient, testPolicyMapper(), []ModuleApiServer{
			{Namespace: "threeport-gone", Service: "threeport-gone-api-server"},
			{Namespace: testModuleNsName, Service: testApiServerName},
		}), "the registry outlives what it describes")
	})

	t.Run("one module that fails does not hide the others", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(),
			testModuleNamespace(testModuleNsName, testModuleNsUID),
			testApiServerService(testModuleNsName, testApiServerName, nil),
			testModuleNamespace(testOtherModuleNs, "other-uid"),
			testApiServerService(testOtherModuleNs, testOtherApiServer, map[string]interface{}{"app.kubernetes.io/name": testOtherApiServer}),
		)
		kubeClient.PrependReactor("get", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.GetNamespace() == "threeport-forbidden" {
				return true, nil, fmt.Errorf("services is forbidden")
			}

			return false, nil, nil
		})

		err := cpi.InstallModuleDatabasePolicies(kubeClient, testPolicyMapper(), []ModuleApiServer{
			{Namespace: "threeport-forbidden", Service: "threeport-forbidden-api-server"},
			{Namespace: testModuleNsName, Service: testApiServerName},
			{Namespace: testOtherModuleNs, Service: testOtherApiServer},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "threeport-forbidden")
		assert.Contains(t, err.Error(), "has no pod labels", "a service selecting nothing must not admit its whole namespace")
		getTestPolicy(t, kubeClient, testOtherApiServer)
	})

	t.Run("no modules is not an error", func(t *testing.T) {
		assert.NoError(t, cpi.InstallModuleDatabasePolicies(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), testPolicyMapper(), nil))
	})
}
