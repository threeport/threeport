package crdbha

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	auth "github.com/threeport/threeport/pkg/auth/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// namespace is where InstallThreeportControlPlaneDependencies writes
// everything this package exercises.
const namespace = "threeport-control-plane"

// crdbPodGVR is the pods resource, used to read scheduling and delete a pod
// directly - there is no higher-level helper for either in this codebase,
// since nothing else needs to reach into a running CockroachDB pod.
var crdbPodGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

// TestProductionTierCockroachDBSurvivesANodeLoss covers #602's own
// acceptance criteria against a real scheduler: a 3-node cluster that
// tolerates a single pod loss without data loss or downtime, and rejoins
// cleanly afterward.
//
// This is the one guarantee a fake dynamic client cannot stand in for - it
// does not schedule anything, so it cannot tell a required anti-affinity
// rule that is honored from one a future change quietly turned into a
// preference again. Only a real scheduler, deciding where three pods
// actually land, can.
func TestProductionTierCockroachDBSurvivesANodeLoss(t *testing.T) {
	cpi := threeport.NewInstaller()
	cpi.Opts.Namespace = namespace
	cpi.Opts.Name = "crdb-ha-test"
	cpi.Opts.Tier = threeport.ControlPlaneTier(threeport.ControlPlaneTierProd)
	// InstallThreeportControlPlaneDependencies does not itself call
	// ValidateTierForProvider - that check lives at the CLI layer, in
	// ValidateCreateGenesisControlPlaneFlags - so this reaches the
	// production manifests directly on a kind cluster, which a real
	// install path never could and is not meant to.
	cpi.Opts.InfraProvider = "kind"

	dbCreds, err := auth.GenerateDbCreds(namespace)
	if err != nil {
		t.Fatalf("failed to generate db creds: %v", err)
	}

	if err := cpi.InstallThreeportControlPlaneDependencies(
		dynamicClient, &restMapper, "01234567890123456789012345678901", dbCreds,
	); err != nil {
		t.Fatalf("failed to install control plane dependencies: %v", err)
	}

	podNames := []string{"crdb-0", "crdb-1", "crdb-2"}

	t.Run("three replicas land on three distinct nodes", func(t *testing.T) {
		nodes := map[string]string{}
		for _, pod := range podNames {
			node := waitForPodScheduled(t, pod)
			nodes[pod] = node
		}

		seen := map[string]bool{}
		for pod, node := range nodes {
			if seen[node] {
				t.Errorf(
					"%s shares node %s with another replica - the required anti-affinity did not hold",
					pod, node,
				)
			}
			seen[node] = true
		}
	})

	for _, pod := range podNames {
		waitForPodReady(t, pod)
	}

	t.Run("reads and writes continue through a pod loss, and data survives the rejoin", func(t *testing.T) {
		mustExecSQL(t, "crdb-0",
			"CREATE DATABASE verify_test; "+
				"CREATE TABLE verify_test.t (id INT PRIMARY KEY, val STRING); "+
				"INSERT INTO verify_test.t VALUES (1, 'before-kill');",
		)

		if err := dynamicClient.Resource(crdbPodGVR).Namespace(namespace).Delete(
			context.Background(), "crdb-1", metav1.DeleteOptions{},
		); err != nil {
			t.Fatalf("failed to delete crdb-1: %v", err)
		}

		// immediately, not after waiting for anything to settle - this is
		// the moment the issue's own acceptance criteria is about
		out := mustExecSQL(t, "crdb-0",
			"INSERT INTO verify_test.t VALUES (2, 'during-outage'); SELECT count(*) FROM verify_test.t;",
		)
		if !bytes.Contains([]byte(out), []byte("2")) {
			t.Fatalf("expected a write during the outage to be readable back, got: %s", out)
		}

		waitForPodReady(t, "crdb-1")

		out = mustExecSQL(t, "crdb-0", "SELECT val FROM verify_test.t ORDER BY id;")
		for _, want := range []string{"before-kill", "during-outage"} {
			if !bytes.Contains([]byte(out), []byte(want)) {
				t.Errorf("expected %q to survive the rejoin, got: %s", want, out)
			}
		}
	})
}

// waitForPodScheduled polls until a pod has a node assigned, and returns it.
func waitForPodScheduled(t *testing.T, name string) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		pod, err := dynamicClient.Resource(crdbPodGVR).Namespace(namespace).Get(
			context.Background(), name, metav1.GetOptions{},
		)
		if err == nil {
			if node, found, _ := unstructured.NestedString(pod.Object, "spec", "nodeName"); found && node != "" {
				return node
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("%s was never scheduled to a node", name)
	return ""
}

// waitForPodReady polls until a pod reports Ready.
func waitForPodReady(t *testing.T, name string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		pod, err := typedClient.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == "True" {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("%s never became ready", name)
}

// mustExecSQL runs a SQL statement inside a pod's db container via
// `cockroach sql`, the same binary and certs path every manifest in this
// package already uses, and fails the test on any error - including one
// CockroachDB itself reported.
func mustExecSQL(t *testing.T, pod, statement string) string {
	t.Helper()

	req := typedClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: "db",
		Command: []string{
			"/cockroach/cockroach", "sql",
			"--certs-dir=/cockroach/cockroach-certs",
			fmt.Sprintf("--host=%s.crdb.%s.svc.cluster.local", pod, namespace),
			"--execute", statement,
		},
		Stdout: true,
		Stderr: true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		t.Fatalf("failed to build exec request against %s: %v", pod, err)
	}

	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		t.Fatalf("cockroach sql against %s failed: %v\nstdout: %s\nstderr: %s", pod, err, stdout.String(), stderr.String())
	}

	return stdout.String()
}
