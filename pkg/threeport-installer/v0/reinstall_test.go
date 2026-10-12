package v0

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
)

// testNamespaceMapper returns a mapper that resolves a core namespace.
func testNamespaceMapper() meta.RESTMapper {
	// register the core v1 group
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})

	// map Namespace at cluster scope
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)

	return mapper
}

// testNamespace returns a namespace, labeled with tier when one is given.
func testNamespace(name, tier string) *unstructured.Unstructured {
	// build a core namespace
	namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}

	// set the tier label when the case supplies one
	if tier != "" {
		namespace.SetLabels(map[string]string{LabelTier: tier})
	}

	return namespace
}

// testStatefulSet returns a StatefulSet in the given namespace.
func testStatefulSet(name, namespace string) *unstructured.Unstructured {
	// build a StatefulSet with no spec
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

// testVolumeClaim returns a persistent volume claim in the given namespace.
func testVolumeClaim(name, namespace string) *unstructured.Unstructured {
	// build a claim with no spec
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

// testManagedDeployment returns an installer-managed Deployment with no
// ready replicas, so a scale-down wait succeeds on the first poll.
func testManagedDeployment(name, namespace string) *unstructured.Unstructured {
	// build a Deployment with one desired replica and no ready replicas
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
			},
		},
	}

	// label it so deployment scale-down selects it
	deployment.SetLabels(map[string]string{LabelManagedBy: LabelManagedByValue})

	return deployment
}

// testKubeClient returns a dynamic client seeded with the given objects.
func testKubeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	// start an empty scheme
	scheme := runtime.NewScheme()

	// register list kinds so the fake client can list each resource
	for _, listKind := range []schema.GroupVersionKind{
		{Version: "v1", Kind: "NamespaceList"},
		{Version: "v1", Kind: "PersistentVolumeClaimList"},
		{Group: "apps", Version: "v1", Kind: "DeploymentList"},
		{Group: "apps", Version: "v1", Kind: "StatefulSetList"},
		{Group: "batch", Version: "v1", Kind: "JobList"},
	} {
		scheme.AddKnownTypeWithName(listKind, &unstructured.UnstructuredList{})
	}

	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

// succeedDropJob records a created job and marks it succeeded.
// The create stays unhandled so the fake client still stores the job.
func succeedDropJob(kubeClient *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	// hold the job the reactor sees
	created := &unstructured.Unstructured{}

	// mark the created job succeeded without handling the create
	kubeClient.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// skip a create that is not an unstructured object
		job, ok := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if !ok {
			return false, nil, nil
		}

		// record the submitted job and set one success
		created.Object = job.Object
		if err := unstructured.SetNestedField(job.Object, int64(1), "status", "succeeded"); err != nil {
			return false, nil, err
		}

		// leave the create unhandled so the object tracker stores the job
		return false, nil, nil
	})

	return created
}

// allowDeploymentScaleDown records each patched deployment name.
// Its reactor returns that deployment at zero replicas and handles the patch.
func allowDeploymentScaleDown(kubeClient *dynamicfake.FakeDynamicClient, scaled *[]string) {
	// answer deployment patches with a zero-replica object
	kubeClient.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// record the patched deployment
		patch := action.(k8stesting.PatchAction)
		*scaled = append(*scaled, patch.GetName())

		// handle the patch because unstructured strategic merge fails on the fake client
		deployment := testManagedDeployment(patch.GetName(), patch.GetNamespace())
		if err := unstructured.SetNestedField(deployment.Object, int64(0), "spec", "replicas"); err != nil {
			return true, nil, err
		}

		return true, deployment, nil
	})
}

// TestDropDatabaseRejectsNonDevelopmentControlPlane rejects a database drop
// when the namespace is not labeled development.
func TestDropDatabaseRejectsNonDevelopmentControlPlane(t *testing.T) {
	// list the tiers a drop must refuse
	tests := []struct {
		name string
		// The tier label, empty when the namespace records none
		tier string
	}{
		{
			name: "production tier is refused",
			tier: ControlPlaneTierProd,
		},
		{
			name: "unrecognized tier is refused",
			tier: "staging",
		},
		{
			name: "absent tier is refused",
			tier: "",
		},
	}

	// refuse each non-development tier
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// build a control plane at that tier with a managed api server
			namespace := "threeport-control-plane"
			kubeClient := testKubeClient(
				testNamespace(namespace, test.tier),
				testManagedDeployment("threeport-api-server", namespace),
			)
			var scaled []string
			allowDeploymentScaleDown(kubeClient, &scaled)
			mapper := testNamespaceMapper()
			cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

			// drop the database
			err := cpi.DropDatabase(kubeClient, &mapper)

			// assert the drop is refused as not development
			if err == nil {
				t.Fatal("expected drop to be refused, got nil error")
			}
			if !errors.Is(err, ErrControlPlaneNotDevelopment) {
				t.Errorf("expected error to match ErrControlPlaneNotDevelopment, got: %v", err)
			}

			// assert nothing was scaled down
			if len(scaled) > 0 {
				t.Errorf("expected no deployment to be scaled down on a refused drop, got %v", scaled)
			}

			// assert no drop job was created
			if _, getErr := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
				context.Background(), dropDatabaseJobName, metav1.GetOptions{},
			); getErr == nil {
				t.Error("expected no database drop job to be created on a refused drop")
			}
		})
	}
}

// TestDropDatabaseRunsDropStatementOnDevelopmentControlPlane accepts a database
// drop on a development control plane and checks the statement and survivors.
func TestDropDatabaseRunsDropStatementOnDevelopmentControlPlane(t *testing.T) {
	// build a development control plane with its database objects
	namespace := "threeport-control-plane"
	kubeClient := testKubeClient(
		testNamespace(namespace, ControlPlaneTierDev),
		testManagedDeployment("threeport-api-server", namespace),
		testStatefulSet("crdb", namespace),
		testStatefulSet("nats-js", namespace),
		testVolumeClaim("datadir-crdb-0", namespace),
	)
	var scaled []string
	allowDeploymentScaleDown(kubeClient, &scaled)
	createdJob := succeedDropJob(kubeClient)
	mapper := testNamespaceMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

	// drop the database
	if err := cpi.DropDatabase(kubeClient, &mapper); err != nil {
		t.Fatalf("expected drop to succeed on a development control plane, got: %v", err)
	}

	// assert only the api server deployment was scaled down
	if len(scaled) != 1 || scaled[0] != "threeport-api-server" {
		t.Errorf("expected the api server deployment to be scaled down, got %v", scaled)
	}

	// assert the statement cancels paused schema jobs before the drop
	statement := dropDatabaseJobStatement(t, createdJob)
	for _, want := range []string{
		"CANCEL JOBS",
		"paused",
		"NEW SCHEMA CHANGE",
		"DROP DATABASE",
		database.ThreeportDatabaseName,
		"CASCADE",
	} {
		if !strings.Contains(statement, want) {
			t.Errorf("expected the drop statement to contain %q, got %q", want, statement)
		}
	}

	// assert the cancel precedes the drop
	if strings.Index(statement, "CANCEL JOBS") > strings.Index(statement, "DROP DATABASE") {
		t.Errorf("expected the cancel to precede the drop, got %q", statement)
	}

	// assert the finished job is removed
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
		context.Background(), dropDatabaseJobName, metav1.GetOptions{},
	); err == nil {
		t.Error("expected the database drop job to be removed once it succeeded")
	}

	// assert the database and message broker stores survive
	for _, survivor := range []struct {
		gvr  schema.GroupVersionResource
		name string
	}{
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, name: "crdb"},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, name: "nats-js"},
		{gvr: schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, name: "datadir-crdb-0"},
	} {
		if _, err := kubeClient.Resource(survivor.gvr).Namespace(namespace).Get(
			context.Background(), survivor.name, metav1.GetOptions{},
		); err != nil {
			t.Errorf("expected %s/%s to survive the drop, got: %v", survivor.gvr.Resource, survivor.name, err)
		}
	}
}

// TestDropDatabaseReportsFailedJob covers a drop job that fails past its backoff limit.
func TestDropDatabaseReportsFailedJob(t *testing.T) {
	// build a development namespace with no deployments
	namespace := "threeport-control-plane"
	kubeClient := testKubeClient(testNamespace(namespace, ControlPlaneTierDev))

	// mark the created job failed past the backoff limit
	kubeClient.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// skip a create that is not an unstructured object
		job, ok := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if !ok {
			return false, nil, nil
		}

		// set failed pods one past the backoff limit
		if err := unstructured.SetNestedField(
			job.Object, int64(dropDatabaseJobBackoffLimit+1), "status", "failed",
		); err != nil {
			return false, nil, err
		}

		// leave the create unhandled so the object tracker stores the job
		return false, nil, nil
	})
	mapper := testNamespaceMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

	// drop the database
	err := cpi.DropDatabase(kubeClient, &mapper)

	// assert the error names the namespace to inspect
	if err == nil {
		t.Fatal("expected a failed drop job to be reported, got nil error")
	}
	if !strings.Contains(err.Error(), namespace) {
		t.Errorf("expected the error to name the namespace to inspect, got: %v", err)
	}
}

// TestDropMessageBrokerStateRejectsNonDevelopmentControlPlane rejects a stream
// drop when the namespace is not labeled development.
func TestDropMessageBrokerStateRejectsNonDevelopmentControlPlane(t *testing.T) {
	// refuse production, an unrecognized tier, and a missing tier
	for _, tier := range []string{ControlPlaneTierProd, "staging", ""} {
		t.Run(fmt.Sprintf("tier %q is refused", tier), func(t *testing.T) {
			// build a control plane namespace at that tier
			namespace := "threeport-control-plane"
			kubeClient := testKubeClient(testNamespace(namespace, tier))
			mapper := testNamespaceMapper()
			cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

			// drop message broker state
			err := cpi.DropMessageBrokerState(kubeClient, &mapper)

			// assert the drop is refused as not development
			if err == nil {
				t.Fatal("expected drop to be refused, got nil error")
			}
			if !errors.Is(err, ErrControlPlaneNotDevelopment) {
				t.Errorf("expected error to match ErrControlPlaneNotDevelopment, got: %v", err)
			}

			// assert no drop job was created
			if _, getErr := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
				context.Background(), dropMessageBrokerJobName, metav1.GetOptions{},
			); getErr == nil {
				t.Error("expected no message broker drop job to be created on a refused drop")
			}
		})
	}
}

// TestDropMessageBrokerStateRemovesEveryStreamOnDevelopmentControlPlane accepts
// a stream drop on a development control plane and checks the script and survivors.
func TestDropMessageBrokerStateRemovesEveryStreamOnDevelopmentControlPlane(t *testing.T) {
	// build a development control plane with message broker objects
	namespace := "threeport-control-plane"
	kubeClient := testKubeClient(
		testNamespace(namespace, ControlPlaneTierDev),
		testStatefulSet("nats-js", namespace),
		testVolumeClaim("datadir-nats-js-0", namespace),
	)
	createdJob := succeedDropJob(kubeClient)
	mapper := testNamespaceMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

	// drop message broker state
	if err := cpi.DropMessageBrokerState(kubeClient, &mapper); err != nil {
		t.Fatalf("expected drop to succeed on a development control plane, got: %v", err)
	}

	// assert the script lists streams instead of naming them
	script := dropJobShellScript(t, createdJob)
	for _, want := range []string{"nats stream ls --names", "nats stream rm", "--force"} {
		if !strings.Contains(script, want) {
			t.Errorf("expected the drop script to contain %q, got %q", want, script)
		}
	}

	// assert the finished job is removed
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
		context.Background(), dropMessageBrokerJobName, metav1.GetOptions{},
	); err == nil {
		t.Error("expected the message broker drop job to be removed once it succeeded")
	}

	// assert the message broker store survives
	for _, survivor := range []struct {
		gvr  schema.GroupVersionResource
		name string
	}{
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, name: "nats-js"},
		{gvr: schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, name: "datadir-nats-js-0"},
	} {
		if _, err := kubeClient.Resource(survivor.gvr).Namespace(namespace).Get(
			context.Background(), survivor.name, metav1.GetOptions{},
		); err != nil {
			t.Errorf("expected %s/%s to survive the drop, got: %v", survivor.gvr.Resource, survivor.name, err)
		}
	}
}

// dropJobShellScript returns the shell script a drop job runs.
func dropJobShellScript(t *testing.T, job *unstructured.Unstructured) string {
	t.Helper()

	// read the container command
	command := dropJobCommand(t, job)

	// return the argument after -c
	for i, arg := range command {
		if arg == "-c" && i+1 < len(command) {
			return command[i+1]
		}
	}

	// fail when the command does not run a script
	t.Fatalf("expected the command to interpret a script, got %v", command)

	return ""
}

// dropDatabaseJobStatement returns the SQL statement a database drop job runs.
func dropDatabaseJobStatement(t *testing.T, job *unstructured.Unstructured) string {
	t.Helper()

	// read the container command
	command := dropJobCommand(t, job)

	// return the argument after --execute
	for i, arg := range command {
		if arg == "--execute" && i+1 < len(command) {
			return command[i+1]
		}
	}

	// fail when the command does not execute a statement
	t.Fatalf("expected the command to execute a statement, got %v", command)

	return ""
}

// dropJobCommand returns the command of a drop job's first container.
func dropJobCommand(t *testing.T, job *unstructured.Unstructured) []string {
	t.Helper()

	// read the job's containers
	containers, found, err := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		t.Fatalf("expected the drop job to define a container, got found=%v err=%v", found, err)
	}

	// take the first container
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected the drop job's container to be an object")
	}

	// read that container's command
	command, found, err := unstructured.NestedStringSlice(container, "command")
	if err != nil || !found {
		t.Fatalf("expected the drop job's container to define a command, got found=%v err=%v", found, err)
	}

	return command
}
