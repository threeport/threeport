package v0

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestRestoreStatementsNameOnlyABackup covers the backup a restore takes as
// an argument and puts in SQL: the latest one, or a path in the form
// CockroachDB gives backups.  Anything else is refused rather than quoted
// into a statement.
func TestRestoreStatementsNameOnlyABackup(t *testing.T) {
	for _, backup := range []string{"", "LATEST", "latest"} {
		statements, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/threeport_api?AUTH=implicit", backup)
		require.NoError(t, err, backup)
		assert.Contains(t, strings.Join(statements, "\n"), "FROM LATEST IN", backup)
	}

	statements, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/threeport_api?AUTH=implicit", "/2026/10/20-050726.51")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(statements, "\n"), "FROM '/2026/10/20-050726.51' IN")

	for _, backup := range []string{
		"2026/10/20-050726.51",
		"/2026/10/20",
		"/2026/10/20-050726.51'; DROP DATABASE system; --",
		"../other",
	} {
		_, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/x", backup)
		assert.ErrorIs(t, err, ErrInvalidBackup, backup)
	}
}

// TestRestoreDropsTheDatabaseOnlyOnceTheBackupIsRestored covers the order
// that makes a failed restore harmless: nothing is dropped before the
// backup is restored alongside the database.
func TestRestoreDropsTheDatabaseOnlyOnceTheBackupIsRestored(t *testing.T) {
	statements, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/x", LatestBackup)
	require.NoError(t, err)

	restoreAt, dropAt := -1, -1
	for i, statement := range statements {
		if strings.HasPrefix(statement, "RESTORE ") {
			restoreAt = i
		}
		if statement == "DROP DATABASE IF EXISTS threeport_api CASCADE" {
			dropAt = i
		}
	}
	require.NotEqual(t, -1, restoreAt)
	require.NotEqual(t, -1, dropAt)
	assert.Less(t, restoreAt, dropAt)
}

// restoreCluster is a fake cluster a restore runs against: it holds the
// control plane's Deployments, scales them as patched, and finishes the
// restore Job with the given outcome.
type restoreCluster struct {
	*dynamicfake.FakeDynamicClient

	mu       sync.Mutex
	replicas map[string]int64
	scaled   [][2]interface{}
	job      *unstructured.Unstructured
	jobsLeft int
}

func newRestoreCluster(t *testing.T, jobSucceeds bool, replicas map[string]int64) *restoreCluster {
	t.Helper()

	objects := []runtime.Object{}
	for name, count := range replicas {
		deployment := testManagedDeployment(name, ControlPlaneNamespace)
		require.NoError(t, unstructured.SetNestedField(deployment.Object, count, "spec", "replicas"))
		objects = append(objects, deployment)
	}
	cluster := &restoreCluster{
		FakeDynamicClient: testKubeClient(objects...),
		replicas:          map[string]int64{},
	}
	for name, count := range replicas {
		cluster.replicas[name] = count
	}

	// scale a deployment as patched and report its replicas ready at once
	cluster.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch := action.(k8stesting.PatchAction)
		var body struct {
			Spec struct {
				Replicas int64 `json:"replicas"`
			} `json:"spec"`
		}
		require.NoError(t, json.Unmarshal(patch.GetPatch(), &body))

		cluster.mu.Lock()
		defer cluster.mu.Unlock()
		cluster.replicas[patch.GetName()] = body.Spec.Replicas
		cluster.scaled = append(cluster.scaled, [2]interface{}{patch.GetName(), body.Spec.Replicas})

		return true, cluster.deployment(patch.GetName()), nil
	})
	cluster.PrependReactor("get", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		cluster.mu.Lock()
		defer cluster.mu.Unlock()
		return true, cluster.deployment(action.(k8stesting.GetAction).GetName()), nil
	})
	cluster.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		cluster.mu.Lock()
		defer cluster.mu.Unlock()
		list := &unstructured.UnstructuredList{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "DeploymentList"}}
		for name := range cluster.replicas {
			list.Items = append(list.Items, *cluster.deployment(name))
		}
		return true, list, nil
	})

	// record the restore job and finish it
	cluster.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		cluster.mu.Lock()
		cluster.job = job.DeepCopy()
		cluster.mu.Unlock()
		if jobSucceeds {
			require.NoError(t, unstructured.SetNestedField(job.Object, int64(1), "status", "succeeded"))
		} else {
			require.NoError(t, unstructured.SetNestedSlice(job.Object, []interface{}{
				map[string]interface{}{"type": "Failed", "status": "True", "reason": "BackoffLimitExceeded"},
			}, "status", "conditions"))
		}
		return false, nil, nil
	})

	return cluster
}

// deployment returns a Deployment as the fake cluster holds it, with every
// replica ready.
func (c *restoreCluster) deployment(name string) *unstructured.Unstructured {
	deployment := testManagedDeployment(name, ControlPlaneNamespace)
	_ = unstructured.SetNestedField(deployment.Object, c.replicas[name], "spec", "replicas")
	_ = unstructured.SetNestedField(deployment.Object, c.replicas[name], "status", "readyReplicas")

	return deployment
}

// jobCommand returns the restore Job's command.
func (c *restoreCluster) jobCommand(t *testing.T) []string {
	t.Helper()

	require.NotNil(t, c.job, "no restore job was created")
	containers, _, err := unstructured.NestedSlice(c.job.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	command := []string{}
	for _, part := range containers[0].(map[string]interface{})["command"].([]interface{}) {
		command = append(command, part.(string))
	}

	return command
}

// restoreInstaller returns an installer for the default control plane.
func restoreInstaller() *ControlPlaneInstaller {
	return &ControlPlaneInstaller{Opts: Options{
		Namespace:   ControlPlaneNamespace,
		RestApiInfo: ThreeportRestApi,
	}}
}

var productionReplicas = map[string]int64{
	ThreeportRestApi.ServiceResourceName: 2,
	"threeport-secret-controller":        2,
	"threeport-terraform-controller":     1,
}

// TestRestoreScalesTheControlPlaneDownAndBack covers a restore that
// succeeds: nothing writes to the database while it is replaced, the
// restore runs each statement on its own, and every Deployment comes back
// at the replica count it had.
func TestRestoreScalesTheControlPlaneDownAndBack(t *testing.T) {
	cluster := newRestoreCluster(t, true, productionReplicas)

	require.NoError(t, restoreInstaller().RestoreDatabase(cluster, "threeport-prod-backups-acme-prod", LatestBackup))

	// scaled to zero before anything else, then back
	for i, scale := range cluster.scaled {
		if i < len(productionReplicas) {
			assert.Equal(t, int64(0), scale[1], "deployment %v was not scaled down first", scale[0])
		}
	}
	assert.Equal(t, productionReplicas, cluster.replicas)

	// every statement is its own --execute, since RESTORE cannot share a transaction
	statements, err := DatabaseRestoreStatements(DatabaseName, "threeport", DatabaseBackupURI("threeport-prod-backups-acme-prod"), LatestBackup)
	require.NoError(t, err)
	executed := []string{}
	command := cluster.jobCommand(t)
	for i, part := range command {
		if part == "--execute" {
			executed = append(executed, command[i+1])
		}
	}
	assert.Equal(t, statements, executed)

	// the finished job is removed
	_, err = cluster.Resource(jobGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), restoreDatabaseJobName, metav1.GetOptions{})
	assert.Error(t, err)
}

// TestAFailedRestoreBringsTheControlPlaneBack covers a restore whose Job
// fails: the control plane comes back up on the database it had, the
// failure is reported, and the Job is left for its logs.
func TestAFailedRestoreBringsTheControlPlaneBack(t *testing.T) {
	cluster := newRestoreCluster(t, false, productionReplicas)

	err := restoreInstaller().RestoreDatabase(cluster, "threeport-prod-backups-acme-prod", LatestBackup)
	assert.ErrorContains(t, err, "restore job")
	assert.Equal(t, productionReplicas, cluster.replicas)

	_, err = cluster.Resource(jobGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), restoreDatabaseJobName, metav1.GetOptions{})
	assert.NoError(t, err, "the failed restore job was removed with its logs")
}

// TestAnInvalidRestoreChangesNothing covers a restore refused before it
// starts: an invalid backup or bucket leaves the control plane running.
func TestAnInvalidRestoreChangesNothing(t *testing.T) {
	for _, args := range [][2]string{
		{"threeport-prod-backups-acme-prod", "not-a-backup"},
		{"Not A Bucket", LatestBackup},
	} {
		cluster := newRestoreCluster(t, true, productionReplicas)
		assert.Error(t, restoreInstaller().RestoreDatabase(cluster, args[0], args[1]), args)
		assert.Empty(t, cluster.scaled, args)
		assert.Nil(t, cluster.job, args)
	}
}
