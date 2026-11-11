package v0

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// balanceTestMapper maps the kinds the cluster autoscaler install creates.
func balanceTestMapper() *meta.RESTMapper {
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

// TestTheAutoscalerBalancesANodeGroupPerZone covers the cluster autoscaler
// an EKS cluster gets: it keeps similar node groups the same size, so a
// production cluster's node group per zone has room in each zone for that
// zone's database replica.
func TestTheAutoscalerBalancesANodeGroupPerZone(t *testing.T) {
	var mu sync.Mutex
	var deployment map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var object map[string]any
		if len(body) > 0 {
			require.NoError(t, json.Unmarshal(body, &object))
			if object["kind"] == "Deployment" {
				mu.Lock()
				deployment = object
				mu.Unlock()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	kubeClient, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	require.NoError(t, installEksClusterAutoscaler(kubeClient, balanceTestMapper(), "production", "123456789012"))

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, deployment)
	containers, found, err := unstructured.NestedSlice(deployment, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	assert.Contains(t, containers[0].(map[string]any)["command"], "--balance-similar-node-groups")
}
