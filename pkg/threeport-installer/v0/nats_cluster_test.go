package v0

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

var configMapsGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

// installTestBroker installs the control plane's dependencies on tier into a
// fake cluster seeded with objects, and returns the cluster and installer.
func installTestBroker(t *testing.T, tier string, objects ...runtime.Object) (dynamic.Interface, *ControlPlaneInstaller) {
	t.Helper()

	fake, kubeClient, cpi := newBrokerTestClient(objects...)
	// a production install waits for the database's init job, which the
	// fake cluster has nothing to run
	succeedDropJob(fake)
	cpi.Opts.Tier = ControlPlaneTier(tier)
	cpi.Opts.CreateOrUpdateKubeResources = true
	installTestDependencies(t, kubeClient, cpi)

	return kubeClient, cpi
}

// brokerConfig returns the nats.conf the installer wrote.
func brokerConfig(t *testing.T, kubeClient dynamic.Interface) string {
	t.Helper()
	configMap := getTestObject(t, kubeClient, configMapsGVR, "nats-js-config")
	config, _, err := unstructured.NestedString(configMap.Object, "data", "nats.conf")
	require.NoError(t, err)

	return config
}

// brokerReplicas returns the replicas of the broker's stateful set.
func brokerReplicas(t *testing.T, kubeClient dynamic.Interface) int64 {
	t.Helper()
	replicas, _, err := unstructured.NestedInt64(getTestObject(t, kubeClient, statefulSetGVR, natsServiceName).Object, "spec", "replicas")
	require.NoError(t, err)

	return replicas
}

// clientReplicas installs the API server and the controllers after the
// dependencies, and returns how many broker servers each is told to
// replicate across: the API server's NATS_REPLICAS and the controllers'
// MSG_BROKER_REPLICAS.
func clientReplicas(t *testing.T, kubeClient dynamic.Interface, cpi *ControlPlaneInstaller) (string, string) {
	t.Helper()

	cpi.Opts.RestApiInfo.Name = "rest-api"
	cpi.Opts.RestApiInfo.BinaryName = "rest-api"
	cpi.Opts.DatabaseMigratorInfo = &v0.ControlPlaneComponent{Name: "database-migrator", BinaryName: "database-migrator"}
	require.NoError(t, cpi.UpdateThreeportAPIDeployment(kubeClient, installerTestMapper(), nil))
	env, err := secretValue(getTestObject(t, kubeClient, secretGVR, ThreeportApiConfigSecret), "env")
	require.NoError(t, err)
	var apiReplicas string
	for _, line := range strings.Split(env, "\n") {
		if value, ok := strings.CutPrefix(line, "NATS_REPLICAS="); ok {
			apiReplicas = value
		}
	}

	require.NoError(t, cpi.InstallThreeportControllers(kubeClient, controllersTestMapper(), nil))
	controllerReplicas := testSecretValue(t, kubeClient, "controller-config", "MSG_BROKER_REPLICAS")

	return apiReplicas, controllerReplicas
}

// controllersTestMapper maps every kind the controllers' install writes.
func controllersTestMapper() *meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"}, {Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	for _, kind := range []string{"ClusterRole", "ClusterRoleBinding"} {
		mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}, meta.RESTScopeRoot)
	}
	for _, kind := range []string{"Role", "RoleBinding"} {
		mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}, meta.RESTScopeNamespace)
	}
	for _, kind := range []string{"Secret", "ServiceAccount", "ConfigMap"} {
		mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: kind}, meta.RESTScopeNamespace)
	}
	var restMapper meta.RESTMapper = mapper

	return &restMapper
}

// TestTheBrokerClustersOnlyOnProduction covers #612's split: production runs
// three servers joined into one cluster over TLS, kept on separate nodes,
// and tells the API server and the controllers to replicate across all
// three.  Development, and an install with no tier, keep the single server
// they have always had, with no cluster and no scheduling constraint.
func TestTheBrokerClustersOnlyOnProduction(t *testing.T) {
	t.Run("production", func(t *testing.T) {
		kubeClient, cpi := installTestBroker(t, ControlPlaneTierProd, dbRootCertsForTest())

		assert.Equal(t, int64(3), brokerReplicas(t, kubeClient))
		config := brokerConfig(t, kubeClient)
		assert.Contains(t, config, "cluster {")
		for i := 0; i < 3; i++ {
			assert.Contains(t, config, fmt.Sprintf("nats://nats-js-%d.nats-js.%s.svc.cluster.local:6222", i, testControlPlaneNamespace))
		}
		assert.NotContains(t, config, "nats-js-3.")

		affinity, found, err := unstructured.NestedMap(getTestObject(t, kubeClient, statefulSetGVR, natsServiceName).Object, "spec", "template", "spec", "affinity")
		require.NoError(t, err)
		require.True(t, found, "a production broker's servers must be kept apart")
		required, found, err := unstructured.NestedSlice(affinity, "podAntiAffinity", "requiredDuringSchedulingIgnoredDuringExecution")
		require.NoError(t, err)
		require.True(t, found, "kept apart as a requirement, not a preference")
		require.Len(t, required, 1)
		labels, _, _ := unstructured.NestedStringMap(required[0].(map[string]interface{}), "labelSelector", "matchLabels")
		template := getTestObject(t, kubeClient, statefulSetGVR, natsServiceName)
		podLabels, _, _ := unstructured.NestedStringMap(template.Object, "spec", "template", "metadata", "labels")
		for key, value := range labels {
			assert.Equal(t, value, podLabels[key], "the anti-affinity must select the broker's own pods")
		}

		apiReplicas, controllerReplicas := clientReplicas(t, kubeClient, cpi)
		assert.Equal(t, "3", apiReplicas)
		assert.Equal(t, "3", controllerReplicas)
	})
	for _, tier := range []string{ControlPlaneTierDev, ""} {
		t.Run(fmt.Sprintf("tier %q", tier), func(t *testing.T) {
			kubeClient, cpi := installTestBroker(t, tier, dbRootCertsForTest())

			assert.Equal(t, int64(1), brokerReplicas(t, kubeClient))
			assert.NotContains(t, brokerConfig(t, kubeClient), "cluster {")
			_, found, err := unstructured.NestedFieldNoCopy(getTestObject(t, kubeClient, statefulSetGVR, natsServiceName).Object, "spec", "template", "spec", "affinity")
			require.NoError(t, err)
			assert.False(t, found, "a single server keeps the scheduling it has always had")

			apiReplicas, controllerReplicas := clientReplicas(t, kubeClient, cpi)
			assert.Equal(t, "1", apiReplicas)
			assert.Equal(t, "1", controllerReplicas)
		})
	}
}

// TestAProductionBrokerInstalledWithOneServerKeepsIt covers a reinstall of a
// production control plane whose broker was installed with one server,
// before production ran three.  A reinstall keeps the stateful set's count,
// so the config stays that of a single server, which a cluster block would
// leave without a JetStream quorum, and the clients keep streams one server
// can hold.
func TestAProductionBrokerInstalledWithOneServerKeepsIt(t *testing.T) {
	existing := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]interface{}{
			"name":            natsServiceName,
			"namespace":       testControlPlaneNamespace,
			"resourceVersion": "7",
			"labels":          map[string]interface{}{LabelPersistent: LabelPersistentValue},
		},
		"spec": map[string]interface{}{
			"replicas":    int64(1),
			"serviceName": natsServiceName,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{map[string]interface{}{"name": "nats", "image": "nats:old"}},
				},
			},
		},
	}}
	kubeClient, cpi := installTestBroker(t, ControlPlaneTierProd, existing, dbRootCertsForTest())

	assert.Equal(t, int64(1), brokerReplicas(t, kubeClient))
	assert.NotContains(t, brokerConfig(t, kubeClient), "cluster {")
	_, found, err := unstructured.NestedFieldNoCopy(getTestObject(t, kubeClient, statefulSetGVR, natsServiceName).Object, "spec", "template", "spec", "affinity")
	require.NoError(t, err)
	assert.False(t, found)

	apiReplicas, controllerReplicas := clientReplicas(t, kubeClient, cpi)
	assert.Equal(t, "1", apiReplicas)
	assert.Equal(t, "1", controllerReplicas)
}

// dbRootCertsForTest is the database certificates secret the database's
// install leaves for the API server's.
func dbRootCertsForTest() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": dbRootCertSecretName, "namespace": testControlPlaneNamespace},
	}}
}
