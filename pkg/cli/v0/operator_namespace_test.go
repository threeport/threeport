package v0

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	api "github.com/threeport/threeport/pkg/api/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestUpgradeStopsWhatRunsInTheKeptNamespace calls the namespace removal the
// upgrade uses. The original namespace stays with its operator and its
// StatefulSets at zero, and a dedicated namespace is deleted.
func TestUpgradeStopsWhatRunsInTheKeptNamespace(t *testing.T) {
	// run the operator and a database in the original namespace and a workload in a dedicated one
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	var rest meta.RESTMapper = mapper
	kube := operatorCluster()
	if err := removeReplacedNamespace(kube, &rest, installer.ControlPlaneNamespace, "primary-05"); err != nil {
		t.Fatal(err)
	}

	// the operator stays in the original namespace, stopped
	operator, err := kube.Resource(deployGVR()).Namespace(installer.ControlPlaneNamespace).Get(
		t.Context(), installer.SupportServicesOperatorDeployName, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if replicas, _, _ := unstructured.NestedInt64(operator.Object, "spec", "replicas"); replicas != 0 {
		t.Fatalf("original namespace operator replicas = %d, want 0", replicas)
	}

	// the replaced control plane's StatefulSets in the original namespace are stopped
	statefulSet, err := kube.Resource(statefulSetGVR()).Namespace(installer.ControlPlaneNamespace).Get(
		t.Context(), "crdb", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if replicas, _, _ := unstructured.NestedInt64(statefulSet.Object, "spec", "replicas"); replicas != 0 {
		t.Fatalf("original namespace database replicas = %d, want 0", replicas)
	}

	// a dedicated source namespace is removed
	if err := removeReplacedNamespace(kube, &rest, "primary-02", "primary-05"); err != nil {
		t.Fatal(err)
	}
	_, err = kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(
		t.Context(), "primary-02", metav1.GetOptions{},
	)
	if err == nil {
		t.Fatal("dedicated source namespace stayed")
	}
}

// operatorCluster returns a cluster with the operator and a dedicated namespace.
func operatorCluster() dynamic.Interface {
	// seed the operator and the dedicated namespace
	scheme := runtime.NewScheme()
	objects := []runtime.Object{
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"name": installer.ControlPlaneNamespace},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"name": "primary-02"},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      installer.SupportServicesOperatorDeployName,
				"namespace": installer.ControlPlaneNamespace,
			},
			"spec": map[string]interface{}{"replicas": int64(1)},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      "crdb",
				"namespace": installer.ControlPlaneNamespace,
			},
			"spec": map[string]interface{}{"replicas": int64(3)},
		}},
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			{Version: "v1", Resource: "namespaces"}: "NamespaceList",
			deployGVR():                             "DeploymentList",
			statefulSetGVR():                        "StatefulSetList",
		},
		objects...,
	)
}

// deployGVR is the apps deployment resource the operator test reads.
func deployGVR() schema.GroupVersionResource {
	// name the deployment resource
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
}

// statefulSetGVR is the apps StatefulSet resource the operator test reads.
func statefulSetGVR() schema.GroupVersionResource {
	// name the StatefulSet resource
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
}

// TestOnlyAControlPlaneWithNoParentRunsSupportServices covers which control
// planes get the support services operator in a blue-green upgrade: one with no
// parent does, and a child, which shares its parent's, does not.
func TestOnlyAControlPlaneWithNoParentRunsSupportServices(t *testing.T) {
	// a control plane with no parent, a child, and no control plane at all
	genesis := &api.ControlPlaneInstance{}
	child := &api.ControlPlaneInstance{ParentControlPlaneInstanceID: util.Ptr(uint(1))}

	// assert only the one with no parent runs the operator
	if !runsSupportServices(genesis) {
		t.Fatal("a control plane with no parent must run the operator")
	}
	if runsSupportServices(child) {
		t.Fatal("a child control plane must not run the operator")
	}
	if runsSupportServices(nil) {
		t.Fatal("a missing control plane must not run the operator")
	}
}
