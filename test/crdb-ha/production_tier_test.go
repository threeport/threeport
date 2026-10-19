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
	"k8s.io/apimachinery/pkg/types"
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

// sqlTimeout bounds every SQL call. A write that blocks until the lost
// replica comes back would otherwise pass as one that survived its loss.
const sqlTimeout = 30 * time.Second

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
		waitForPodReady(t, pod, "")
	}

	t.Run("reads and writes continue through a pod loss, and data survives the rejoin", func(t *testing.T) {
		mustExecSQL(t, "crdb-0",
			"CREATE DATABASE verify_test; "+
				"CREATE TABLE verify_test.t (id INT PRIMARY KEY, val STRING); "+
				"INSERT INTO verify_test.t VALUES (1, 'before-kill');",
		)

		victim, err := typedClient.CoreV1().Pods(namespace).Get(context.Background(), "crdb-1", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get crdb-1: %v", err)
		}
		oldUID := victim.UID
		victimNode := victim.Spec.NodeName

		// Cordoning the node keeps the replacement Pending for as long as the
		// outage needs to last: its volume is bound to that node, and the
		// required anti-affinity rules out the other two. Without it, the
		// StatefulSet controller races the assertions below.
		setUnschedulable(t, victimNode, true)
		t.Cleanup(func() { setUnschedulable(t, victimNode, false) })

		if err := typedClient.CoreV1().Pods(namespace).Delete(
			context.Background(), "crdb-1", metav1.DeleteOptions{},
		); err != nil {
			t.Fatalf("failed to delete crdb-1: %v", err)
		}

		// a graceful delete leaves the old pod running for up to its grace
		// period; its object is only gone once the container has stopped
		waitForReplacementPending(t, "crdb-1", oldUID)

		out := mustExecSQL(t, "crdb-0",
			"INSERT INTO verify_test.t VALUES (2, 'during-outage'); SELECT count(*) FROM verify_test.t;",
		)
		if !bytes.Contains([]byte(out), []byte("2")) {
			t.Fatalf("expected a write during the outage to be readable back, got: %s", out)
		}

		// the write has to have happened inside the outage, not after it
		assertUnscheduled(t, "crdb-1")

		setUnschedulable(t, victimNode, false)
		waitForPodReady(t, "crdb-1", oldUID)

		// read through the rejoined node itself, not only a survivor
		out = mustExecSQL(t, "crdb-1", "SELECT val FROM verify_test.t ORDER BY id;")
		for _, want := range []string{"before-kill", "during-outage"} {
			if !bytes.Contains([]byte(out), []byte(want)) {
				t.Errorf("expected %q to survive the rejoin, got: %s", want, out)
			}
		}
	})
}

// setUnschedulable cordons or uncordons a node.
func setUnschedulable(t *testing.T, node string, unschedulable bool) {
	t.Helper()

	patch := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, unschedulable)
	if _, err := typedClient.CoreV1().Nodes().Patch(
		context.Background(), node, types.MergePatchType, []byte(patch), metav1.PatchOptions{},
	); err != nil {
		t.Fatalf("failed to set unschedulable=%t on %s: %v", unschedulable, node, err)
	}
}

// waitForReplacementPending polls until the pod with oldUID is gone and its
// StatefulSet replacement exists without a node.
func waitForReplacementPending(t *testing.T, name string, oldUID types.UID) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		pod, err := typedClient.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil && pod.UID != oldUID {
			if pod.Spec.NodeName != "" {
				t.Fatalf("%s's replacement was scheduled to %s while its node was cordoned", name, pod.Spec.NodeName)
			}
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("%s (uid %s) never went away", name, oldUID)
}

// assertUnscheduled fails the test if a pod has been given a node.
func assertUnscheduled(t *testing.T, name string) {
	t.Helper()

	pod, err := typedClient.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get %s: %v", name, err)
	}
	if pod.Spec.NodeName != "" {
		t.Fatalf("%s was back on %s before the outage write was checked", name, pod.Spec.NodeName)
	}
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

// waitForPodReady polls until a pod reports Ready. A pod with notUID, or one
// already being deleted, does not count - so a caller waiting on a
// replacement cannot be satisfied by the pod it replaced.
func waitForPodReady(t *testing.T, name string, notUID types.UID) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		pod, err := typedClient.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil && pod.UID != notUID && pod.DeletionTimestamp == nil {
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

	ctx, cancel := context.WithTimeout(context.Background(), sqlTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		t.Fatalf("cockroach sql against %s failed: %v\nstdout: %s\nstderr: %s", pod, err, stdout.String(), stderr.String())
	}

	return stdout.String()
}
