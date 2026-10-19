package v0

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// TestCrdbReplicasMatchesTheTier covers the one number tier gates directly.
func TestCrdbReplicasMatchesTheTier(t *testing.T) {
	if got := crdbReplicas(ControlPlaneTierDev); got != 1 {
		t.Errorf("expected development to run a single node, got %d", got)
	}
	if got := crdbReplicas(ControlPlaneTierProd); got != 3 {
		t.Errorf("expected production to run three nodes, got %d", got)
	}
}

// TestCrdbStartCommandMatchesTheTier covers the command each tier actually
// starts with, not just its replica count.
//
// start-single-node and start --join are different startup paths, not two
// sizes of the same one: a node brought up with start-single-node can never
// later join a multi-node cluster. A command that silently defaulted to one
// shape regardless of tier would not fail at install - the development path
// would simply never be able to scale into a real cluster, and nothing would
// say so until someone tried.
func TestCrdbStartCommandMatchesTheTier(t *testing.T) {
	dev := crdbStartCommand(ControlPlaneTierDev)
	if !strings.Contains(dev, "start-single-node") {
		t.Errorf("expected development to use start-single-node, got %q", dev)
	}
	if strings.Contains(dev, "--join") {
		t.Errorf("start-single-node does not take --join, got %q", dev)
	}

	prod := crdbStartCommand(ControlPlaneTierProd)
	if strings.Contains(prod, "start-single-node") {
		t.Errorf("production must not use start-single-node, got %q", prod)
	}
	if !strings.Contains(prod, "cockroach start ") {
		t.Errorf("expected production to use the multi-node start command, got %q", prod)
	}
	for _, pod := range []string{"crdb-0", "crdb-1", "crdb-2"} {
		if !strings.Contains(prod, pod) {
			t.Errorf("expected --join to name %s, got %q", pod, prod)
		}
	}
}

// TestCrdbInitRunsOnlyForProduction covers the tier gate itself: the init
// job is the one step with no equivalent on the development path, so it has
// to run exactly when the tier that needs it is the one being installed.
func TestCrdbInitRunsOnlyForProduction(t *testing.T) {
	for _, tc := range []struct {
		tier     ControlPlaneTier
		wantInit bool
	}{
		{ControlPlaneTierDev, false},
		{ControlPlaneTierProd, true},
	} {
		t.Run(string(tc.tier), func(t *testing.T) {
			namespace := "threeport-control-plane"
			kubeClient := testKubeClient()
			createdJob := succeedDropJob(kubeClient)
			cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace, Tier: tc.tier}}

			if err := cpi.installCrdbForTier(kubeClient); err != nil {
				t.Fatalf("expected the install step to succeed, got: %v", err)
			}

			ran := createdJob.Object != nil
			if ran != tc.wantInit {
				t.Errorf("tier %s: expected init job run=%v, got %v", tc.tier, tc.wantInit, ran)
			}
		})
	}
}

// TestCrdbInitJobIsIdempotentOnAnAlreadyInitializedCluster covers the
// retry/reinstall path: a production install run twice must not fail the
// second time just because the cluster answered the first one.
//
// The script's idempotency was confirmed against a running CockroachDB
// container rather than assumed: a first `cockroach init` reports "Cluster
// successfully initialized", and a second against the same cluster exits 1
// and reports "ERROR: cluster has already been initialized" - the exact text
// crdbAlreadyInitializedMarker matches. This test cannot run that binary, so
// it asserts the job carries the marker and the certs an already-initialized
// cluster needs to answer when it is reached.
func TestCrdbInitJobIsIdempotentOnAnAlreadyInitializedCluster(t *testing.T) {
	namespace := "threeport-control-plane"
	kubeClient := testKubeClient()
	createdJob := succeedDropJob(kubeClient)
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace, Tier: ControlPlaneTierProd}}

	if err := cpi.runCrdbInitJob(kubeClient, namespace); err != nil {
		t.Fatalf("expected the init job to succeed, got: %v", err)
	}

	script := crdbInitJobScript(t, createdJob)
	for _, want := range []string{
		crdbAlreadyInitializedMarker,
		"exit 0",
		dbRootCertsMountPath,
		"crdb-0.crdb." + namespace + ".svc.cluster.local",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("expected the init script to contain %q, got %q", want, script)
		}
	}

	// the job mounts the root cert, not the node cert this StatefulSet's own
	// pods use - a cluster that has never seen the node cert still has to
	// recognize this job's client certificate to answer "already initialized"
	volumes, _, err := unstructured.NestedSlice(createdJob.Object, "spec", "template", "spec", "volumes")
	if err != nil || len(volumes) != 1 {
		t.Fatalf("expected one volume on the init job, got %v (err: %v)", volumes, err)
	}
	secretName, _, _ := unstructured.NestedString(volumes[0].(map[string]interface{}), "secret", "secretName")
	if secretName != dbRootCertSecretName {
		t.Errorf("expected the init job to mount %s, got %s", dbRootCertSecretName, secretName)
	}
}

// TestCrdbInitJobReportsAGenuineFailure covers the other half of the
// idempotency check: a failure that is not "already initialized" must still
// surface as a failure, or a cluster that never came up would be installed
// against as if it had.
func TestCrdbInitJobReportsAGenuineFailure(t *testing.T) {
	namespace := "threeport-control-plane"
	kubeClient := testKubeClient()

	// mark the created job failed past the backoff limit, same shape as the
	// existing database-drop failure test
	kubeClient.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job, ok := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if !ok {
			return false, nil, nil
		}
		if err := unstructured.SetNestedField(
			job.Object, int64(crdbInitJobBackoffLimit+1), "status", "failed",
		); err != nil {
			return false, nil, err
		}

		return false, nil, nil
	})
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace, Tier: ControlPlaneTierProd}}

	err := cpi.runCrdbInitJob(kubeClient, namespace)
	if err == nil {
		t.Fatal("expected a failed init job to be reported, got nil error")
	}
	if !strings.Contains(err.Error(), namespace) {
		t.Errorf("expected the error to name the namespace to inspect, got: %v", err)
	}
}

// TestCrdbAlreadyInitializedMarkerMatchesCockroachOwnWording pins the exact
// text, not just that the script and the constant agree with each other -
// which they would even if the constant itself drifted from what CockroachDB
// actually prints, since the script is built from the same constant.
//
// Confirmed by running `cockroach init` twice against a live container: the
// first succeeds ("Cluster successfully initialized"); the second, against
// the same cluster, exits 1 and prints exactly "ERROR: cluster has already
// been initialized". If this marker stops matching that wording, every retry
// against an already-initialized cluster fails for real instead of being
// recognized as done.
func TestCrdbAlreadyInitializedMarkerMatchesCockroachOwnWording(t *testing.T) {
	if crdbAlreadyInitializedMarker != "already been initialized" {
		t.Errorf(
			"expected the exact text a live cockroach init prints on a second run, got %q",
			crdbAlreadyInitializedMarker,
		)
	}
}

// crdbInitJobScript returns the script the init job's single container runs,
// the counterpart of dropDatabaseJobStatement for the drop job.
func crdbInitJobScript(t *testing.T, job *unstructured.Unstructured) string {
	t.Helper()

	containers, _, err := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	if err != nil || len(containers) != 1 {
		t.Fatalf("expected one container on the init job, got %v (err: %v)", containers, err)
	}
	command, _, _ := unstructured.NestedStringSlice(containers[0].(map[string]interface{}), "command")
	if len(command) != 3 {
		t.Fatalf("expected a 3-element /bin/sh -c <script> command, got %v", command)
	}

	return command[2]
}
