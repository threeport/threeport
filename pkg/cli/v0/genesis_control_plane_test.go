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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// fakeKubernetesRuntimeInfra is a KubernetesRuntimeInfra that records whether
// Delete was called, without touching a real cloud account.
type fakeKubernetesRuntimeInfra struct {
	deleteCalled bool
	deleteErr    error
}

func (f *fakeKubernetesRuntimeInfra) Create() (*kube.KubeConnectionInfo, error) {
	return nil, nil
}

func (f *fakeKubernetesRuntimeInfra) Delete() error {
	f.deleteCalled = true

	return f.deleteErr
}

// testNamespaceMapper returns a mapper that resolves a core namespace,
// matching the one threeport-installer/v0's own tests use for the same
// DeleteNamespaces call this one reaches.
func testNamespaceMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)

	return mapper
}

// testUninstaller builds an Uninstaller that cleanOnCreateError can run
// against without a cloud account: the kind provider skips the AWS/OCI
// branch of the switch below the infra delete. The Kubernetes client is a
// fake carrying the control plane's own namespace, since the no-preexisting-
// namespace case reaches a real DeleteNamespaces call through
// UnInstallThreeportControlPlaneComponents.
func testUninstaller(namespacePresence threeport.NamespacePresence, controlPlaneOnly bool, infra *fakeKubernetesRuntimeInfra) *Uninstaller {
	cpi := &threeport.ControlPlaneInstaller{}
	cpi.Opts.Namespace = "threeport-control-plane"
	cpi.Opts.NamespacePresence = namespacePresence
	cpi.Opts.ControlPlaneOnly = controlPlaneOnly

	namespace := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]interface{}{"name": "threeport-control-plane"},
	}}
	kubeClient := dynamic.Interface(newTeardownKubeClient(namespace))
	mapper := meta.RESTMapper(testNamespaceMapper())

	return &Uninstaller{
		controlPlane:           &threeport.ControlPlane{InfraProvider: v0.KubernetesRuntimeInfraProviderKind},
		kubernetesRuntimeInfra: infra,
		dynamicKubeClient:      kubeClient,
		mapper:                 &mapper,
		cleanConfig:            util.Ptr(false),
		cpi:                    cpi,
		teardownOnFailure:      util.Ptr(true),
	}
}

// TestCleanOnCreateErrorSkipsOnlyTheNamespaceDeletion covers what lander2k2
// found on #625: the guard against deleting a namespace this install did not
// create was returning from the whole function, not just the one step that
// could destroy it.
//
// A namespace left alone is right for --control-plane-only, which reapplies
// into a namespace a running control plane already owns. It is not a reason
// to skip the infra teardown on a full install's retry: the same preexisting
// namespace can show up there too, by way of --force-overwrite-config reusing
// a name whose earlier attempt got partway through creating the namespace
// before failing.
func TestCleanOnCreateErrorSkipsOnlyTheNamespaceDeletion(t *testing.T) {
	t.Run("a full install deletes the namespace and the infra when the namespace predates it", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(threeport.NamespacePresent, false, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.True(
			t, namespaceDeleteRequested(t, u),
			"the cluster is deleted below, so keeping the namespace keeps nothing - it only leaves the API's load balancer behind, which on EKS blocks the stack's subnets",
		)
		assert.NotContains(t, err.Error(), "already present", "the namespace was not left alone")
		assert.True(
			t, infra.deleteCalled,
			"returning early here is what left EKS/VPC/IAM resources running on a --force-overwrite-config retry",
		)
	})

	t.Run("control-plane-only leaves a namespace that predates it, and the infra, alone", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(threeport.NamespacePresent, true, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.False(t, namespaceDeleteRequested(t, u), "the running control plane's API, database and data are in it")
		assert.Contains(t, err.Error(), "already present", "the operator is told why it is still there")
		assert.False(t, infra.deleteCalled, "there is no infra to tear down for a control-plane-only reapply")
	})

	t.Run("a namespace this install created is still torn down in full", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(threeport.NamespaceAbsent, false, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.True(t, namespaceDeleteRequested(t, u))
		assert.NotContains(t, err.Error(), "already present", "this install owns the namespace, so there is nothing to excuse")
		assert.True(t, infra.deleteCalled)
	})

	t.Run("control-plane-only leaves a namespace whose presence it never found out alone", func(t *testing.T) {
		// an install that fails after it has a Kubernetes client but before
		// it checks for the namespace, or whose check fails, can't tell a
		// namespace it created from a running control plane's
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(threeport.NamespacePresenceUnknown, true, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.False(t, namespaceDeleteRequested(t, u), "it may be a running control plane's")
		assert.Contains(t, err.Error(), "before it could find out whether it created the threeport-control-plane namespace")
		assert.NotContains(t, err.Error(), "already present", "it does not know that")
		assert.False(t, infra.deleteCalled)
	})

	t.Run("a full install deletes a namespace whose presence it never found out, with the cluster", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(threeport.NamespacePresenceUnknown, false, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.True(t, namespaceDeleteRequested(t, u), "the cluster goes below, and the namespace with it")
		assert.True(t, infra.deleteCalled)
	})

	t.Run("control-plane-only still deletes a namespace it created", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(threeport.NamespaceAbsent, true, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.True(t, namespaceDeleteRequested(t, u))
		assert.False(t, infra.deleteCalled)
	})
}

// namespaceDeleteRequested reports whether the uninstaller asked its fake
// cluster to delete a namespace.
func namespaceDeleteRequested(t *testing.T, u *Uninstaller) bool {
	t.Helper()
	client, ok := u.dynamicKubeClient.(*dynamicfake.FakeDynamicClient)
	require.True(t, ok)
	for _, action := range client.Actions() {
		if action.Matches("delete", "namespaces") {
			return true
		}
	}

	return false
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

// newTeardownKubeClient returns a fake cluster holding objects that the
// uninstall can run against. The uninstall lists the persistent volumes
// behind the control plane's claims before deleting its namespace, and the
// fake client can list only the kinds it is told about.
func newTeardownKubeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Version: "v1", Resource: "namespaces"}:        "NamespaceList",
			{Version: "v1", Resource: "persistentvolumes"}: "PersistentVolumeList",
		},
		objects...,
	)
}

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
	client := newTeardownKubeClient()
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
	client := newTeardownKubeClient()
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
	earlier := newTeardownKubeClient()
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
	earlier := newTeardownKubeClient()
	u, _ := newTestUninstaller(earlier)

	refreshed := newTeardownKubeClient()
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
