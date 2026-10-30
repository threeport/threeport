package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// infraOnlyTestNamespace is the control plane namespace the infra-only
// teardown tests use.
const infraOnlyTestNamespace = "threeport-control-plane"

// fakeClusterApi is a Kubernetes API holding the control plane's namespace
// and the persistent volume behind its database's claim, enough for the
// control plane to be deleted from it. Deleting the namespace deletes the
// volume, as the storage driver would once it deleted the disk.
type fakeClusterApi struct {
	mu               sync.Mutex
	namespaceDeleted bool
	requests         []string
}

func (f *fakeClusterApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)

	reply := func(body interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
	notFound := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": "NotFound", "code": 404,
		})
	}
	volume := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolume",
		"metadata":   map[string]interface{}{"name": "pvc-crdb"},
		"spec": map[string]interface{}{
			"persistentVolumeReclaimPolicy": "Delete",
			"claimRef":                      map[string]interface{}{"namespace": infraOnlyTestNamespace, "name": "datadir-crdb-0"},
			"csi":                           map[string]interface{}{"driver": "pd.csi.storage.gke.io", "volumeHandle": "projects/p/zones/us-east1-b/disks/pvc-crdb"},
		},
	}

	switch r.Method + " " + r.URL.Path {
	case "GET /api":
		reply(map[string]interface{}{"kind": "APIVersions", "versions": []string{"v1"}})
	case "GET /apis":
		reply(map[string]interface{}{"kind": "APIGroupList", "apiVersion": "v1", "groups": []interface{}{}})
	case "GET /api/v1":
		reply(map[string]interface{}{
			"kind": "APIResourceList", "groupVersion": "v1",
			"resources": []interface{}{
				map[string]interface{}{"name": "namespaces", "kind": "Namespace", "namespaced": false, "verbs": []string{"get", "list", "delete"}},
				map[string]interface{}{"name": "persistentvolumes", "kind": "PersistentVolume", "namespaced": false, "verbs": []string{"get", "list"}},
			},
		})
	case "GET /api/v1/persistentvolumes":
		items := []interface{}{}
		if !f.namespaceDeleted {
			items = append(items, volume)
		}
		reply(map[string]interface{}{"kind": "PersistentVolumeList", "apiVersion": "v1", "items": items})
	case "GET /api/v1/persistentvolumes/pvc-crdb":
		if f.namespaceDeleted {
			notFound()
			return
		}
		reply(volume)
	case "DELETE /api/v1/namespaces/" + infraOnlyTestNamespace:
		f.namespaceDeleted = true
		reply(map[string]interface{}{"kind": "Status", "apiVersion": "v1", "status": "Success"})
	case "GET /api/v1/namespaces/" + infraOnlyTestNamespace:
		if f.namespaceDeleted {
			notFound()
			return
		}
		reply(map[string]interface{}{"kind": "Namespace", "apiVersion": "v1", "metadata": map[string]interface{}{"name": infraOnlyTestNamespace}})
	default:
		notFound()
	}
}

// requested reports whether the API was sent a request.
func (f *fakeClusterApi) requested(request string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r == request {
			return true
		}
	}

	return false
}

// testConnection is the connection a provider would give to the cluster
// behind server.
func testConnection(server *httptest.Server) *kube.KubeConnectionInfo {
	return &kube.KubeConnectionInfo{APIEndpoint: server.URL, Token: "token"}
}

// infraOnlyTestInstaller is an installer for the test namespace.
func infraOnlyTestInstaller() *threeport.ControlPlaneInstaller {
	cpi := threeport.NewInstaller()
	cpi.Opts.Namespace = infraOnlyTestNamespace

	return cpi
}

// TestInfraOnlyDeletesTheControlPlaneThroughTheProvidersConnection covers the
// fix for the disks --infra-only left behind: the control plane's namespace is
// deleted through the connection the provider gives, without the Threeport
// API, and its disks are waited for, before the infra is destroyed.
func TestInfraOnlyDeletesTheControlPlaneThroughTheProvidersConnection(t *testing.T) {
	cluster := &fakeClusterApi{}
	server := httptest.NewServer(cluster)
	defer server.Close()

	removeControlPlaneWithoutApi(infraOnlyTestInstaller(), v0.KubernetesRuntimeInfraProviderGKE, nil, testConnection(server))

	assert.True(t, cluster.requested("DELETE /api/v1/namespaces/"+infraOnlyTestNamespace), "the control plane's namespace is deleted")
	assert.True(t, cluster.requested("GET /api/v1/persistentvolumes/pvc-crdb"), "its disks are waited for")
}

// TestInfraOnlyGoesOnWhenTheControlPlaneCannotBeDeleted covers a teardown
// with --infra-only of a cluster the control plane cannot be deleted from: it
// warns and returns, for the infra to be destroyed anyway.
func TestInfraOnlyGoesOnWhenTheControlPlaneCannotBeDeleted(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()

	assert.NotPanics(t, func() {
		removeControlPlaneWithoutApi(infraOnlyTestInstaller(), v0.KubernetesRuntimeInfraProviderGKE, nil, testConnection(failing))
	})
	assert.NotPanics(t, func() {
		removeControlPlaneWithoutApi(infraOnlyTestInstaller(), v0.KubernetesRuntimeInfraProviderGKE, nil, nil)
	}, "a missing connection is a warning too")
}

// TestInfraOnlyLeavesOtherProvidersClustersAlone covers the providers the
// control plane is not deleted ahead of the infra for: kind, whose cluster
// takes everything with it, and OKE.
func TestInfraOnlyLeavesOtherProvidersClustersAlone(t *testing.T) {
	cluster := &fakeClusterApi{}
	server := httptest.NewServer(cluster)
	defer server.Close()

	for _, providerName := range []string{v0.KubernetesRuntimeInfraProviderKind, v0.KubernetesRuntimeInfraProviderOKE} {
		removeControlPlaneWithoutApi(infraOnlyTestInstaller(), providerName, nil, testConnection(server))
	}
	assert.Empty(t, cluster.requests)
}

// TestTheDisksStillToBeDeletedAreReadThroughTheProvidersConnection covers the
// read, before a GKE cluster is destroyed, of the only disks the cleanup after
// it may delete.
func TestTheDisksStillToBeDeletedAreReadThroughTheProvidersConnection(t *testing.T) {
	cluster := &fakeClusterApi{}
	server := httptest.NewServer(cluster)
	defer server.Close()

	disks, err := deletedControlPlaneVolumes(testConnection(server), infraOnlyTestNamespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"projects/p/zones/us-east1-b/disks/pvc-crdb"}, disks)

	_, err = deletedControlPlaneVolumes(nil, infraOnlyTestNamespace)
	assert.Error(t, err)
}
