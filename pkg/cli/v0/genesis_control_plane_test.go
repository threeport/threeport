package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestDropSameNameControlPlaneRejectsWithoutForce covers the same-name guard.
func TestDropSameNameControlPlaneRejectsWithoutForce(t *testing.T) {
	require := require.New(t)

	cfg := &ThreeportConfig{
		ControlPlanes: []ControlPlane{
			{Name: "keep"},
			{Name: "target"},
		},
	}

	err := dropSameNameControlPlane(cfg, "target", false)

	require.Error(err)
	require.ErrorIs(err, ErrThreeportConfigAlreadyExists)
	require.Len(cfg.ControlPlanes, 2)
}

// TestDropSameNameControlPlaneForceKeepsSiblings covers additive overwrite.
func TestDropSameNameControlPlaneForceKeepsSiblings(t *testing.T) {
	require := require.New(t)

	cfg := &ThreeportConfig{
		ControlPlanes: []ControlPlane{
			{Name: "keep"},
			{Name: "target"},
		},
	}

	err := dropSameNameControlPlane(cfg, "target", true)

	require.NoError(err)
	require.Equal([]ControlPlane{{Name: "keep"}}, cfg.ControlPlanes)
}

// TestDropSameNameControlPlaneMissingNameIsNoop covers a first install.
func TestDropSameNameControlPlaneMissingNameIsNoop(t *testing.T) {
	require := require.New(t)

	cfg := &ThreeportConfig{
		ControlPlanes: []ControlPlane{{Name: "keep"}},
	}

	err := dropSameNameControlPlane(cfg, "new", false)

	require.NoError(err)
	require.Equal([]ControlPlane{{Name: "keep"}}, cfg.ControlPlanes)
}

// TestEnsureBootstrapKubernetesRuntimeReusesExistingRows covers a retry.
func TestEnsureBootstrapKubernetesRuntimeReusesExistingRows(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		name := r.URL.Query().Get("name")
		fmt.Fprintf(w, `{"Data":[{"ID":9,"Name":%q}]}`, name)
	}))
	defer server.Close()

	def, inst, err := ensureBootstrapKubernetesRuntime(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		"threeport-dev",
		"threeport-dev",
		true,
		"kind",
		&v0.KubernetesRuntimeInstance{Instance: v0.Instance{Name: util.Ptr("threeport-dev")}},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(9), *def.ID)
	require.Equal(uint(9), *inst.ID)
}

// TestEnsureBootstrapKubernetesRuntimeCreatesMissingRows covers a first install.
func TestEnsureBootstrapKubernetesRuntimeCreatesMissingRows(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusCreated)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			name, _ := body["Name"].(string)
			fmt.Fprintf(w, `{"Data":[{"ID":%d,"Name":%q}]}`, posts, name)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Data":[]}`))
	}))
	defer server.Close()

	def, inst, err := ensureBootstrapKubernetesRuntime(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		"threeport-dev",
		"threeport-dev",
		true,
		"kind",
		&v0.KubernetesRuntimeInstance{Instance: v0.Instance{Name: util.Ptr("threeport-dev")}},
	)

	require.NoError(err)
	require.Equal(2, posts)
	require.NotNil(def.ID)
	require.NotNil(inst.ID)
}

// TestEnsureBootstrapKubernetesRuntimeSurfacesLookupErrors covers a failed GET.
func TestEnsureBootstrapKubernetesRuntimeSurfacesLookupErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"Data":[]}`))
	}))
	defer server.Close()

	_, _, err := ensureBootstrapKubernetesRuntime(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		"threeport-dev",
		"threeport-dev",
		true,
		"kind",
		&v0.KubernetesRuntimeInstance{Instance: v0.Instance{Name: util.Ptr("threeport-dev")}},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to look up kubernetes runtime definition by name")
}

// fakeRuntimeInfra records infra deletes for cleanOnCreateError tests.
type fakeRuntimeInfra struct {
	deletes int
}

func (f *fakeRuntimeInfra) Create() (*kube.KubeConnectionInfo, error) { return nil, nil }

func (f *fakeRuntimeInfra) Delete() error {
	f.deletes++
	return nil
}

var _ provider.KubernetesRuntimeInfra = (*fakeRuntimeInfra)(nil)

// newTestUninstaller returns an Uninstaller set to tear down on failure and
// the infra fake it deletes through.
func newTestUninstaller(client dynamic.Interface) (*Uninstaller, *fakeRuntimeInfra) {
	infra := &fakeRuntimeInfra{}
	mapper := meta.RESTMapper(meta.NewDefaultRESTMapper(nil))
	u := &Uninstaller{
		controlPlane:           &threeport.ControlPlane{InfraProvider: v0.KubernetesRuntimeInfraProviderKind},
		kubernetesRuntimeInfra: infra,
		dynamicKubeClient:      client,
		mapper:                 &mapper,
		cleanConfig:            util.Ptr(false),
		cpi:                    &threeport.ControlPlaneInstaller{Opts: threeport.Options{Namespace: "threeport-control-plane"}},
		teardownOnFailure:      util.Ptr(true),
	}
	return u, infra
}

// TestCleanOnCreateErrorNilClientStillDeletesInfra covers a nil client, as an
// uninstaller has before the first client is built: no panic, and the infra is
// still deleted.
func TestCleanOnCreateErrorNilClientStillDeletesInfra(t *testing.T) {
	u, infra := newTestUninstaller(nil)

	var err error
	require.NotPanics(t, func() {
		err = u.cleanOnCreateError("failed to refresh the Kubernetes client and mapper", assert.AnError)
	})

	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 1, infra.deletes)
}

// TestCleanOnCreateErrorDeletesWorkloadsThenInfra covers the normal cleanup.
func TestCleanOnCreateErrorDeletesWorkloadsThenInfra(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	u, infra := newTestUninstaller(client)

	err := u.cleanOnCreateError("install failed", assert.AnError)

	require.ErrorIs(t, err, assert.AnError)
	var deletedNamespace bool
	for _, a := range client.Actions() {
		if a.Matches("delete", "namespaces") {
			deletedNamespace = true
		}
	}
	assert.True(t, deletedNamespace, "control plane namespace delete should be attempted")
	assert.Equal(t, 1, infra.deletes)
}

// TestCleanOnCreateErrorWorkloadFailureStillDeletesInfra covers a workload
// delete that fails, e.g. an expired token: the infra delete must still run.
func TestCleanOnCreateErrorWorkloadFailureStillDeletesInfra(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	workloadErr := fmt.Errorf("unauthorized")
	client.PrependReactor("delete", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, workloadErr
	})
	u, infra := newTestUninstaller(client)

	err := u.cleanOnCreateError("install failed", assert.AnError)

	require.ErrorIs(t, err, assert.AnError)
	require.ErrorIs(t, err, workloadErr)
	assert.Equal(t, 1, infra.deletes)
}

// TestRefreshKubeClientFailureKeepsEarlierClient covers a failed refresh: the
// uninstaller keeps the earlier client, cleanup deletes the namespace with it,
// and the infra is still deleted.
func TestRefreshKubeClientFailureKeepsEarlierClient(t *testing.T) {
	earlier := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	u, infra := newTestUninstaller(earlier)
	earlierMapper := u.mapper

	refreshErr := fmt.Errorf("token refresh failed")
	client, mapper, err := u.refreshKubeClient(
		func() (dynamic.Interface, *meta.RESTMapper, error) {
			return nil, nil, refreshErr
		},
	)

	require.ErrorIs(t, err, refreshErr)
	assert.Nil(t, client)
	assert.Nil(t, mapper)
	assert.Same(t, earlier, u.dynamicKubeClient)
	assert.Same(t, earlierMapper, u.mapper)

	var cleanupErr error
	require.NotPanics(t, func() {
		cleanupErr = u.cleanOnCreateError("failed to refresh the Kubernetes client and mapper", err)
	})
	require.ErrorIs(t, cleanupErr, refreshErr)
	var deletedNamespace bool
	for _, a := range earlier.Actions() {
		if a.Matches("delete", "namespaces") {
			deletedNamespace = true
		}
	}
	assert.True(t, deletedNamespace, "earlier client should delete the control plane namespace")
	assert.Equal(t, 1, infra.deletes)
}

// TestRefreshKubeClientSuccessReplacesClient covers a successful refresh: the
// uninstaller adopts the new client and mapper.
func TestRefreshKubeClientSuccessReplacesClient(t *testing.T) {
	earlier := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	u, _ := newTestUninstaller(earlier)

	refreshed := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	refreshedMapper := meta.RESTMapper(meta.NewDefaultRESTMapper(nil))
	client, mapper, err := u.refreshKubeClient(
		func() (dynamic.Interface, *meta.RESTMapper, error) {
			return refreshed, &refreshedMapper, nil
		},
	)

	require.NoError(t, err)
	assert.Same(t, refreshed, client)
	assert.Same(t, &refreshedMapper, mapper)
	assert.Same(t, refreshed, u.dynamicKubeClient)
	assert.Same(t, &refreshedMapper, u.mapper)
}
