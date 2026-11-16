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
	if err := removeReplacedNamespace(kube, &rest, installer.ControlPlaneNamespace, "primary-05", true); err != nil {
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
	if err := removeReplacedNamespace(kube, &rest, "primary-02", "primary-05", true); err != nil {
		t.Fatal(err)
	}
	_, err = kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(
		t.Context(), "primary-02", metav1.GetOptions{},
	)
	if err == nil {
		t.Fatal("dedicated source namespace stayed")
	}
}

// TestAChildKeepsTheOperatorRunningInTheKeptNamespace covers a replaced control
// plane that did not take the operator over: its StatefulSets stop and the
// operator keeps running.
func TestAChildKeepsTheOperatorRunningInTheKeptNamespace(t *testing.T) {
	// run the operator and a database in the original namespace
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	var rest meta.RESTMapper = mapper
	kube := operatorCluster()

	// remove the namespace of a control plane that does not run the operator
	if err := removeReplacedNamespace(kube, &rest, installer.ControlPlaneNamespace, "primary-05", false); err != nil {
		t.Fatal(err)
	}

	// assert the operator keeps its replica and the database stops
	for resource, want := range map[string]struct {
		gvr      schema.GroupVersionResource
		name     string
		replicas int64
	}{
		"operator": {deployGVR(), installer.SupportServicesOperatorDeployName, 1},
		"database": {statefulSetGVR(), "crdb", 0},
	} {
		object, err := kube.Resource(want.gvr).Namespace(installer.ControlPlaneNamespace).Get(
			t.Context(), want.name, metav1.GetOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if replicas, _, _ := unstructured.NestedInt64(object.Object, "spec", "replicas"); replicas != want.replicas {
			t.Fatalf("%s replicas = %d, want %d", resource, replicas, want.replicas)
		}
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

// TestGrantSourceDatabaseGrantsTheSourceAccount covers the grant before a
// blue-green backup: the database account of the source namespace is granted,
// by its UID, and a source with no database account is refused before any grant.
func TestGrantSourceDatabaseGrantsTheSourceAccount(t *testing.T) {
	// a source namespace whose database account has a UID
	account := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]interface{}{
			"name":      installer.DatabaseServiceAccountName,
			"namespace": "primary-01",
			"uid":       "d5b617f3-30c6-450a-b10b-b55cad429306",
		},
	}}
	kube := fake.NewSimpleDynamicClient(runtime.NewScheme(), account)
	var granted []string
	grant := func(uid string) error {
		granted = append(granted, uid)
		return nil
	}

	// grant the source database, then a namespace with no database account
	source := installer.NewInstaller(installer.Namespace("primary-01"))
	if err := grantSourceDatabase(source, kube, grant); err != nil {
		t.Fatal(err)
	}
	missing := installer.NewInstaller(installer.Namespace("primary-02"))
	if err := grantSourceDatabase(missing, kube, grant); err == nil {
		t.Fatal("a source with no database account must be refused")
	}

	// assert only the source's account was granted
	if len(granted) != 1 || granted[0] != "d5b617f3-30c6-450a-b10b-b55cad429306" {
		t.Fatalf("granted = %v, want the source database account's UID once", granted)
	}
}
