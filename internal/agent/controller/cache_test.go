package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/threeport/threeport/internal/agent"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
)

// selectorOf returns the label selector a set of cache options applies to
// ThreeportWorkloads.
func selectorOf(t *testing.T, options cache.Options) labels.Selector {
	t.Helper()

	require.Len(t, options.ByObject, 1)
	for object, byObject := range options.ByObject {
		assert.IsType(t, &agentapi.ThreeportWorkload{}, object)
		require.NotNil(t, byObject.Label)
		return byObject.Label
	}

	return nil
}

// every workload belongs to exactly one of the two caches, or to neither when
// it is another control plane's
func TestCacheSelectorsPartitionWorkloads(t *testing.T) {
	scoped := selectorOf(t, CacheOptions("mine"))
	legacy := selectorOf(t, LegacyCacheOptions())

	mine := labels.Set(agent.ThreeportWorkloadLabels("mine"))
	theirs := labels.Set(agent.ThreeportWorkloadLabels("theirs"))
	unlabeled := labels.Set{}
	otherLabels := labels.Set{"app": "something"}

	assert.True(t, scoped.Matches(mine))
	assert.False(t, legacy.Matches(mine))

	assert.False(t, scoped.Matches(theirs), "another control plane's workload stays out of the cache")
	assert.False(t, legacy.Matches(theirs), "another control plane's workload stays out of the legacy cache")

	assert.False(t, scoped.Matches(unlabeled))
	assert.True(t, legacy.Matches(unlabeled), "a workload from before the label must stay visible to be finalized")
	assert.True(t, legacy.Matches(otherLabels))
}

// an agent that has not been told its control plane keeps watching everything
func TestCacheOptionsUnscopedWithoutName(t *testing.T) {
	assert.Empty(t, CacheOptions("").ByObject)

	legacy, err := NewLegacyWorkloadCluster(nil, nil, "")
	require.NoError(t, err)
	assert.Nil(t, legacy, "no second cache is needed when nothing is scoped")
}

// legacyCache stands in for the cache of unlabeled workloads
type legacyCache struct {
	cache.Cache
	reader client.Reader
}

func (c legacyCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.reader.Get(ctx, key, obj, opts...)
}

func TestGetThreeportWorkloadFallsBackToLegacyCache(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, agentapi.AddToScheme(scheme))

	named := func(name string) *agentapi.ThreeportWorkload {
		return &agentapi.ThreeportWorkload{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	scoped := fake.NewClientBuilder().WithScheme(scheme).WithObjects(named("labeled")).Build()
	legacy := fake.NewClientBuilder().WithScheme(scheme).WithObjects(named("unlabeled")).Build()

	r := &ThreeportWorkloadReconciler{
		Client:      scoped,
		LegacyCache: legacyCache{reader: legacy},
	}

	for _, name := range []string{"labeled", "unlabeled"} {
		var got agentapi.ThreeportWorkload
		require.NoError(t, r.getThreeportWorkload(context.Background(), types.NamespacedName{Name: name}, &got), name)
		assert.Equal(t, name, got.Name)
	}

	// a workload in neither cache is another control plane's
	var missing agentapi.ThreeportWorkload
	err := r.getThreeportWorkload(context.Background(), types.NamespacedName{Name: "theirs"}, &missing)
	assert.True(t, client.IgnoreNotFound(err) == nil && err != nil, "expected NotFound, got %v", err)

	// with no legacy cache only the scoped one is read
	r.LegacyCache = nil
	var unlabeled agentapi.ThreeportWorkload
	err = r.getThreeportWorkload(context.Background(), types.NamespacedName{Name: "unlabeled"}, &unlabeled)
	assert.Error(t, err)
}

// fakeAPIServer serves just enough discovery for the ThreeportWorkload kind and
// records the label selector of every list and watch request for it.
func fakeAPIServer(t *testing.T) (*rest.Config, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var selectors []string

	groupVersion := agentapi.GroupVersion.String()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"kind":"APIVersions","versions":["v1"]}`)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fmt.Sprintf(
			`{"kind":"APIGroupList","apiVersion":"v1","groups":[{"name":%q,"versions":[{"groupVersion":%q,"version":%q}],"preferredVersion":{"groupVersion":%q,"version":%q}}]}`,
			agentapi.GroupVersion.Group, groupVersion, agentapi.GroupVersion.Version, groupVersion, agentapi.GroupVersion.Version,
		))
	})
	mux.HandleFunc("/apis/"+groupVersion, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fmt.Sprintf(
			`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":%q,"resources":[{"name":"threeportworkloads","singularName":"threeportworkload","namespaced":false,"kind":"ThreeportWorkload","verbs":["get","list","watch"]}]}`,
			groupVersion,
		))
	})
	mux.HandleFunc("/apis/"+groupVersion+"/threeportworkloads", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		selectors = append(selectors, r.URL.Query().Get("labelSelector"))
		mu.Unlock()

		if r.URL.Query().Get("watch") == "true" {
			w.Header().Set("Content-Type", "application/json")
			// a streaming list is complete once the server says its initial
			// events have ended
			if r.URL.Query().Get("sendInitialEvents") == "true" {
				_, _ = fmt.Fprintf(w,
					`{"type":"BOOKMARK","object":{"kind":"ThreeportWorkload","apiVersion":%q,"metadata":{"resourceVersion":"1","annotations":{"k8s.io/initial-events-end":"true"}}}}`+"\n",
					groupVersion,
				)
			}
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		writeJSON(w, fmt.Sprintf(
			`{"kind":"ThreeportWorkloadList","apiVersion":%q,"metadata":{"resourceVersion":"1"},"items":[]}`,
			groupVersion,
		))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &rest.Config{Host: server.URL}, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), selectors...)
	}
}

// the manager's cache and the legacy cluster's cache each ask the API server
// only for their own slice of ThreeportWorkloads, so another control plane's
// workloads are never sent to this agent at all
func TestCachesRequestOnlyTheirOwnWorkloads(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, agentapi.AddToScheme(scheme))
	config, requestedSelectors := fakeAPIServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scopedOptions := CacheOptions("mine")
	scopedOptions.Scheme = scheme
	scoped, err := cache.New(config, scopedOptions)
	require.NoError(t, err)

	legacy, err := NewLegacyWorkloadCluster(config, scheme, "mine")
	require.NoError(t, err)
	require.NotNil(t, legacy)

	go func() { _ = scoped.Start(ctx) }()
	go func() { _ = legacy.Start(ctx) }()

	for _, c := range []cache.Cache{scoped, legacy.GetCache()} {
		_, err := c.GetInformer(ctx, &agentapi.ThreeportWorkload{})
		require.NoError(t, err)
		require.True(t, c.WaitForCacheSync(ctx))
	}

	assert.ElementsMatch(t, []string{
		agent.ControlPlaneNameLabelKey + "=mine",
		"!" + agent.ControlPlaneNameLabelKey,
	}, uniqueSelectors(requestedSelectors()))
}

func uniqueSelectors(selectors []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, selector := range selectors {
		if !seen[selector] {
			seen[selector] = true
			unique = append(unique, selector)
		}
	}

	return unique
}
