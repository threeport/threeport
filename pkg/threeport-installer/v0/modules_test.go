package v0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

// TestDiscoverModuleNamespacesReadsTheRegistry covers unique namespaces the
// registry lists for non-core module controllers.
func TestDiscoverModuleNamespacesReadsTheRegistry(t *testing.T) {
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

	// discover module namespaces
	namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// check unique non-core namespaces in registry order
	want := []string{"example-namespace", "other-namespace"}
	if len(namespaces) != len(want) {
		t.Fatalf("expected namespaces %v, got %v", want, namespaces)
	}
	for i, namespace := range want {
		if namespaces[i] != namespace {
			t.Errorf("expected namespace %s at position %d, got %s", namespace, i, namespaces[i])
		}
	}
}

// TestDiscoverModuleNamespacesExcludesTheControlPlane covers discovery
// omitting a non-core controller in the control plane namespace.
func TestDiscoverModuleNamespacesExcludesTheControlPlane(t *testing.T) {
	// seed a non-core controller in the control plane namespace
	apiServer := &moduleRegistryApiServer{
		apis: []v0.ModuleApi{testModuleApi(2, "example-module", false)},
		controllersByApiId: map[uint][]string{
			2: {"threeport-control-plane/threeport-example-controller"},
		},
	}
	apiClient, apiAddr := apiServer.serve(t)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// discover module namespaces
	namespaces, err := cpi.DiscoverModuleNamespaces(apiClient, apiAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// check that discovery omits the control plane namespace
	if len(namespaces) != 0 {
		t.Errorf("expected the control plane's own namespace to be excluded, got %v", namespaces)
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
		testModuleDeployment("threeport-example-controller", namespace, 2),
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
	// seed a running deployment and a zero-replica deployment
	namespace := "example-namespace"
	kubeClient := testKubeClient(
		testModuleDeployment("threeport-example-rest-api", namespace, 1),
		testModuleDeployment("threeport-disabled-controller", namespace, 0),
	)
	var patched []string
	recordDeploymentScaling(kubeClient, &patched)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: "threeport-control-plane"}}

	// scale the named deployments to zero
	scales, err := cpi.ScaleDownModules(kubeClient, []ModuleDeploymentScale{
		{Namespace: namespace, Name: "threeport-example-rest-api"},
		{Namespace: namespace, Name: "threeport-disabled-controller"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// check that scale-down records only the running deployment
	if len(scales) != 1 || scales[0].Name != "threeport-example-rest-api" {
		t.Errorf("expected only the running deployment to be recorded, got %v", scales)
	}
	// check that scale-down does not patch the stopped deployment
	for _, unwanted := range patched {
		if strings.Contains(unwanted, "threeport-disabled-controller") {
			t.Errorf("expected the stopped deployment to be left alone, got patch %s", unwanted)
		}
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
