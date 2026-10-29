package v0

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// A restore replaces the control plane database with one of its backups, in
// three phases that each run as a Job.  The backup is restored under another
// name first and only swapped in once it is complete, so a restore that fails
// before the swap leaves the database as it was.
const (
	// LatestBackup restores the most recent backup in the bucket.
	LatestBackup = "LATEST"

	// restoreJobNamePrefix names the Job of each restore phase.
	restoreJobNamePrefix = "threeport-restore-database-"

	// restoreDatabaseJobBackoffLimit is how many failed pods end the prepare
	// and backup phases, whose statements can be run again as they are.  The
	// swap is never retried on its own: run again after it renamed the
	// restored database into place, it would drop that database.  A restore
	// run again starts from prepare, which restores the backup afresh, and
	// the swap is safe from there wherever the last one stopped.
	restoreDatabaseJobBackoffLimit = 2

	// swapStartedConfigName is the ConfigMap that records a swap was started
	// and has not completed, so the database may be part way between the
	// live and the restored one.
	swapStartedConfigName = "threeport-restore-swap-started"

	// restoreDatabaseTimeout is how long the Job of a phase may run.
	restoreDatabaseTimeout = 30 * time.Minute

	// restoringDatabaseSuffix names the database a backup is restored into
	// before it replaces the live one.
	restoringDatabaseSuffix = "_restore"

	// replicasBeforeRestoreAnnotation records on each control plane
	// Deployment the replica count it had before a restore scaled it down,
	// so a restore run again after one that left the control plane down
	// brings it back to that count rather than to zero.
	replicasBeforeRestoreAnnotation = "threeport.io/replicas-before-restore"
)

// restoreJobWait is how long a restore waits for the Job of a phase before
// stopping it, a little past the Job's own deadline, and how often it checks.
var (
	restoreJobWait         = restoreDatabaseTimeout + 2*time.Minute
	restoreJobPollInterval = 5 * time.Second
)

// backupPathPattern matches a backup's path in a bucket, as CockroachDB
// names it and `SHOW BACKUPS` lists it: /YYYY/MM/DD-HHMMSS.SS.
var backupPathPattern = regexp.MustCompile(`^/\d{4}/\d{2}/\d{2}-\d{6}\.\d{2}$`)

// bucketNamePattern matches a GCS bucket name.
var bucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$`)

// ErrInvalidBackup is returned for a backup that names neither the latest
// backup nor a backup path.
var ErrInvalidBackup = errors.New("invalid backup")

// RestorePhases are the statements of each phase of a restore.  Each phase
// runs its statements one at a time in one session, and can be run again
// after it failed part way.
type RestorePhases struct {
	// Prepare restores the backup under another name, after removing what an
	// earlier restore that failed may have left under it.  The live database
	// is not touched.
	Prepare []string

	// Swap drops the live database, renames the restored one into its place
	// and hands its objects back to the user that owns them: a restore makes
	// the restoring user their owner and leaves the database without its
	// grants, which would lock the API server out of its data.  The database
	// may be part way between the two while it runs.
	Swap []string

	// Backup takes a full backup of the restored database where the
	// scheduled backups go.  Its tables are new to the backups already there,
	// so the scheduled incremental backups would fail on top of them until
	// the next scheduled full one.  Empty when there is no schedule.
	Backup []string
}

// All returns every statement of a restore in order.
func (p RestorePhases) All() []string {
	all := append(append([]string{}, p.Prepare...), p.Swap...)
	return append(all, p.Backup...)
}

// DatabaseRestoreStatements returns the phases of replacing a database with
// one of its backups.  The backup is restored from sourceURI, which is the
// schedule's own location unless the backups were copied elsewhere, and the
// fresh full backup goes to scheduleURI, if there is a schedule.
func DatabaseRestoreStatements(databaseName, ownerName, sourceURI, scheduleURI, backup string) (RestorePhases, error) {
	from, err := backupSource(backup)
	if err != nil {
		return RestorePhases{}, err
	}
	restoring := databaseName + restoringDatabaseSuffix

	phases := RestorePhases{
		Prepare: []string{
			fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", restoring),
			fmt.Sprintf("RESTORE DATABASE %s FROM %s IN '%s' WITH new_db_name = '%s'", databaseName, from, sourceURI, restoring),
		},
		Swap: []string{
			fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", databaseName),
			fmt.Sprintf("ALTER DATABASE %s RENAME TO %s", restoring, databaseName),
			fmt.Sprintf("USE %s", databaseName),
			fmt.Sprintf("REASSIGN OWNED BY root TO %s", ownerName),
			fmt.Sprintf("ALTER DATABASE %s OWNER TO root", databaseName),
			"ALTER SCHEMA public OWNER TO root",
			fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", databaseName, ownerName),
		},
	}
	if scheduleURI != "" {
		phases.Backup = []string{fmt.Sprintf("BACKUP DATABASE %s INTO '%s'", databaseName, scheduleURI)}
	}

	return phases, nil
}

// backupSource returns how a RESTORE statement names a backup.
func backupSource(backup string) (string, error) {
	if backup == "" || strings.EqualFold(backup, LatestBackup) {
		return LatestBackup, nil
	}
	if !backupPathPattern.MatchString(backup) {
		return "", fmt.Errorf(
			"%w %q: use %s or a backup path such as /2026/10/20-050726.51, as listed in the bucket",
			ErrInvalidBackup, backup, LatestBackup,
		)
	}

	return fmt.Sprintf("'%s'", backup), nil
}

// RestoreDatabase replaces the control plane database with a backup from
// sourceBucket.  scheduleBucket is where the control plane's scheduled
// backups go, which the restore takes a fresh full backup into; empty when
// it has no schedule.
//
// The control plane is scaled down so nothing writes to the database while it
// is replaced.  It is scaled back up only when the database is known to be
// whole: after a restore that succeeded, or one that stopped before the swap
// and left the database as it was.  A restore that stopped during the swap
// leaves the control plane down, since the database may be part way between
// the two, and running the restore again finishes it and brings the control
// plane back.  A phase's Job is never left running when the control plane
// comes back: it has finished, or it is stopped and confirmed gone.
func (cpi *ControlPlaneInstaller) RestoreDatabase(
	kubeClient dynamic.Interface,
	sourceBucket string,
	scheduleBucket string,
	backup string,
) error {
	if !bucketNamePattern.MatchString(sourceBucket) {
		return fmt.Errorf("invalid backup bucket name %q", sourceBucket)
	}
	scheduleURI := ""
	if scheduleBucket != "" {
		if !bucketNamePattern.MatchString(scheduleBucket) {
			return fmt.Errorf("invalid backup bucket name %q", scheduleBucket)
		}
		scheduleURI = DatabaseBackupURI(scheduleBucket)
	}
	phases, err := DatabaseRestoreStatements(
		database.ThreeportDatabaseName,
		database.ThreeportDatabaseUser,
		DatabaseBackupURI(sourceBucket),
		scheduleURI,
		backup,
	)
	if err != nil {
		return err
	}
	namespace := cpi.Opts.Namespace

	// one restore at a time: two would stop each other's jobs and act on
	// each other's state
	release, err := acquireRestoreLock(kubeClient, namespace)
	if err != nil {
		return err
	}
	restoreErr := cpi.restoreDatabaseLocked(kubeClient, namespace, phases, scheduleURI)
	if err := release(); err != nil {
		return errors.Join(restoreErr, err)
	}

	return restoreErr
}

// restoreDatabaseLocked runs a restore holding the restore lock.
func (cpi *ControlPlaneInstaller) restoreDatabaseLocked(
	kubeClient dynamic.Interface,
	namespace string,
	phases RestorePhases,
	scheduleURI string,
) error {
	// an earlier restore that stopped during its swap may have left the
	// database part way, whatever happens to this one before its own swap
	swapStarted, err := swapStartedRecorded(kubeClient, namespace)
	if err != nil {
		return err
	}

	// a job of an earlier attempt, such as one whose CLI exited during the
	// swap, could still be changing the databases this one is about to, so
	// every one is stopped before anything changes
	for _, phase := range restorePhaseNames {
		if err := deleteJob(kubeClient, namespace, restoreJobNamePrefix+phase); err != nil {
			return fmt.Errorf(
				"%w: the %s job of an earlier restore could not be stopped, so nothing was changed: %w",
				errRestoreJobNotStopped, phase, err,
			)
		}
	}

	// record the replica counts to scale back to, unless an earlier restore
	// already did and left the control plane down
	replicas, err := cpi.recordReplicasBeforeRestore(kubeClient, namespace)
	if err != nil {
		return err
	}

	// stop everything that writes to the database, and wait until none of
	// its pods is left: one not ready can still be writing, such as an API
	// server pod running its database migrator or one being terminated.  A
	// scale-down that fails part way may already have stopped some of it,
	// so the control plane is brought back before the failure is returned.
	scaleDownErr := cpi.scaleDownDeployments(kubeClient, namespace)
	if scaleDownErr == nil {
		scaleDownErr = waitForPodsGone(kubeClient, namespace, replicas)
	}
	if err := scaleDownErr; err != nil {
		scaleDownErr := fmt.Errorf("failed to scale control plane down before the restore: %w", err)
		if swapStarted {
			return fmt.Errorf(
				"%w; an earlier restore stopped while replacing the database, so the control plane is left scaled down until a restore completes",
				scaleDownErr,
			)
		}
		if err := cpi.scaleDeployments(kubeClient, namespace, replicas); err != nil {
			return errors.Join(scaleDownErr, err)
		}
		return scaleDownErr
	}

	// prepare: the live database is untouched, so a failure brings the
	// control plane back on it once the Job is no longer running - unless an
	// earlier swap left the database part way
	if err := runRestorePhase(kubeClient, namespace, "prepare", phases.Prepare, restoreDatabaseJobBackoffLimit); err != nil {
		if swapStarted {
			return fmt.Errorf(
				"the restore failed before replacing the database, but an earlier restore stopped while replacing it, "+
					"so it may be part way; the control plane is left scaled down until a restore completes: %w",
				err,
			)
		}
		prepareErr := fmt.Errorf("the restore failed before replacing the database, which is unchanged: %w", err)
		if errors.Is(err, errRestoreJobNotStopped) {
			return fmt.Errorf("%w; the control plane is left scaled down until it is", prepareErr)
		}
		if err := cpi.scaleDeployments(kubeClient, namespace, replicas); err != nil {
			return errors.Join(prepareErr, err)
		}
		return prepareErr
	}

	// swap: record that it started, so a later restore knows the database
	// may be part way if this one stops, and keep the control plane down
	// when it fails
	if err := recordSwapStarted(kubeClient, namespace); err != nil {
		prepareErr := fmt.Errorf("failed to record that the restore is replacing the database, so it was not replaced: %w", err)
		if swapStarted {
			return prepareErr
		}
		if err := cpi.scaleDeployments(kubeClient, namespace, replicas); err != nil {
			return errors.Join(prepareErr, err)
		}
		return prepareErr
	}
	if err := runRestorePhase(kubeClient, namespace, "swap", phases.Swap, 0); err != nil {
		return fmt.Errorf(
			"the restore failed while replacing the database, which may be part way between the live and the restored one; "+
				"the control plane is left scaled down, and running the restore again with the same backup finishes it: %w",
			err,
		)
	}
	swapRecordErr := clearSwapStarted(kubeClient, namespace)

	// backup: the database is restored either way
	backupErr := swapRecordErr
	if len(phases.Backup) > 0 {
		if err := runRestorePhase(kubeClient, namespace, "backup", phases.Backup, restoreDatabaseJobBackoffLimit); err != nil {
			backupErr = errors.Join(swapRecordErr, fmt.Errorf(
				"the database was restored, but the full backup into %s failed, so scheduled incremental backups fail until the next scheduled full one: %w",
				scheduleURI, err,
			))
			if errors.Is(err, errRestoreJobNotStopped) {
				return fmt.Errorf("%w; the control plane is left scaled down until it is", backupErr)
			}
		}
	}

	if err := cpi.scaleDeployments(kubeClient, namespace, replicas); err != nil {
		return errors.Join(backupErr, err)
	}

	// wait for the API server to serve the restored database
	fmt.Printf("Info: waiting for rest-api deployment to become ready (timeout %s)\n", restApiDeploymentReadyTimeout)
	if err := cpi.waitForRestAPIReady(kubeClient, namespace, restApiDeploymentReadyTimeout); err != nil {
		return errors.Join(backupErr, fmt.Errorf("rest-api did not become ready after the restore: %w", err))
	}

	return backupErr
}

// recordReplicasBeforeRestore returns the replica count each control plane
// Deployment a restore scales down should come back to, and records it on the
// Deployment.  A count already recorded by an earlier restore that left the
// control plane down is kept, since the Deployment is at zero by then.
func (cpi *ControlPlaneInstaller) recordReplicasBeforeRestore(
	kubeClient dynamic.Interface,
	namespace string,
) (map[string]int64, error) {
	selector := fmt.Sprintf(
		"%s=%s,%s!=%s",
		LabelManagedBy, LabelManagedByValue,
		LabelPersistent, LabelPersistentValue,
	)
	deployments, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list control plane deployments: %w", err)
	}

	replicas := map[string]int64{}
	for _, deployment := range deployments.Items {
		if recorded, ok := deployment.GetAnnotations()[replicasBeforeRestoreAnnotation]; ok {
			count, err := strconv.ParseInt(recorded, 10, 64)
			if err != nil {
				return nil, fmt.Errorf(
					"deployment %s records an invalid replica count %q in %s: %w",
					deployment.GetName(), recorded, replicasBeforeRestoreAnnotation, err,
				)
			}
			replicas[deployment.GetName()] = count
			continue
		}

		count, found, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
		if !found {
			// an unset count is one replica
			count = 1
		}
		patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, replicasBeforeRestoreAnnotation, strconv.FormatInt(count, 10)))
		if _, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Patch(
			context.Background(), deployment.GetName(), "application/merge-patch+json", patch, metav1.PatchOptions{},
		); err != nil {
			return nil, fmt.Errorf("failed to record the replica count of deployment %s: %w", deployment.GetName(), err)
		}
		replicas[deployment.GetName()] = count
	}

	return replicas, nil
}

// scaleDeployments sets each Deployment to its replica count and removes the
// count recorded before the restore.
func (cpi *ControlPlaneInstaller) scaleDeployments(
	kubeClient dynamic.Interface,
	namespace string,
	replicas map[string]int64,
) error {
	fmt.Println("Info: scaling control plane deployments back up")
	var scaleErrors []error
	for name, count := range replicas {
		patch := []byte(fmt.Sprintf(
			`{"metadata":{"annotations":{%q:null}},"spec":{"replicas":%d}}`,
			replicasBeforeRestoreAnnotation, count,
		))
		if _, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Patch(
			context.Background(),
			name,
			"application/merge-patch+json",
			patch,
			metav1.PatchOptions{},
		); err != nil && !k8serrors.IsNotFound(err) {
			scaleErrors = append(scaleErrors, fmt.Errorf("failed to scale deployment %s to %d: %w", name, count, err))
		}
	}

	return errors.Join(scaleErrors...)
}

// restorePhaseNames are the phases of a restore, in the order they run.
var restorePhaseNames = []string{"prepare", "swap", "backup"}

// errRestoreJobNotStopped is returned when the Job of a phase may still be
// running: it neither finished in time nor could be confirmed gone.
var errRestoreJobNotStopped = errors.New("the restore job could not be confirmed stopped")

// runRestorePhase runs the statements of a phase in a one-shot Job with the
// database's own client, and returns once the Job's pods are no longer
// running.  A Job that does not finish in time is deleted, and the phase
// returns only once it is gone; one that cannot be confirmed gone returns
// errRestoreJobNotStopped.  A Job that failed is left in place so its pod
// logs can be read.
func runRestorePhase(kubeClient dynamic.Interface, namespace, phase string, statements []string, backoffLimit int64) error {
	jobName := restoreJobNamePrefix + phase

	// a leftover job is removed first, since its pod template cannot change
	if err := deleteJob(kubeClient, namespace, jobName); err != nil {
		return fmt.Errorf("%w: failed to remove the leftover %s job: %w", errRestoreJobNotStopped, phase, err)
	}

	// every statement runs on its own: a RESTORE cannot run in a transaction
	// with other statements
	command := []interface{}{
		"/cockroach/cockroach",
		"sql",
		fmt.Sprintf("--certs-dir=%s", dbRootCertsMountPath),
		fmt.Sprintf("--host=%s", database.ThreeportDatabaseHost),
		fmt.Sprintf("--port=%s", database.ThreeportDatabasePort),
	}
	for _, statement := range statements {
		fmt.Printf("Info: running %q against the database\n", statement)
		command = append(command, "--execute", statement)
	}

	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      jobName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"backoffLimit":          backoffLimit,
				"activeDeadlineSeconds": int64(restoreDatabaseTimeout.Seconds()),
				"template": map[string]interface{}{
					// the label the database network policy admits; a restore
					// runs while the API server is scaled to zero, so it has to
					// reach the database directly
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{LabelMaintenance: "true"},
					},
					"spec": map[string]interface{}{
						"restartPolicy": "Never",
						"containers": []interface{}{
							map[string]interface{}{
								"name":    "restore-database",
								"image":   fmt.Sprintf("cockroachdb/cockroach:%s", DatabaseImageTag),
								"command": command,
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      dbRootCertSecretName,
										"mountPath": dbRootCertsMountPath,
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": dbRootCertSecretName,
								"secret": map[string]interface{}{
									"secretName":  dbRootCertSecretName,
									"defaultMode": int64(dbRootCertsFileMode),
								},
							},
						},
					},
				},
			},
		},
	}
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("failed to create the %s job: %w", phase, err)
	}

	succeeded, failure, err := waitForRestoreJob(kubeClient, namespace, jobName)
	if err != nil {
		// stop a job that did not finish in time before anything relies on
		// it not running
		if deleteErr := deleteJob(kubeClient, namespace, jobName); deleteErr != nil {
			return fmt.Errorf("%w: the %s job %s did not finish (%w) and could not be deleted: %w",
				errRestoreJobNotStopped, phase, jobName, err, deleteErr)
		}
		return fmt.Errorf("the %s job %s did not finish and was stopped: %w", phase, jobName, err)
	}
	if !succeeded {
		return fmt.Errorf(
			"the %s job %s failed (%s): inspect its pod logs in namespace %s",
			phase, jobName, failure, namespace,
		)
	}

	return deleteJob(kubeClient, namespace, jobName)
}

// waitForRestoreJob polls a restore Job until it succeeded, or failed with
// none of its pods still running.  It returns an error when neither happens
// in time.
func waitForRestoreJob(kubeClient dynamic.Interface, namespace, jobName string) (bool, string, error) {
	var succeeded bool
	var failure string
	attempts := int(restoreJobWait/restoreJobPollInterval) + 1

	err := pollEvery(attempts, restoreJobPollInterval, func() error {
		job, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
			context.Background(), jobName, metav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to read the status of job %s: %w", jobName, err)
		}

		if count, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "succeeded"); count > 0 {
			succeeded = true
			return nil
		}

		// a failed job is done once none of its pods is still running or
		// being terminated
		conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
		for _, item := range conditions {
			condition, ok := item.(map[string]interface{})
			if !ok || condition["type"] != "Failed" || condition["status"] != "True" {
				continue
			}
			active, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "active")
			terminating, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "terminating")
			if active > 0 || terminating > 0 {
				return fmt.Errorf("job %s failed but still has pods running", jobName)
			}
			failure = fmt.Sprint(condition["reason"])
			return nil
		}

		return fmt.Errorf("job %s has not completed", jobName)
	})

	return succeeded, failure, err
}

// deleteJob removes a Job and its pods and waits until it is gone.
func deleteJob(kubeClient dynamic.Interface, namespace, name string) error {
	deletePolicy := metav1.DeletePropagationForeground
	if err := kubeClient.Resource(jobGVR).Namespace(namespace).Delete(
		context.Background(), name, metav1.DeleteOptions{PropagationPolicy: &deletePolicy},
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete job %s: %w", name, err)
	}

	return util.Retry(20, 3, func() error {
		_, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
			context.Background(), name, metav1.GetOptions{},
		)
		if k8serrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to check job %s: %w", name, err)
		}

		return fmt.Errorf("job %s still terminating", name)
	})
}

// pollEvery runs f until it succeeds or has been tried attempts times,
// waiting interval between tries, and returns its last error.
func pollEvery(attempts int, interval time.Duration, f func() error) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = f(); err == nil {
			return nil
		}
		if attempt < attempts {
			time.Sleep(interval)
		}
	}

	return err
}

// configMapResource is the core v1 configmaps resource.
var configMapResource = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

// swapStartedRecorded reports whether a restore started a swap that has not
// completed.
func swapStartedRecorded(kubeClient dynamic.Interface, namespace string) (bool, error) {
	_, err := kubeClient.Resource(configMapResource).Namespace(namespace).Get(
		context.Background(), swapStartedConfigName, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check for an unfinished restore: %w", err)
	}

	return true, nil
}

// recordSwapStarted records that a swap started.
func recordSwapStarted(kubeClient dynamic.Interface, namespace string) error {
	_, err := kubeClient.Resource(configMapResource).Namespace(namespace).Create(
		context.Background(),
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      swapStartedConfigName,
				"namespace": namespace,
			},
		}},
		metav1.CreateOptions{},
	)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// clearSwapStarted records that the swap completed.
func clearSwapStarted(kubeClient dynamic.Interface, namespace string) error {
	err := kubeClient.Resource(configMapResource).Namespace(namespace).Delete(
		context.Background(), swapStartedConfigName, metav1.DeleteOptions{},
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf(
			"the database was restored, but failed to clear the record of the swap in config map %s, "+
				"so a later restore that fails before its swap keeps the control plane down; delete it: %w",
			swapStartedConfigName, err,
		)
	}

	return nil
}

// restoreLockName is the ConfigMap a restore holds while it runs.
const restoreLockName = "threeport-restore-lock"

// acquireRestoreLock claims the restore of a control plane for this command,
// and returns a function that releases it.  Creating the ConfigMap is atomic,
// so of two restores started together only one gets it.
func acquireRestoreLock(kubeClient dynamic.Interface, namespace string) (func() error, error) {
	host, _ := os.Hostname()
	holder := fmt.Sprintf("%s (pid %d)", host, os.Getpid())
	_, err := kubeClient.Resource(configMapResource).Namespace(namespace).Create(
		context.Background(),
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      restoreLockName,
				"namespace": namespace,
			},
			"data": map[string]interface{}{
				"holder":  holder,
				"started": time.Now().UTC().Format(time.RFC3339),
			},
		}},
		metav1.CreateOptions{},
	)
	if k8serrors.IsAlreadyExists(err) {
		existing, getErr := kubeClient.Resource(configMapResource).Namespace(namespace).Get(
			context.Background(), restoreLockName, metav1.GetOptions{},
		)
		otherHolder, otherStarted := "an unknown holder", "an unknown time"
		if getErr == nil {
			data, _, _ := unstructured.NestedStringMap(existing.Object, "data")
			otherHolder, otherStarted = data["holder"], data["started"]
		}
		return nil, fmt.Errorf(
			"another restore of this control plane is running, started at %s by %s, so nothing was changed; "+
				"if it is not running any more, delete config map %s in namespace %s and run the restore again",
			otherStarted, otherHolder, restoreLockName, namespace,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim the restore lock: %w", err)
	}

	return func() error {
		err := kubeClient.Resource(configMapResource).Namespace(namespace).Delete(
			context.Background(), restoreLockName, metav1.DeleteOptions{},
		)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf(
				"failed to release the restore lock; delete config map %s in namespace %s before the next restore: %w",
				restoreLockName, namespace, err,
			)
		}
		return nil
	}, nil
}

// podResource is the core v1 pods resource.
var podResource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

// restoreWritersWait is how long a restore waits for the pods of the
// Deployments it scaled down to be gone, and how often it checks.
var (
	restoreWritersWait         = 5 * time.Minute
	restoreWritersPollInterval = 3 * time.Second
)

// waitForPodsGone waits until none of the pods of the given Deployments is
// left, running, starting or terminating.  A Deployment's readiness says
// nothing about a pod still in its init containers or shutting down.
func waitForPodsGone(kubeClient dynamic.Interface, namespace string, deployments map[string]int64) error {
	selectors := map[string]string{}
	for name := range deployments {
		deployment, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
			context.Background(), name, metav1.GetOptions{},
		)
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to read deployment %s: %w", name, err)
		}
		matchLabels, _, _ := unstructured.NestedStringMap(deployment.Object, "spec", "selector", "matchLabels")
		if len(matchLabels) == 0 {
			return fmt.Errorf("deployment %s selects its pods by no label, so they cannot be checked for", name)
		}
		var terms []string
		for key, value := range matchLabels {
			terms = append(terms, key+"="+value)
		}
		sort.Strings(terms)
		selectors[name] = strings.Join(terms, ",")
	}

	attempts := int(restoreWritersWait/restoreWritersPollInterval) + 1
	return pollEvery(attempts, restoreWritersPollInterval, func() error {
		var left []string
		for name, selector := range selectors {
			pods, err := kubeClient.Resource(podResource).Namespace(namespace).List(
				context.Background(), metav1.ListOptions{LabelSelector: selector},
			)
			if err != nil {
				return fmt.Errorf("failed to list the pods of deployment %s: %w", name, err)
			}
			for _, pod := range pods.Items {
				left = append(left, pod.GetName())
			}
		}
		if len(left) > 0 {
			sort.Strings(left)
			return fmt.Errorf("pods still running after the scale-down: %s", strings.Join(left, ", "))
		}
		return nil
	})
}
