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

// testNamespaceMapper returns a REST mapper that knows the Namespace kind.
func testNamespaceMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)

	return mapper
}

// testNamespace returns a Namespace labeled with tier. An empty tier produces no labels.
func testNamespace(name, tier string) *unstructured.Unstructured {
	namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	if tier != "" {
		namespace.SetLabels(map[string]string{LabelTier: tier})
	}

	return namespace
}

// testStatefulSet returns a namespaced StatefulSet.
func testStatefulSet(name, namespace string) *unstructured.Unstructured {
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

// testVolumeClaim returns a namespaced PersistentVolumeClaim.
func testVolumeClaim(name, namespace string) *unstructured.Unstructured {
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
// ready replicas, so scale-down wait succeeds on the first poll.
func testManagedDeployment(name, namespace string) *unstructured.Unstructured {
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
	deployment.SetLabels(map[string]string{LabelManagedBy: LabelManagedByValue})

	return deployment
}

// testKubeClient returns a fake dynamic client seeded with objects.
func testKubeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
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

// succeedDropJob captures the created drop job and marks it succeeded.
func succeedDropJob(kubeClient *dynamicfake.FakeDynamicClient) *unstructured.Unstructured {
	created := &unstructured.Unstructured{}

	kubeClient.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job, ok := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if !ok {
			return false, nil, nil
		}
		// capture the job and mark it succeeded
		created.Object = job.Object
		if err := unstructured.SetNestedField(job.Object, int64(1), "status", "succeeded"); err != nil {
			return false, nil, err
		}

		// leave handled false so the tracker still stores the job
		return false, nil, nil
	})

	return created
}

// allowDeploymentScaleDown records each patched deployment and returns it scaled to zero.
func allowDeploymentScaleDown(kubeClient *dynamicfake.FakeDynamicClient, scaled *[]string) {
	kubeClient.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch := action.(k8stesting.PatchAction)
		*scaled = append(*scaled, patch.GetName())

		// handle the patch; unstructured strategic merge fails on the fake
		deployment := testManagedDeployment(patch.GetName(), patch.GetNamespace())
		if err := unstructured.SetNestedField(deployment.Object, int64(0), "spec", "replicas"); err != nil {
			return true, nil, err
		}

		return true, deployment, nil
	})
}

// TestDropDatabaseRejectsNonDevelopmentControlPlane covers a drop that refuses
// a control plane it cannot confirm is development, and that the drop touches nothing.
func TestDropDatabaseRejectsNonDevelopmentControlPlane(t *testing.T) {
	tests := []struct {
		name string
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// seed a non-development control plane the drop would otherwise act on
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

			// check the refusal is the not-development error
			if err == nil {
				t.Fatal("expected drop to be refused, got nil error")
			}
			if !errors.Is(err, ErrControlPlaneNotDevelopment) {
				t.Errorf("expected error to match ErrControlPlaneNotDevelopment, got: %v", err)
			}

			// check no deployment was scaled down
			if len(scaled) > 0 {
				t.Errorf("expected no deployment to be scaled down on a refused drop, got %v", scaled)
			}
			// check no drop job was created
			if _, getErr := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
				context.Background(), dropDatabaseJobName, metav1.GetOptions{},
			); getErr == nil {
				t.Error("expected no database drop job to be created on a refused drop")
			}
		})
	}
}

// TestDropDatabaseRunsDropStatementOnDevelopmentControlPlane covers a
// successful development-tier drop that leaves the database and message broker stores in place.
func TestDropDatabaseRunsDropStatementOnDevelopmentControlPlane(t *testing.T) {
	// seed a development control plane with its api server, database, and message broker
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

	// check the api server was scaled down
	if len(scaled) != 1 || scaled[0] != "threeport-api-server" {
		t.Errorf("expected the api server deployment to be scaled down, got %v", scaled)
	}

	// check the statement cancels paused schema changes then drops
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

	// check the cancel precedes the drop
	if strings.Index(statement, "CANCEL JOBS") > strings.Index(statement, "DROP DATABASE") {
		t.Errorf("expected the cancel to precede the drop, got %q", statement)
	}

	// check success deletes the drop job
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
		context.Background(), dropDatabaseJobName, metav1.GetOptions{},
	); err == nil {
		t.Error("expected the database drop job to be removed once it succeeded")
	}

	// check the database and message broker stores survive
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

// TestDropDatabaseReportsFailedJob covers a drop job that exceeds its retry
// budget and names the namespace to inspect.
func TestDropDatabaseReportsFailedJob(t *testing.T) {
	// seed a development control plane
	namespace := "threeport-control-plane"
	kubeClient := testKubeClient(testNamespace(namespace, ControlPlaneTierDev))

	// fail the drop job past its backoff limit
	kubeClient.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job, ok := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if !ok {
			return false, nil, nil
		}
		if err := unstructured.SetNestedField(
			job.Object, int64(dropDatabaseJobBackoffLimit+1), "status", "failed",
		); err != nil {
			return false, nil, err
		}

		// leave handled false so the tracker still stores the job
		return false, nil, nil
	})
	mapper := testNamespaceMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

	// drop the database
	err := cpi.DropDatabase(kubeClient, &mapper)
	if err == nil {
		t.Fatal("expected a failed drop job to be reported, got nil error")
	}
	// check the error names the namespace to inspect
	if !strings.Contains(err.Error(), namespace) {
		t.Errorf("expected the error to name the namespace to inspect, got: %v", err)
	}
}

// TestDropMessageBrokerStateRejectsNonDevelopmentControlPlane covers a
// message broker drop that refuses a control plane it cannot confirm is development.
func TestDropMessageBrokerStateRejectsNonDevelopmentControlPlane(t *testing.T) {
	for _, tier := range []string{ControlPlaneTierProd, "staging", ""} {
		t.Run(fmt.Sprintf("tier %q is refused", tier), func(t *testing.T) {
			// seed a control plane at a non-development tier
			namespace := "threeport-control-plane"
			kubeClient := testKubeClient(testNamespace(namespace, tier))
			mapper := testNamespaceMapper()
			cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}

			// drop message broker state
			err := cpi.DropMessageBrokerState(kubeClient, &mapper)

			// check the refusal is the not-development error
			if err == nil {
				t.Fatal("expected drop to be refused, got nil error")
			}
			if !errors.Is(err, ErrControlPlaneNotDevelopment) {
				t.Errorf("expected error to match ErrControlPlaneNotDevelopment, got: %v", err)
			}

			// check no drop job was created
			if _, getErr := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
				context.Background(), dropMessageBrokerJobName, metav1.GetOptions{},
			); getErr == nil {
				t.Error("expected no message broker drop job to be created on a refused drop")
			}
		})
	}
}

// TestDropMessageBrokerStateRemovesEveryStreamOnDevelopmentControlPlane covers
// a development-tier drop of every stream that leaves the broker in place.
func TestDropMessageBrokerStateRemovesEveryStreamOnDevelopmentControlPlane(t *testing.T) {
	// seed a development control plane with its message broker
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

	// check the script enumerates streams rather than naming them
	script := dropJobShellScript(t, createdJob)
	for _, want := range []string{"nats stream ls --names", "nats stream rm", "--force"} {
		if !strings.Contains(script, want) {
			t.Errorf("expected the drop script to contain %q, got %q", want, script)
		}
	}

	// check success deletes the drop job
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
		context.Background(), dropMessageBrokerJobName, metav1.GetOptions{},
	); err == nil {
		t.Error("expected the message broker drop job to be removed once it succeeded")
	}

	// check the message broker store survives
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

// dropJobShellScript returns the shell script the drop job runs.
func dropJobShellScript(t *testing.T, job *unstructured.Unstructured) string {
	t.Helper()

	command := dropJobCommand(t, job)

	// return the script after -c
	for i, arg := range command {
		if arg == "-c" && i+1 < len(command) {
			return command[i+1]
		}
	}

	t.Fatalf("expected the command to interpret a script, got %v", command)

	return ""
}

// dropDatabaseJobStatement returns the SQL the drop job executes.
func dropDatabaseJobStatement(t *testing.T, job *unstructured.Unstructured) string {
	t.Helper()

	command := dropJobCommand(t, job)

	// return the statement after --execute
	for i, arg := range command {
		if arg == "--execute" && i+1 < len(command) {
			return command[i+1]
		}
	}

	t.Fatalf("expected the command to execute a statement, got %v", command)

	return ""
}

// dropJobCommand returns the drop job's container command.
func dropJobCommand(t *testing.T, job *unstructured.Unstructured) []string {
	t.Helper()

	containers, found, err := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		t.Fatalf("expected the drop job to define a container, got found=%v err=%v", found, err)
	}

	container, ok := containers[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected the drop job's container to be an object")
	}

	command, found, err := unstructured.NestedStringSlice(container, "command")
	if err != nil || !found {
		t.Fatalf("expected the drop job's container to define a command, got found=%v err=%v", found, err)
	}

	return command
}
