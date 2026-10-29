package v0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// moduleRegistryApiServer is a threeport API stand-in that lists registered
// module APIs and the controller deployments that belong to each.
type moduleRegistryApiServer struct {
	// The controller deployment names keyed by module API ID, as namespace and name joined by a slash
	controllersByApiId map[uint][]string
	// The registered module APIs
	apis []v0.ModuleApi
}

// serve starts a test API server and returns a client and an API address.
func (s *moduleRegistryApiServer) serve(t *testing.T) (*http.Client, string) {
	t.Helper()

	// serve module API and controller list routes
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// set the response content type
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasPrefix(r.URL.Path, v0.PathModuleApis):
			// list registered module APIs
			data := []apiserver_lib.Object{}
			for i := range s.apis {
				data = append(data, s.apis[i])
			}
			s.write(t, w, data)
		case strings.HasPrefix(r.URL.Path, v0.PathModuleControllers):
			// list controllers for the queried module API
			apiId := uint(0)
			for _, api := range s.apis {
				if api.ID == nil {
					continue
				}
				if strings.Contains(r.URL.RawQuery, fmt.Sprintf("moduleapiid=%d", *api.ID)) {
					apiId = *api.ID
					break
				}
			}
			data := []apiserver_lib.Object{}
			for _, deploymentName := range s.controllersByApiId[apiId] {
				name := deploymentName
				data = append(data, v0.ModuleController{DeploymentName: &name})
			}
			s.write(t, w, data)
		default:
			// fail the test on an unexpected path
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	// close the server when the test ends
	t.Cleanup(server.Close)

	// strip the scheme; API requests prepend one
	return &http.Client{}, strings.TrimPrefix(server.URL, "http://")
}

// write encodes a successful API list response.
func (s *moduleRegistryApiServer) write(t *testing.T, w http.ResponseWriter, data []apiserver_lib.Object) {
	t.Helper()

	// write the ok status and the encoded object list
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(apiserver_lib.Response{Data: data}); err != nil {
		t.Errorf("failed to encode response: %v", err)
	}
}

// testModuleApi returns a module API. A true core flag marks the control
// plane's own API; a false flag leaves Core nil.
func testModuleApi(id uint, name string, core bool) v0.ModuleApi {
	// set the id and the name
	moduleApi := v0.ModuleApi{
		Common: v0.Common{ID: &id},
		Name:   &name,
	}
	// set Core only when core is true
	if core {
		isCore := true
		moduleApi.Core = &isCore
	}

	return moduleApi
}

// testModuleApiWithEndpoint is testModuleApi plus the cluster-local endpoint
// the installer writes for a real module, in the same
// <service>.<namespace>.svc.cluster.local shape.
func testModuleApiWithEndpoint(id uint, name, serviceName, namespace string) v0.ModuleApi {
	return testModuleApiWithEndpointAndCore(id, name, serviceName, namespace, false)
}

// testModuleApiWithEndpointAndCore is testModuleApiWithEndpoint with an
// explicit Core flag, for the one case that needs it set true at a namespace
// other than the control plane's own.
func testModuleApiWithEndpointAndCore(id uint, name, serviceName, namespace string, core bool) v0.ModuleApi {
	moduleApi := testModuleApi(id, name, core)
	endpoint := serviceName + "." + namespace + ".svc.cluster.local"
	moduleApi.Endpoint = &endpoint

	return moduleApi
}

// testControllerDeployment returns a module controller deployment.
// It omits the installer managed-by label.
func testControllerDeployment(name, namespace string, replicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": replicas,
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.kubernetes.io/name": name,
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":    "controller",
								"command": []interface{}{"/example-controller"},
							},
						},
					},
				},
			},
		},
	}
}

// testModuleDeployment returns a namespaced Deployment with spec.replicas
// and no ready replicas, so the scale-down wait succeeds on the first poll.
func testModuleDeployment(name, namespace string, replicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					LabelManagedBy: LabelManagedByValue,
				},
			},
			"spec": map[string]interface{}{
				"replicas": replicas,
			},
		},
	}
}

// recordDeploymentScaling records each replica patch as namespace, name, and replica count.
func recordDeploymentScaling(kubeClient *dynamicfake.FakeDynamicClient, patched *[]string) {
	// intercept deployment patch requests
	kubeClient.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch := action.(k8stesting.PatchAction)

		// read the replica count from the patch body
		var replicas struct {
			Spec struct {
				Replicas int64 `json:"replicas"`
			} `json:"spec"`
		}
		if err := json.Unmarshal(patch.GetPatch(), &replicas); err != nil {
			return true, nil, err
		}
		// record namespace, name, and replica count
		*patched = append(*patched, fmt.Sprintf(
			"%s/%s=%d", patch.GetNamespace(), patch.GetName(), replicas.Spec.Replicas,
		))

		// handle the patch; strategic merge on unstructured fails in the fake client
		return true, testModuleDeployment(patch.GetName(), patch.GetNamespace(), replicas.Spec.Replicas), nil
	})
}

// TestDiscoverModuleDeploymentsReadsTheRegistry covers the deployments the
// registry lists for non-core module controllers.
func TestDiscoverModuleDeploymentsReadsTheRegistry(t *testing.T) {
	// seed registered module APIs and controller deployments
	apiServer := &moduleRegistryApiServer{
		apis: []v0.ModuleApi{
			testModuleApi(1, "threeport", true),
			testModuleApi(2, "example-module", false),
			testModuleApi(3, "other-module", false),
		},
		controllersByApiId: map[uint][]string{
			1: {"threeport-control-plane/threeport-secret-controller"},
			2: {
				"example-namespace/threeport-example-controller",
				"example-namespace/threeport-second-controller",
			},
			3: {"other-namespace/threeport-other-controller"},
		},
	}
	apiClient, apiAddr := apiServer.serve(t)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// discover module deployments
	deployments, err := cpi.DiscoverModuleDeployments(apiClient, apiAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// check non-core deployments in registry order
	want := []string{
		"example-namespace/threeport-example-controller",
		"example-namespace/threeport-second-controller",
		"other-namespace/threeport-other-controller",
	}
	if len(deployments) != len(want) {
		t.Fatalf("expected deployments %v, got %v", want, deployments)
	}
	for i, deployment := range deployments {
		if got := deployment.Namespace + "/" + deployment.Name; got != want[i] {
			t.Errorf("expected deployment %s at position %d, got %s", want[i], i, got)
		}
	}
}

// TestDiscoverModuleDeploymentsExcludesTheControlPlane covers discovery
// omitting a non-core controller in the control plane namespace.
func TestDiscoverModuleDeploymentsExcludesTheControlPlane(t *testing.T) {
	// seed a non-core controller in the control plane namespace
	apiServer := &moduleRegistryApiServer{
		apis: []v0.ModuleApi{testModuleApi(2, "example-module", false)},
		controllersByApiId: map[uint][]string{
			2: {"threeport-control-plane/threeport-example-controller"},
		},
	}
	apiClient, apiAddr := apiServer.serve(t)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// discover module deployments
	deployments, err := cpi.DiscoverModuleDeployments(apiClient, apiAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// check that discovery omits the control plane namespace
	if len(deployments) != 0 {
		t.Errorf("expected the control plane's own namespace to be excluded, got %v", deployments)
	}
}

// TestScaleDownModulesAndRestore covers scaling a registered module controller
// to zero and restoring its recorded replica count.
func TestScaleDownModulesAndRestore(t *testing.T) {
	// seed a module API server and a controller in the same namespace
	namespace := "example-namespace"
	kubeClient := testKubeClient(
		// the module API server is not a registered controller
		testModuleDeployment("threeport-example-rest-api", namespace, 1),
		testControllerDeployment("threeport-example-controller", namespace, 2),
	)
	var patched []string
	recordDeploymentScaling(kubeClient, &patched)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// scale the registered controller to zero
	scales, err := cpi.ScaleDownModules(kubeClient, []ModuleDeploymentScale{{
		Namespace: namespace,
		Name:      "threeport-example-controller",
	}})
	if err != nil {
		t.Fatalf("unexpected error scaling down: %v", err)
	}

	// check that scale-down records only the registered controller
	if len(scales) != 1 || scales[0].Name != "threeport-example-controller" || scales[0].Replicas != 2 {
		t.Fatalf("expected the registered controller to be recorded, got %v", scales)
	}
	if !containsString(patched, "example-namespace/threeport-example-controller=0") {
		t.Errorf("expected scale-down patch for the controller, got %v", patched)
	}
	for _, unwanted := range patched {
		if strings.Contains(unwanted, "threeport-example-rest-api") {
			t.Errorf("expected the unregistered deployment to be left alone, got patch %s", unwanted)
		}
	}

	// restore recorded replica counts
	patched = nil
	if err := cpi.RestoreModuleScale(kubeClient, scales); err != nil {
		t.Fatalf("unexpected error restoring: %v", err)
	}

	// check that restore puts the controller back at its replica count
	if !containsString(patched, "example-namespace/threeport-example-controller=2") {
		t.Errorf("expected restore patch for the controller, got %v", patched)
	}
}

// TestScaleDownModulesSkipsDeploymentsAlreadyStopped covers scale-down
// skipping a zero-replica deployment.
func TestScaleDownModulesSkipsDeploymentsAlreadyStopped(t *testing.T) {
	// seed a running controller and a zero-replica controller
	namespace := "example-namespace"
	kubeClient := testKubeClient(
		testControllerDeployment("threeport-example-controller", namespace, 1),
		testControllerDeployment("threeport-disabled-controller", namespace, 0),
	)
	var patched []string
	recordDeploymentScaling(kubeClient, &patched)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// scale the named deployments to zero
	scales, err := cpi.ScaleDownModules(kubeClient, []ModuleDeploymentScale{
		{Namespace: namespace, Name: "threeport-example-controller"},
		{Namespace: namespace, Name: "threeport-disabled-controller"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// check that scale-down records only the running deployment
	if len(scales) != 1 || scales[0].Name != "threeport-example-controller" {
		t.Errorf("expected only the running deployment to be recorded, got %v", scales)
	}
	// check that scale-down does not patch the stopped deployment
	for _, unwanted := range patched {
		if strings.Contains(unwanted, "threeport-disabled-controller") {
			t.Errorf("expected the stopped deployment to be left alone, got patch %s", unwanted)
		}
	}
}

// TestScaleDownModulesLeavesOtherWorkloadsRunning covers a registered name
// that points at a workload other than a module controller.
func TestScaleDownModulesLeavesOtherWorkloadsRunning(t *testing.T) {
	// seed a controller and two workloads that are not controllers
	namespace := "example-namespace"
	kubeClient := testKubeClient(
		testControllerDeployment("threeport-example-controller", namespace, 2),
		testModuleDeployment("threeport-example-rest-api", namespace, 1),
		testModuleDeployment("payments-controller", namespace, 1),
	)
	var patched []string
	recordDeploymentScaling(kubeClient, &patched)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// scale the registered names
	scales, err := cpi.ScaleDownModules(kubeClient, []ModuleDeploymentScale{
		{Namespace: namespace, Name: "threeport-example-controller"},
		{Namespace: namespace, Name: "threeport-example-rest-api"},
		{Namespace: namespace, Name: "payments-controller"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// check that scale-down records only the controller
	if len(scales) != 1 || scales[0].Name != "threeport-example-controller" || scales[0].Replicas != 2 {
		t.Fatalf("expected only the controller to be recorded, got %v", scales)
	}
	if !containsString(patched, "example-namespace/threeport-example-controller=0") {
		t.Errorf("expected scale-down patch for the controller, got %v", patched)
	}
	for _, unwanted := range patched {
		if strings.Contains(unwanted, "threeport-example-rest-api") || strings.Contains(unwanted, "payments-controller") {
			t.Errorf("expected the other workload to be left alone, got patch %s", unwanted)
		}
	}
}

// TestScaleDownModulesReturnsCountsWhenALaterScaleFails covers an error after
// an earlier controller was already scaled to zero.
func TestScaleDownModulesReturnsCountsWhenALaterScaleFails(t *testing.T) {
	// seed two controllers and fail the second replica patch
	namespace := "example-namespace"
	kubeClient := testKubeClient(
		testControllerDeployment("threeport-example-controller", namespace, 2),
		testControllerDeployment("threeport-second-controller", namespace, 3),
	)
	var patched []string
	recordDeploymentScaling(kubeClient, &patched)
	kubeClient.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch := action.(k8stesting.PatchAction)
		if patch.GetName() == "threeport-second-controller" {
			return true, nil, fmt.Errorf("patch failed")
		}
		return false, nil, nil
	})
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// scale both controllers
	scales, err := cpi.ScaleDownModules(kubeClient, []ModuleDeploymentScale{
		{Namespace: namespace, Name: "threeport-example-controller"},
		{Namespace: namespace, Name: "threeport-second-controller"},
	})

	// check that the error still returns the earlier count
	if err == nil {
		t.Fatal("expected scale-down to fail")
	}
	if len(scales) != 1 || scales[0].Name != "threeport-example-controller" || scales[0].Replicas != 2 {
		t.Fatalf("expected the earlier controller count, got %v", scales)
	}
}

// TestScaleDownModulesReturnsCountsWhenALaterReadFails covers an error reading
// a later deployment after an earlier controller was scaled to zero.
func TestScaleDownModulesReturnsCountsWhenALaterReadFails(t *testing.T) {
	// seed two controllers and fail the second read
	namespace := "example-namespace"
	kubeClient := testKubeClient(
		testControllerDeployment("threeport-example-controller", namespace, 2),
		testControllerDeployment("threeport-second-controller", namespace, 3),
	)
	var patched []string
	recordDeploymentScaling(kubeClient, &patched)
	kubeClient.PrependReactor("get", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		get := action.(k8stesting.GetAction)
		if get.GetName() == "threeport-second-controller" {
			return true, nil, fmt.Errorf("get failed")
		}
		return false, nil, nil
	})
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// scale both controllers
	scales, err := cpi.ScaleDownModules(kubeClient, []ModuleDeploymentScale{
		{Namespace: namespace, Name: "threeport-example-controller"},
		{Namespace: namespace, Name: "threeport-second-controller"},
	})

	// check that the error still returns the earlier count
	if err == nil {
		t.Fatal("expected scale-down to fail")
	}
	if len(scales) != 1 || scales[0].Name != "threeport-example-controller" || scales[0].Replicas != 2 {
		t.Fatalf("expected the earlier controller count, got %v", scales)
	}
}

// TestRestoreModuleScaleToleratesRemovedDeployment covers restore skipping
// a missing deployment.
func TestRestoreModuleScaleToleratesRemovedDeployment(t *testing.T) {
	// seed an empty cluster
	namespace := "example-namespace"
	kubeClient := testKubeClient()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// restore a scale for a missing deployment
	err := cpi.RestoreModuleScale(kubeClient, []ModuleDeploymentScale{
		{Namespace: namespace, Name: "threeport-uninstalled-controller", Replicas: 1},
	})
	// check that restore succeeds
	if err != nil {
		t.Fatalf("expected a removed deployment to be skipped, got: %v", err)
	}

	// check that restore does not create the missing deployment
	if _, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(), "threeport-uninstalled-controller", metav1.GetOptions{},
	); err == nil {
		t.Error("expected the removed deployment to stay removed")
	}
}

// containsString reports whether want is present in values.
func containsString(values []string, want string) bool {
	// return true on an exact match
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// recordNamespaceLabelling records each namespace patch as name and the
// labels it carried, and optionally fails one of them.
func recordNamespaceLabelling(
	kubeClient *dynamicfake.FakeDynamicClient,
	patched *map[string]string,
	failFor string,
) {
	kubeClient.PrependReactor("patch", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch := action.(k8stesting.PatchAction)
		if patch.GetName() == failFor {
			return true, nil, fmt.Errorf("namespaces %q is forbidden", failFor)
		}

		var labels struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(patch.GetPatch(), &labels); err != nil {
			return true, nil, err
		}
		for key, value := range labels.Metadata.Labels {
			(*patched)[patch.GetName()] = key + "=" + value
		}

		return true, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"name": patch.GetName()},
		}}, nil
	})
}

// TestLabelModuleNamespacesMarksWhatThePolicyAdmits covers the modules that
// were installed before the database network policy existed.
//
// The SDK stamps the label onto namespaces it creates, so a module generated
// against a current SDK carries it. One installed before does not, and nothing
// regenerates it on its own - so applying the policy would cut it away from
// its own database, and the module would fail long after the install reported
// success.
func TestLabelModuleNamespacesMarksWhatThePolicyAdmits(t *testing.T) {
	cpi := testInstaller("dev-0")
	want := LabelModule + "=" + LabelModuleValue

	t.Run("every module namespace is marked", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		patched := map[string]string{}
		recordNamespaceLabelling(kubeClient, &patched, "")

		require.NoError(t, cpi.LabelModuleNamespaces(kubeClient, []string{
			"threeport-test",
			"threeport-other",
		}))
		assert.Equal(t, map[string]string{
			"threeport-test":  want,
			"threeport-other": want,
		}, patched)
	})

	t.Run("a namespace holding two controllers is patched once", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		patches := 0
		kubeClient.PrependReactor("patch", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
			patches++

			return true, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1", "kind": "Namespace",
			}}, nil
		})

		require.NoError(t, cpi.LabelModuleNamespaces(kubeClient, []string{
			"threeport-test",
			"threeport-test",
		}))
		assert.Equal(t, 1, patches, "the same namespace listed twice is still patched once")
	})

	t.Run("a namespace that is gone is not a failure", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		kubeClient.PrependReactor("patch", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, k8serrors.NewNotFound(
				schema.GroupResource{Resource: "namespaces"}, action.(k8stesting.PatchAction).GetName(),
			)
		})

		require.NoError(
			t, cpi.LabelModuleNamespaces(kubeClient, []string{"threeport-gone"}),
			"the registry outlives what it describes, and a namespace already removed needs no label",
		)
	})

	t.Run("one namespace that cannot be labelled does not hide the others", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		patched := map[string]string{}
		recordNamespaceLabelling(kubeClient, &patched, "threeport-forbidden")

		err := cpi.LabelModuleNamespaces(kubeClient, []string{
			"threeport-forbidden",
			"threeport-fine",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "threeport-forbidden")
		assert.Equal(
			t, map[string]string{"threeport-fine": want}, patched,
			"stopping at the first refusal would leave the rest unlabelled and unreported",
		)
	})

	t.Run("no modules is not an error", func(t *testing.T) {
		kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

		assert.NoError(t, cpi.LabelModuleNamespaces(kubeClient, nil))
	})
}

// TestDiscoverModuleNamespacesCoversAModuleWithNoControllers covers what
// DiscoverModuleDeployments cannot see.
//
// The SDK supports a module with no reconcilable objects, which registers no
// ModuleController. DiscoverModuleDeployments finds namespaces only through
// those, so such a module's namespace never surfaces through it - and it
// would be left unlabelled when the database network policy applies,
// cutting its API server off from its own data. Every module still
// registers exactly one ModuleApi, and the installer's endpoint for it
// already names the namespace.
func TestDiscoverModuleNamespacesCoversAModuleWithNoControllers(t *testing.T) {
	apiServer := &moduleRegistryApiServer{
		apis: []v0.ModuleApi{
			testModuleApiWithEndpoint(1, "threeport", "threeport-api-server", "threeport-control-plane"),
			testModuleApiWithEndpoint(2, "example-module", "example-api-server", "example-namespace"),
			testModuleApiWithEndpoint(3, "controller-free-module", "controller-free-api-server", "controller-free-namespace"),
		},
		controllersByApiId: map[uint][]string{
			2: {"example-namespace/threeport-example-controller"},
			// module 3 registers no controllers at all
		},
	}
	apiClient, apiAddr := apiServer.serve(t)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
	require.NoError(t, err)
	assert.ElementsMatch(
		t, []string{"example-namespace", "controller-free-namespace"}, namespaces,
		"a module with no controllers is still a module with a database to protect",
	)
	assert.NotContains(
		t, namespaces, "threeport-control-plane",
		"the core API's own namespace is not a module's, Core flag or not",
	)
}

// TestDiscoverModuleNamespacesDeduplicates covers two modules that happen to
// share a namespace - or the same module registering twice.
func TestDiscoverModuleNamespacesDeduplicates(t *testing.T) {
	apiServer := &moduleRegistryApiServer{
		apis: []v0.ModuleApi{
			testModuleApiWithEndpoint(1, "first-api", "first-api-server", "shared-namespace"),
			testModuleApiWithEndpoint(2, "second-api", "second-api-server", "shared-namespace"),
		},
	}
	apiClient, apiAddr := apiServer.serve(t)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
	require.NoError(t, err)
	assert.Equal(
		t, []string{"shared-namespace"}, namespaces,
		"the same namespace reached through two module APIs is still one namespace to label",
	)
}

// TestDiscoverModuleNamespacesExcludesTheCoreApiAndControlPlane covers the
// two namespaces that are never a module's to label.
func TestDiscoverModuleNamespacesExcludesTheCoreApiAndControlPlane(t *testing.T) {
	t.Run("the core API's own entry is not a module", func(t *testing.T) {
		apiServer := &moduleRegistryApiServer{
			apis: []v0.ModuleApi{
				testModuleApiWithEndpoint(1, "threeport", "threeport-api-server", "threeport-control-plane"),
				testModuleApiWithEndpoint(2, "example-module", "example-api-server", "example-namespace"),
			},
		}
		apiClient, apiAddr := apiServer.serve(t)
		cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

		namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
		require.NoError(t, err)
		assert.Equal(
			t, []string{"example-namespace"}, namespaces,
			"the core API's own namespace is the control plane's, not a module's - its Core flag says so even "+
				"if a module somehow shared its endpoint shape",
		)
	})

	t.Run("a core entry at some other namespace is still excluded by its flag", func(t *testing.T) {
		// the namespace here is deliberately NOT the control plane's own, so
		// only the Core flag - not the namespace exclusion below - can be
		// what keeps this out
		apiServer := &moduleRegistryApiServer{
			apis: []v0.ModuleApi{
				testModuleApiWithEndpointAndCore(1, "threeport", "threeport-api-server", "unrelated-namespace", true),
			},
		}
		apiClient, apiAddr := apiServer.serve(t)
		cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

		namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
		require.NoError(t, err)
		assert.Empty(t, namespaces, "Core is what marks this as the threeport API itself, not a module")
	})

	t.Run("a non-core entry pointed at the control plane namespace is not labelled", func(t *testing.T) {
		// Core unset but Endpoint happens to resolve to the control plane's
		// own namespace - the exclusion has to hold on the namespace itself,
		// not only on trusting the Core flag
		apiServer := &moduleRegistryApiServer{
			apis: []v0.ModuleApi{
				testModuleApiWithEndpoint(1, "not-flagged-core", "something", "threeport-control-plane"),
			},
		}
		apiClient, apiAddr := apiServer.serve(t)
		cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

		namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
		require.NoError(t, err)
		assert.Empty(
			t, namespaces,
			"labelling the control plane's own namespace as a module's is not what the policy expects of it",
		)
	})
}

// TestModuleNamespaceFromEndpoint covers the one assumption the whole
// function rests on: what the installer actually writes.
func TestModuleNamespaceFromEndpoint(t *testing.T) {
	t.Run("the installer's own shape is read", func(t *testing.T) {
		endpoint := "example-api-server.example-namespace.svc.cluster.local"
		namespace, ok := moduleNamespaceFromEndpoint(&endpoint)
		assert.True(t, ok)
		assert.Equal(t, "example-namespace", namespace)
	})

	t.Run("a nil endpoint is not a namespace", func(t *testing.T) {
		_, ok := moduleNamespaceFromEndpoint(nil)
		assert.False(t, ok)
	})

	t.Run("an external endpoint is not guessed at", func(t *testing.T) {
		endpoint := "https://modules.example.com"
		_, ok := moduleNamespaceFromEndpoint(&endpoint)
		assert.False(t, ok, "admitting the wrong namespace to the database is worse than leaving one unlabelled")
	})

	t.Run("a service name with a dot in it is not mistaken for one with a namespace", func(t *testing.T) {
		endpoint := "example.svc.cluster.local"
		_, ok := moduleNamespaceFromEndpoint(&endpoint)
		assert.False(t, ok, "this has only one label before the suffix, not a service and a namespace")
	})
}
