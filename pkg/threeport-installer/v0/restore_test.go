package v0

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
		phases, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/threeport_api?AUTH=implicit", "gs://b/threeport_api?AUTH=implicit", backup)
		require.NoError(t, err, backup)
		assert.Contains(t, strings.Join(phases.Prepare, "\n"), "FROM LATEST IN", backup)
	}

	phases, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/threeport_api?AUTH=implicit", "gs://b/threeport_api?AUTH=implicit", "/2026/10/20-050726.51")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(phases.Prepare, "\n"), "FROM '/2026/10/20-050726.51' IN")

	for _, backup := range []string{
		"2026/10/20-050726.51",
		"/2026/10/20",
		"/2026/10/20-050726.51'; DROP DATABASE system; --",
		"../other",
	} {
		_, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/x", "gs://b/x", backup)
		assert.ErrorIs(t, err, ErrInvalidBackup, backup)
	}
}

// TestOnlyTheSwapTouchesTheLiveDatabase covers what makes a restore that
// fails before the swap harmless: preparing restores alongside the live
// database and touches nothing of it, which is dropped only in the swap.
func TestOnlyTheSwapTouchesTheLiveDatabase(t *testing.T) {
	phases, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://b/x", "gs://b/x", LatestBackup)
	require.NoError(t, err)

	for _, statement := range append(append([]string{}, phases.Prepare...), phases.Backup...) {
		assert.NotContains(t, statement, "DROP DATABASE IF EXISTS threeport_api ", statement)
		assert.NotContains(t, statement, "RENAME", statement)
	}
	assert.Contains(t, phases.Swap, "DROP DATABASE IF EXISTS threeport_api CASCADE")
	assert.True(t, strings.HasPrefix(phases.Prepare[len(phases.Prepare)-1], "RESTORE "))
}

// TestTheFreshBackupGoesWhereTheScheduleWrites covers a restore from a
// bucket other than the one the schedule writes to, such as a copy of the
// backups: the backup is read from the copy, and the full backup the restore
// ends with goes to the schedule's bucket.  Without a schedule, no backup is
// taken.
func TestTheFreshBackupGoesWhereTheScheduleWrites(t *testing.T) {
	phases, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://copy/threeport_api", "gs://scheduled/threeport_api", LatestBackup)
	require.NoError(t, err)
	assert.Contains(t, phases.Prepare[1], "IN 'gs://copy/threeport_api'")
	assert.Equal(t, []string{"BACKUP DATABASE threeport_api INTO 'gs://scheduled/threeport_api'"}, phases.Backup)

	withoutSchedule, err := DatabaseRestoreStatements(DatabaseName, "threeport", "gs://copy/threeport_api", "", LatestBackup)
	require.NoError(t, err)
	assert.Empty(t, withoutSchedule.Backup)
}

// jobOutcome is how a fake restore Job ends.
type jobOutcome int

const (
	jobSucceeds jobOutcome = iota
	jobFails
	jobHangs
	// marked failed, as past its deadline, while its pod is still running
	jobFailsStillRunning
)

// restoreCluster is a fake cluster a restore runs against: it holds the
// control plane's Deployments, scales and annotates them as patched, and ends
// each phase's Job as told.
type restoreCluster struct {
	*dynamicfake.FakeDynamicClient

	mu          sync.Mutex
	replicas    map[string]int64
	annotations map[string]map[string]string
	scaled      [][2]interface{}
	outcomes    map[string]jobOutcome
	jobs        []string
	commands    map[string][]string
	backoffs    map[string]int64
	jobDeleteOK bool
	// how many more pod listings report a pod still there
	podListsLeft int
	podLists     int
}

func newRestoreCluster(t *testing.T, outcomes map[string]jobOutcome, replicas map[string]int64) *restoreCluster {
	t.Helper()

	cluster := &restoreCluster{
		FakeDynamicClient: testKubeClient(),
		replicas:          map[string]int64{},
		annotations:       map[string]map[string]string{},
		outcomes:          outcomes,
		commands:          map[string][]string{},
		backoffs:          map[string]int64{},
		jobDeleteOK:       true,
	}
	for name, count := range replicas {
		cluster.replicas[name] = count
		cluster.annotations[name] = map[string]string{}
	}

	// apply a merge patch of replicas and annotations
	cluster.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patch := action.(k8stesting.PatchAction)
		var body struct {
			Metadata struct {
				Annotations map[string]*string `json:"annotations"`
			} `json:"metadata"`
			Spec struct {
				Replicas *int64 `json:"replicas"`
			} `json:"spec"`
		}
		require.NoError(t, json.Unmarshal(patch.GetPatch(), &body))

		cluster.mu.Lock()
		defer cluster.mu.Unlock()
		for key, value := range body.Metadata.Annotations {
			if value == nil {
				delete(cluster.annotations[patch.GetName()], key)
			} else {
				cluster.annotations[patch.GetName()][key] = *value
			}
		}
		if body.Spec.Replicas != nil {
			cluster.replicas[patch.GetName()] = *body.Spec.Replicas
			cluster.scaled = append(cluster.scaled, [2]interface{}{patch.GetName(), *body.Spec.Replicas})
		}

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

	// report a pod still there for as many listings as told
	cluster.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		cluster.mu.Lock()
		defer cluster.mu.Unlock()
		cluster.podLists++
		list := &unstructured.UnstructuredList{Object: map[string]interface{}{"apiVersion": "v1", "kind": "PodList"}}
		if cluster.podListsLeft > 0 {
			cluster.podListsLeft--
			// the pod carries the labels the listing selects by, as the
			// fake client filters what a listing returns by them
			labels := map[string]interface{}{}
			for _, term := range strings.Split(action.(k8stesting.ListAction).GetListRestrictions().Labels.String(), ",") {
				if key, value, ok := strings.Cut(term, "="); ok {
					labels[key] = value
				}
			}
			pod := unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "threeport-api-server-7dd4f-x8ts5",
					"namespace": ControlPlaneNamespace,
					"labels":    labels,
				},
			}}
			list.Items = append(list.Items, pod)
		}
		return true, list, nil
	})

	// record each phase's job and end it as told
	cluster.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		phase := strings.TrimPrefix(job.GetName(), restoreJobNamePrefix)

		cluster.mu.Lock()
		cluster.jobs = append(cluster.jobs, phase)
		containers, _, _ := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
		var command []string
		for _, part := range containers[0].(map[string]interface{})["command"].([]interface{}) {
			command = append(command, part.(string))
		}
		cluster.commands[phase] = command
		cluster.backoffs[phase], _, _ = unstructured.NestedInt64(job.Object, "spec", "backoffLimit")
		outcome := cluster.outcomes[phase]
		cluster.mu.Unlock()

		switch outcome {
		case jobSucceeds:
			require.NoError(t, unstructured.SetNestedField(job.Object, int64(1), "status", "succeeded"))
		case jobFails:
			require.NoError(t, unstructured.SetNestedSlice(job.Object, []interface{}{
				map[string]interface{}{"type": "Failed", "status": "True", "reason": "BackoffLimitExceeded"},
			}, "status", "conditions"))
		case jobHangs:
			require.NoError(t, unstructured.SetNestedField(job.Object, int64(1), "status", "active"))
		case jobFailsStillRunning:
			require.NoError(t, unstructured.SetNestedSlice(job.Object, []interface{}{
				map[string]interface{}{"type": "Failed", "status": "True", "reason": "DeadlineExceeded"},
			}, "status", "conditions"))
			require.NoError(t, unstructured.SetNestedField(job.Object, int64(1), "status", "terminating"))
		}
		return false, nil, nil
	})
	cluster.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !cluster.jobDeleteOK {
			return true, nil, errors.New("the API server is unavailable")
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
	_ = unstructured.SetNestedStringMap(deployment.Object, map[string]string{"app.kubernetes.io/name": name}, "spec", "selector", "matchLabels")
	if len(c.annotations[name]) > 0 {
		deployment.SetAnnotations(c.annotations[name])
	}

	return deployment
}

// executed returns the statements a phase's job ran.
func (c *restoreCluster) executed(phase string) []string {
	var statements []string
	command := c.commands[phase]
	for i, part := range command {
		if part == "--execute" {
			statements = append(statements, command[i+1])
		}
	}

	return statements
}

// jobExists reports whether a phase's job is still in the cluster.
func (c *restoreCluster) jobExists(phase string) bool {
	_, err := c.Resource(jobGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), restoreJobNamePrefix+phase, metav1.GetOptions{})
	return err == nil
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

const testBucket = "threeport-prod-backups-acme-prod"

// restore runs a restore of the latest backup against cluster.
func restore(cluster *restoreCluster) error {
	return restoreInstaller().RestoreDatabase(cluster, testBucket, testBucket, LatestBackup)
}

// TestRestoreScalesTheControlPlaneDownAndBack covers a restore that
// succeeds: nothing writes to the database while it is replaced, each phase
// runs its own statements, one at a time since a RESTORE cannot share a
// transaction, and every Deployment comes back at the replica count it had,
// with nothing left recorded on it.
func TestRestoreScalesTheControlPlaneDownAndBack(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)

	require.NoError(t, restore(cluster))

	for i, scale := range cluster.scaled {
		if i < len(productionReplicas) {
			assert.Equal(t, int64(0), scale[1], "deployment %v was not scaled down first", scale[0])
		}
	}
	assert.Equal(t, productionReplicas, cluster.replicas)
	for name, annotations := range cluster.annotations {
		assert.NotContains(t, annotations, replicasBeforeRestoreAnnotation, name)
	}

	phases, err := DatabaseRestoreStatements(DatabaseName, "threeport", DatabaseBackupURI(testBucket), DatabaseBackupURI(testBucket), LatestBackup)
	require.NoError(t, err)
	assert.Equal(t, []string{"prepare", "swap", "backup"}, cluster.jobs)
	assert.Equal(t, phases.Prepare, cluster.executed("prepare"))
	assert.Equal(t, phases.Swap, cluster.executed("swap"))
	assert.Equal(t, phases.Backup, cluster.executed("backup"))
	for _, phase := range cluster.jobs {
		assert.False(t, cluster.jobExists(phase), "the finished %s job was left behind", phase)
	}
}

// TestAFailedPrepareBringsTheControlPlaneBack covers a restore that fails
// before the swap: the live database was not touched, so the control plane
// comes back on it, nothing else runs, and the failed job is left for its
// logs.
func TestAFailedPrepareBringsTheControlPlaneBack(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{"prepare": jobFails}, productionReplicas)

	err := restore(cluster)

	assert.ErrorContains(t, err, "unchanged")
	assert.Equal(t, productionReplicas, cluster.replicas)
	assert.Equal(t, []string{"prepare"}, cluster.jobs)
	assert.True(t, cluster.jobExists("prepare"), "the failed job was removed with its logs")
}

// swapRecorded reports whether the cluster records an unfinished swap.
func (c *restoreCluster) swapRecorded() bool {
	_, err := c.Resource(configMapResource).Namespace(ControlPlaneNamespace).Get(context.Background(), swapStartedConfigName, metav1.GetOptions{})
	return err == nil
}

// TestTheSwapIsNeverRetriedOnItsOwn covers the phase that cannot be run
// again as it is: run after it renamed the restored database into place, it
// would drop that database.  Its job gets no retries, while the phases that
// can be run again do.
func TestTheSwapIsNeverRetriedOnItsOwn(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)

	require.NoError(t, restore(cluster))
	assert.Equal(t, int64(0), cluster.backoffs["swap"])
	assert.Equal(t, int64(restoreDatabaseJobBackoffLimit), cluster.backoffs["prepare"])
	assert.Equal(t, int64(restoreDatabaseJobBackoffLimit), cluster.backoffs["backup"])
	assert.False(t, cluster.swapRecorded(), "a completed swap is still recorded as unfinished")
}

// TestAFailedSwapLeavesTheControlPlaneDown covers a restore that fails while
// replacing the database: it may be part way between the two, so nothing
// writes to it, and the error says to run the restore again.
func TestAFailedSwapLeavesTheControlPlaneDown(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{"swap": jobFails}, productionReplicas)

	err := restore(cluster)

	assert.ErrorContains(t, err, "running the restore again")
	assert.True(t, cluster.swapRecorded(), "the unfinished swap was not recorded")
	for name, count := range cluster.replicas {
		assert.Zero(t, count, "%s was brought back on a database that may be part way", name)
		assert.Equal(t, strconv.FormatInt(productionReplicas[name], 10), cluster.annotations[name][replicasBeforeRestoreAnnotation], name)
	}
}

// TestRunningTheRestoreAgainFinishesIt covers the way out of a failed swap:
// the restore run again, with the control plane already down, brings every
// Deployment back to the count it had before the first attempt rather than
// to zero.
func TestRunningTheRestoreAgainFinishesIt(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{"swap": jobFails}, productionReplicas)
	require.Error(t, restore(cluster))

	cluster.outcomes = map[string]jobOutcome{}
	require.NoError(t, restore(cluster))

	assert.Equal(t, productionReplicas, cluster.replicas)
	assert.False(t, cluster.swapRecorded(), "the finished swap is still recorded as unfinished")
	for name, annotations := range cluster.annotations {
		assert.NotContains(t, annotations, replicasBeforeRestoreAnnotation, name)
	}
}

// TestAFailedPrepareAfterAFailedSwapKeepsTheControlPlaneDown covers the
// restore run again after a swap failed, failing itself before its own swap:
// the earlier swap may have left the database part way, so the control plane
// stays down rather than coming back on it.
func TestAFailedPrepareAfterAFailedSwapKeepsTheControlPlaneDown(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{"swap": jobFails}, productionReplicas)
	require.Error(t, restore(cluster))

	cluster.outcomes = map[string]jobOutcome{"prepare": jobFails}
	err := restore(cluster)

	assert.ErrorContains(t, err, "an earlier restore stopped while replacing it")
	for name, count := range cluster.replicas {
		assert.Zero(t, count, "%s was brought back on a database an earlier swap may have left part way", name)
	}
	assert.True(t, cluster.swapRecorded())
}

// TestAFailedBackupStillBringsTheControlPlaneBack covers a restore whose last
// phase fails: the database is restored, so the control plane comes back,
// and the error says the scheduled incremental backups will fail.
func TestAFailedBackupStillBringsTheControlPlaneBack(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{"backup": jobFails}, productionReplicas)

	err := restore(cluster)

	assert.ErrorContains(t, err, "the database was restored")
	assert.ErrorContains(t, err, "incremental backups fail")
	assert.Equal(t, productionReplicas, cluster.replicas)
}

// TestAJobThatDoesNotFinishIsStoppedFirst covers a phase whose job neither
// succeeds nor fails in time: it is deleted, and only once it is gone does
// the control plane come back on the untouched database.
func TestAJobThatDoesNotFinishIsStoppedFirst(t *testing.T) {
	shortenRestoreWait(t)
	cluster := newRestoreCluster(t, map[string]jobOutcome{"prepare": jobHangs}, productionReplicas)

	err := restore(cluster)

	assert.ErrorContains(t, err, "was stopped")
	assert.False(t, cluster.jobExists("prepare"), "the job that did not finish is still there")
	assert.Equal(t, productionReplicas, cluster.replicas)
}

// TestAFailedJobWithAPodStillRunningIsStopped covers a job marked failed,
// as one past its deadline is, whose pod is still being terminated: it is not
// done until no pod runs, so it is stopped like one that did not finish.
func TestAFailedJobWithAPodStillRunningIsStopped(t *testing.T) {
	shortenRestoreWait(t)
	cluster := newRestoreCluster(t, map[string]jobOutcome{"prepare": jobFailsStillRunning}, productionReplicas)

	err := restore(cluster)

	assert.ErrorContains(t, err, "was stopped")
	assert.False(t, cluster.jobExists("prepare"), "the job with a pod still running is still there")
}

// TestAJobThatCannotBeStoppedKeepsTheControlPlaneDown covers a job that did
// not finish and cannot be deleted: it may still be writing, so nothing else
// writes to the database.
func TestAJobThatCannotBeStoppedKeepsTheControlPlaneDown(t *testing.T) {
	shortenRestoreWait(t)
	cluster := newRestoreCluster(t, map[string]jobOutcome{"prepare": jobHangs}, productionReplicas)
	cluster.PrependReactor("create", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		cluster.jobDeleteOK = false
		return false, nil, nil
	})

	err := restore(cluster)

	assert.ErrorIs(t, err, errRestoreJobNotStopped)
	for name, count := range cluster.replicas {
		assert.Zero(t, count, "%s was brought back while the job may still be running", name)
	}
}

// TestAFailedScaleDownBringsTheControlPlaneBack covers a scale-down that
// fails part way, after some Deployments are already at zero: the restore
// does not start, and every Deployment is scaled back to its count.
func TestAFailedScaleDownBringsTheControlPlaneBack(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)

	// the first patch to zero succeeds, the next one fails
	zeroPatches := 0
	cluster.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !strings.Contains(string(action.(k8stesting.PatchAction).GetPatch()), `"replicas":0`) {
			return false, nil, nil
		}
		zeroPatches++
		if zeroPatches == 2 {
			return true, nil, errors.New("the API server is unavailable")
		}
		return false, nil, nil
	})

	err := restore(cluster)
	assert.ErrorContains(t, err, "scale control plane down")
	assert.Equal(t, productionReplicas, cluster.replicas)
	assert.Empty(t, cluster.jobs, "the restore ran after the scale-down failed")
}

// TestAnInvalidRestoreChangesNothing covers a restore refused before it
// starts: an invalid backup or bucket leaves the control plane running.
func TestAnInvalidRestoreChangesNothing(t *testing.T) {
	for _, args := range [][2]string{
		{testBucket, "not-a-backup"},
		{"Not A Bucket", LatestBackup},
	} {
		cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
		assert.Error(t, restoreInstaller().RestoreDatabase(cluster, args[0], args[0], args[1]), args)
		assert.Empty(t, cluster.scaled, args)
		assert.Empty(t, cluster.jobs, args)
	}
}

// shortenRestoreWait makes a restore stop waiting for a job at once.
func shortenRestoreWait(t *testing.T) {
	wait, interval := restoreJobWait, restoreJobPollInterval
	restoreJobWait, restoreJobPollInterval = time.Millisecond, time.Millisecond
	t.Cleanup(func() { restoreJobWait, restoreJobPollInterval = wait, interval })
}

// leftoverJob returns a restore job of an earlier attempt, still running.
func leftoverJob(phase string) *unstructured.Unstructured {
	job := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      restoreJobNamePrefix + phase,
			"namespace": ControlPlaneNamespace,
		},
	}}
	_ = unstructured.SetNestedField(job.Object, int64(1), "status", "active")

	return job
}

// TestAnEarlierAttemptsJobsAreStoppedFirst covers a restore started while a
// job of an earlier one is still running, as when its CLI exited during the
// swap: that job is stopped before this restore changes anything, so the two
// never change the databases together.
func TestAnEarlierAttemptsJobsAreStoppedFirst(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
	require.NoError(t, cluster.Tracker().Add(leftoverJob("swap")))
	cluster.ClearActions()

	require.NoError(t, restore(cluster))

	swapDeleted, firstChange := -1, -1
	for i, action := range cluster.Actions() {
		if action.Matches("delete", "jobs") && action.(k8stesting.DeleteAction).GetName() == restoreJobNamePrefix+"swap" && swapDeleted == -1 {
			swapDeleted = i
		}
		if (action.Matches("patch", "deployments") || action.Matches("create", "jobs")) && firstChange == -1 {
			firstChange = i
		}
	}
	require.NotEqual(t, -1, swapDeleted, "the earlier swap job was not stopped")
	assert.Less(t, swapDeleted, firstChange, "the restore changed something before stopping the earlier swap job")
}

// TestAnEarlierJobThatCannotBeStoppedChangesNothing covers an earlier job
// that cannot be deleted: it may still be changing the databases, so the
// restore stops before scaling anything or starting a job of its own.
func TestAnEarlierJobThatCannotBeStoppedChangesNothing(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
	cluster.jobDeleteOK = false

	err := restore(cluster)

	assert.ErrorIs(t, err, errRestoreJobNotStopped)
	assert.Empty(t, cluster.scaled)
	assert.Empty(t, cluster.jobs)
}

// TestAFailedScaleDownAfterAFailedSwapKeepsTheControlPlaneDown covers a
// restore run again after a failed swap whose scale-down fails: the earlier
// swap may have left the database part way, so nothing is scaled back up.
func TestAFailedScaleDownAfterAFailedSwapKeepsTheControlPlaneDown(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{"swap": jobFails}, productionReplicas)
	require.Error(t, restore(cluster))

	cluster.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if strings.Contains(string(action.(k8stesting.PatchAction).GetPatch()), `"replicas":0`) {
			return true, nil, errors.New("the API server is unavailable")
		}
		return false, nil, nil
	})
	cluster.outcomes = map[string]jobOutcome{}
	err := restore(cluster)

	assert.ErrorContains(t, err, "an earlier restore stopped while replacing the database")
	for name, count := range cluster.replicas {
		assert.Zero(t, count, "%s was brought back on a database an earlier swap may have left part way", name)
	}
}

// lockHeld reports whether the restore lock is held.
func (c *restoreCluster) lockHeld() bool {
	_, err := c.Resource(configMapResource).Namespace(ControlPlaneNamespace).Get(context.Background(), restoreLockName, metav1.GetOptions{})
	return err == nil
}

// shortenWritersWait makes a restore stop waiting for pods at once.
func shortenWritersWait(t *testing.T) {
	wait, interval := restoreWritersWait, restoreWritersPollInterval
	restoreWritersWait, restoreWritersPollInterval = time.Millisecond, time.Millisecond
	t.Cleanup(func() { restoreWritersWait, restoreWritersPollInterval = wait, interval })
}

// TestTheRestoreWaitsForEveryPodToBeGone covers writers a Deployment's
// readiness does not show: an API server pod running its database migrator,
// or one shutting down, is not ready but can still write.  The restore waits
// for every pod of the Deployments it scaled down to be gone before it
// starts.
func TestTheRestoreWaitsForEveryPodToBeGone(t *testing.T) {
	shortenWritersWait(t)
	restoreWritersWait = time.Second
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
	cluster.podListsLeft = 5

	require.NoError(t, restore(cluster))

	assert.Zero(t, cluster.podListsLeft, "the restore started while a pod was still there")
	assert.Greater(t, cluster.podLists, 5)
}

// TestPodsThatStayBringTheControlPlaneBack covers pods that are not gone in
// time: the database was not touched, so the restore stops there and brings
// the control plane back.
func TestPodsThatStayBringTheControlPlaneBack(t *testing.T) {
	shortenWritersWait(t)
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
	cluster.podListsLeft = 1000

	err := restore(cluster)

	assert.ErrorContains(t, err, "pods still running after the scale-down")
	assert.Empty(t, cluster.jobs)
	assert.Equal(t, productionReplicas, cluster.replicas)
}

// TestOneRestoreAtATime covers a restore started while another holds the
// lock: it changes nothing, names the other, and leaves its lock alone.
func TestOneRestoreAtATime(t *testing.T) {
	cluster := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
	release, err := acquireRestoreLock(cluster, ControlPlaneNamespace)
	require.NoError(t, err)

	err = restore(cluster)

	assert.ErrorContains(t, err, "another restore of this control plane is running")
	assert.Empty(t, cluster.scaled)
	assert.Empty(t, cluster.jobs)
	assert.True(t, cluster.lockHeld(), "the other restore's lock was released")

	require.NoError(t, release())
	require.NoError(t, restore(cluster))
}

// TestTheLockIsReleasedEitherWay covers the lock after a restore that
// succeeded and one that failed: the next restore can always start.
func TestTheLockIsReleasedEitherWay(t *testing.T) {
	succeeded := newRestoreCluster(t, map[string]jobOutcome{}, productionReplicas)
	require.NoError(t, restore(succeeded))
	assert.False(t, succeeded.lockHeld())

	failed := newRestoreCluster(t, map[string]jobOutcome{"swap": jobFails}, productionReplicas)
	require.Error(t, restore(failed))
	assert.False(t, failed.lockHeld())
}
