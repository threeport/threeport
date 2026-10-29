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
func testUninstaller(namespacePreexisted, controlPlaneOnly bool, infra *fakeKubernetesRuntimeInfra) *Uninstaller {
	cpi := &threeport.ControlPlaneInstaller{}
	cpi.Opts.Namespace = "threeport-control-plane"
	cpi.Opts.NamespacePreexisted = namespacePreexisted
	cpi.Opts.ControlPlaneOnly = controlPlaneOnly

	namespace := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]interface{}{"name": "threeport-control-plane"},
	}}
	kubeClient := dynamic.Interface(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), namespace))
	mapper := meta.RESTMapper(testNamespaceMapper())

	return &Uninstaller{
		controlPlane:           &threeport.ControlPlane{InfraProvider: v0.KubernetesRuntimeInfraProviderKind},
		kubernetesRuntimeInfra: infra,
		dynamicKubeClient:      &kubeClient,
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
	t.Run("a full install still tears down infra when the namespace predates it", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(true, false, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.Contains(
			t, err.Error(), "already present",
			"the namespace note still has to reach the operator, just not at the cost of the infra below it",
		)
		assert.True(
			t, infra.deleteCalled,
			"returning early here is what left EKS/VPC/IAM resources running on a --force-overwrite-config retry",
		)
	})

	t.Run("control-plane-only still leaves infra alone, preexisting namespace or not", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(true, true, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.False(t, infra.deleteCalled, "there is no infra to tear down for a control-plane-only reapply")
	})

	t.Run("a namespace this install created is still torn down in full", func(t *testing.T) {
		infra := &fakeKubernetesRuntimeInfra{}
		u := testUninstaller(false, false, infra)

		err := u.cleanOnCreateError("", fmt.Errorf("create failed"))

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "already present", "this install owns the namespace, so there is nothing to excuse")
		assert.True(t, infra.deleteCalled)
	})
}
