package v0

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// roundTripClient hands every object it creates or updates to the fake client
// as it would arrive from the API server, with its numbers as int64.  The
// fake client deep-copies what it is given and panics on the plain ints the
// install manifests are written with.
type roundTripClient struct{ dynamic.Interface }

func (c roundTripClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return roundTripResource{c.Interface.Resource(gvr)}
}

type roundTripResource struct {
	dynamic.NamespaceableResourceInterface
}

func (r roundTripResource) Namespace(namespace string) dynamic.ResourceInterface {
	return roundTripNamespacedResource{r.NamespaceableResourceInterface.Namespace(namespace)}
}

func (r roundTripResource) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, sub ...string) (*unstructured.Unstructured, error) {
	return r.NamespaceableResourceInterface.Create(ctx, roundTrip(obj), opts, sub...)
}

type roundTripNamespacedResource struct{ dynamic.ResourceInterface }

func (r roundTripNamespacedResource) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, sub ...string) (*unstructured.Unstructured, error) {
	return r.ResourceInterface.Create(ctx, roundTrip(obj), opts, sub...)
}

// roundTrip returns obj round-tripped through JSON.
func roundTrip(obj *unstructured.Unstructured) *unstructured.Unstructured {
	data, err := obj.MarshalJSON()
	if err != nil {
		panic(err)
	}
	out := &unstructured.Unstructured{}
	if err := out.UnmarshalJSON(data); err != nil {
		panic(err)
	}

	return out
}

// dependenciesMapper returns a mapper for every kind the dependencies install
// creates.
func dependenciesMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "policy", Version: "v1"},
	})
	for _, gvk := range []schema.GroupVersionKind{
		{Version: "v1", Kind: "Secret"},
		{Version: "v1", Kind: "ConfigMap"},
		{Version: "v1", Kind: "Service"},
		{Version: "v1", Kind: "ServiceAccount"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
	} {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)

	return mapper
}

// natsContainers installs the control plane dependencies into a fake cluster
// and returns the containers of the NATS StatefulSet and of nats-box, keyed by
// container name.
func natsContainers(t *testing.T) map[string]map[string]interface{} {
	t.Helper()

	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mapper := dependenciesMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{
		Name:                        ControlPlaneName,
		Namespace:                   ControlPlaneNamespace,
		RestApiInfo:                 ThreeportRestApi,
		DatabaseMigratorInfo:        DatabaseMigrator,
		AgentInfo:                   ThreeportAgent,
		AdditionalOptions:           map[string]interface{}{},
		CreateOrUpdateKubeResources: true,
	}}
	dbCreds, err := auth.GenerateDbCreds(ControlPlaneNamespace)
	require.NoError(t, err)
	require.NoError(t, cpi.InstallThreeportControlPlaneDependencies(roundTripClient{kubeClient}, &mapper, "", dbCreds))

	containers := map[string]map[string]interface{}{}
	for gvr, name := range map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: natsServiceName,
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "nats-js-box",
	} {
		object, err := kubeClient.Resource(gvr).Namespace(ControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err, name)
		list, _, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
		require.NoError(t, err)
		for _, item := range list {
			container := item.(map[string]interface{})
			containers[container["name"].(string)] = container
		}
	}
	require.Len(t, containers, 4, "the nats, reloader, metrics and nats-box containers")

	return containers
}

// quantity reads a container's resource quantity.
func quantity(t *testing.T, container map[string]interface{}, kind, name string) resource.Quantity {
	t.Helper()

	value, found, err := unstructured.NestedString(container, "resources", kind, name)
	require.NoError(t, err)
	require.True(t, found, "%s has no %s %s", container["name"], kind, name)

	return resource.MustParse(value)
}

// TestEveryNatsContainerHasResources covers the issue's resource gap: every
// container of the message broker, nats-box included, requests what it needs
// and is limited, with each limit at least its request.
func TestEveryNatsContainerHasResources(t *testing.T) {
	for name, container := range natsContainers(t) {
		for _, resourceName := range []string{"cpu", "memory"} {
			request := quantity(t, container, "requests", resourceName)
			limit := quantity(t, container, "limits", resourceName)
			assert.True(t, request.Cmp(limit) <= 0, "%s requests more %s than its limit", name, resourceName)
		}
	}
}

// TestTheNatsServerLimitLeavesHeadroom covers the server's memory limit
// against what it was measured using: twice its peak under far more traffic
// than a control plane makes, with the Go runtime told to collect before the
// container reaches it.
func TestTheNatsServerLimitLeavesHeadroom(t *testing.T) {
	const measuredPeak = "199Mi"

	server := natsContainers(t)["nats"]
	limit := quantity(t, server, "limits", "memory")
	peak := resource.MustParse(measuredPeak)
	peak.Add(peak)
	assert.True(t, limit.Cmp(peak) >= 0, "the server's memory limit %s is under twice its measured peak", limit.String())

	var goMemLimit string
	for _, item := range server["env"].([]interface{}) {
		env := item.(map[string]interface{})
		if env["name"] == "GOMEMLIMIT" {
			goMemLimit = env["value"].(string)
		}
	}
	require.NotEmpty(t, goMemLimit, "the Go runtime is not told the memory limit")
	soft := resource.MustParse(strings.TrimSuffix(goMemLimit, "B"))
	assert.True(t, soft.Cmp(limit) < 0, "GOMEMLIMIT %s is not under the memory limit %s", goMemLimit, limit.String())
	assert.Greater(t, soft.Value(), limit.Value()*8/10, "GOMEMLIMIT %s leaves most of the limit unused", goMemLimit)
}

// TestTheNatsServerHasALivenessProbe covers the issue's probe gap: a server
// that stops answering is restarted.  The probe checks only that JetStream is
// enabled, since the full health check fails while a restarted server catches
// up, and gives up later than the readiness probe, behind a startup probe.
func TestTheNatsServerHasALivenessProbe(t *testing.T) {
	server := natsContainers(t)["nats"]

	liveness, found, err := unstructured.NestedMap(server, "livenessProbe")
	require.NoError(t, err)
	require.True(t, found, "the NATS server has no liveness probe")
	path, _, _ := unstructured.NestedString(liveness, "httpGet", "path")
	assert.Equal(t, "/healthz?js-enabled-only=true", path)
	port, _, _ := unstructured.NestedInt64(liveness, "httpGet", "port")
	assert.Equal(t, int64(8222), port, "the probe checks the monitoring port")

	_, hasStartup, _ := unstructured.NestedMap(server, "startupProbe")
	assert.True(t, hasStartup, "nothing holds the liveness probe off while the server starts")

	readiness, _, _ := unstructured.NestedMap(server, "readinessProbe")
	window := func(probe map[string]interface{}) int64 {
		period, _, _ := unstructured.NestedInt64(probe, "periodSeconds")
		failures, _, _ := unstructured.NestedInt64(probe, "failureThreshold")
		return period * failures
	}
	assert.Greater(t, window(liveness), window(readiness), "the server is restarted before it is even taken out of service")
}
