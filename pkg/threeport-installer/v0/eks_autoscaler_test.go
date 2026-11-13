package v0

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// autoscalerTestMapper maps the kinds the cluster autoscaler install creates.
func autoscalerTestMapper() *meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	for _, kind := range []string{"Role", "RoleBinding"} {
		mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}, meta.RESTScopeNamespace)
	}
	for _, kind := range []string{"ClusterRole", "ClusterRoleBinding"} {
		mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}, meta.RESTScopeRoot)
	}

	var restMapper meta.RESTMapper = mapper
	return &restMapper
}

// TestEveryVersionThreeportCreatesHasAnAutoscalerImage covers the version
// map: each supported version has an image, and each image is the release for
// the minor version it is listed under.
func TestEveryVersionThreeportCreatesHasAnAutoscalerImage(t *testing.T) {
	for _, version := range api_v0.SupportedKubernetesVersions {
		_, err := ClusterAutoscalerImage(version)
		assert.NoError(t, err, version)
	}
	for version, image := range clusterAutoscalerImages {
		assert.True(t,
			strings.HasPrefix(image, fmt.Sprintf("registry.k8s.io/autoscaling/cluster-autoscaler:v%s.", version)),
			"%s is listed for %s", image, version,
		)
	}

	_, err := ClusterAutoscalerImage("1.26")
	assert.ErrorContains(t, err, "no cluster autoscaler image is known for Kubernetes 1.26")
}

// recordingCluster is a Kubernetes API that accepts every object created on it
// and keeps what it was sent, by kind.
type recordingCluster struct {
	mu      sync.Mutex
	created map[string]map[string]any
}

// newRecordingCluster starts a recording API and returns a client for it.
func newRecordingCluster(t *testing.T) (*recordingCluster, dynamic.Interface) {
	t.Helper()
	cluster := &recordingCluster{created: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var object map[string]any
		if len(body) > 0 {
			require.NoError(t, json.Unmarshal(body, &object))
			cluster.mu.Lock()
			cluster.created[object["kind"].(string)] = object
			cluster.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	return cluster, client
}

// TestTheAutoscalerMatchesTheClustersVersionAndName covers the autoscaler an
// EKS cluster gets: the release for the cluster's minor version, finding its
// node groups by the cluster's name.
func TestTheAutoscalerMatchesTheClustersVersionAndName(t *testing.T) {
	cluster, kubeClient := newRecordingCluster(t)
	require.NoError(t, installEksClusterAutoscaler(
		kubeClient, autoscalerTestMapper(), "existing-cluster", "123456789012", "1.35",
	))

	deployment := cluster.created["Deployment"]
	require.NotNil(t, deployment)
	containers, found, err := unstructured.NestedSlice(deployment, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	container := containers[0].(map[string]any)
	assert.Equal(t, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.35.3", container["image"])
	assert.Contains(t, container["command"],
		"--node-group-auto-discovery=asg:tag=k8s.io/cluster-autoscaler/enabled,k8s.io/cluster-autoscaler/existing-cluster")
}

// TestAnAutoscalerWithNoImageForTheVersionIsNotInstalled covers a cluster on
// a version with no known image: the install fails before it creates
// anything, rather than leaving an autoscaler that doesn't match.
func TestAnAutoscalerWithNoImageForTheVersionIsNotInstalled(t *testing.T) {
	cluster, kubeClient := newRecordingCluster(t)
	err := installEksClusterAutoscaler(kubeClient, autoscalerTestMapper(), "old-cluster", "123456789012", "1.26")
	assert.ErrorContains(t, err, "no cluster autoscaler image is known for Kubernetes 1.26")
	assert.Empty(t, cluster.created)
}

// TestAnUpgradedClustersAutoscalerIsUpdated covers installing the autoscaler
// on a cluster that already runs one, as an upgraded cluster does: the
// deployment is updated to the new version's release, rather than left as it
// was.
func TestAnUpgradedClustersAutoscalerIsUpdated(t *testing.T) {
	var mu sync.Mutex
	var updated map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		isDeployment := strings.Contains(r.URL.Path, "/deployments")
		switch {
		case r.Method == http.MethodPost && isDeployment:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"AlreadyExists","code":409}`))
		case r.Method == http.MethodGet && isDeployment:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"cluster-autoscaler","namespace":"kube-system","resourceVersion":"7"}}`))
		case r.Method == http.MethodPut && isDeployment:
			mu.Lock()
			require.NoError(t, json.Unmarshal(body, &updated))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	kubeClient, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	require.NoError(t, installEksClusterAutoscaler(
		kubeClient, autoscalerTestMapper(), "upgraded-cluster", "123456789012", "1.36",
	))

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, updated, "the existing deployment was updated")
	containers, _, err := unstructured.NestedSlice(updated, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	assert.Equal(t, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.36.2", containers[0].(map[string]any)["image"])
	assert.Equal(t, "7", updated["metadata"].(map[string]any)["resourceVersion"])
}
