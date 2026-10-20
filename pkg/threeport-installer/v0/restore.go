package v0

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// A restore replaces the control plane database with one of its backups.
// The backup is restored under another name first and only swapped in once
// it is complete, so a restore that fails leaves the database as it was.
const (
	// LatestBackup restores the most recent backup in the bucket.
	LatestBackup = "LATEST"

	// restoreDatabaseJobName is the Job that runs the restore.
	restoreDatabaseJobName = "threeport-restore-database"

	// restoreDatabaseJobBackoffLimit is how many failed pods end the restore.
	// Every statement of the restore can be run again, so a retry is safe.
	restoreDatabaseJobBackoffLimit = 2

	// restoreDatabaseTimeout is how long the restore Job may run.
	restoreDatabaseTimeout = 30 * time.Minute

	// restoringDatabaseSuffix names the database a backup is restored into
	// before it replaces the live one.
	restoringDatabaseSuffix = "_restore"
)

// backupPathPattern matches a backup's path in a bucket, as CockroachDB
// names it and `SHOW BACKUPS` lists it: /YYYY/MM/DD-HHMMSS.SS.
var backupPathPattern = regexp.MustCompile(`^/\d{4}/\d{2}/\d{2}-\d{6}\.\d{2}$`)

// bucketNamePattern matches a GCS bucket name.
var bucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$`)

// ErrInvalidBackup is returned for a backup that names neither the latest
// backup nor a backup path.
var ErrInvalidBackup = errors.New("invalid backup")

// DatabaseRestoreStatements returns the statements that replace a database
// with one of its backups, each to be run on its own in one session:
//
//   - the backup is restored under another name, after removing what an
//     earlier restore that failed may have left under it
//   - the live database is dropped and the restored one renamed into its place
//   - the restored objects are handed back to the user that owns them.  A
//     restore makes the restoring user their owner and leaves the database
//     without its grants, which would lock the API server out of its data.
//   - a full backup of the restored database is taken into scheduleURI, where
//     the scheduled backups go.  Its tables are new to the backups already
//     there, so the scheduled incremental backups would fail on top of them
//     until the next scheduled full one.  With no scheduleURI there is no
//     schedule to keep going, and no backup is taken.
//
// The backup is restored from sourceURI, which is the schedule's own
// location unless the backups were copied elsewhere.  Nothing is dropped
// until the backup is fully restored, and running every statement again
// after a failure part way finishes the restore.
func DatabaseRestoreStatements(databaseName, ownerName, sourceURI, scheduleURI, backup string) ([]string, error) {
	from, err := backupSource(backup)
	if err != nil {
		return nil, err
	}
	restoring := databaseName + restoringDatabaseSuffix

	statements := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", restoring),
		fmt.Sprintf("RESTORE DATABASE %s FROM %s IN '%s' WITH new_db_name = '%s'", databaseName, from, sourceURI, restoring),
		fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", databaseName),
		fmt.Sprintf("ALTER DATABASE %s RENAME TO %s", restoring, databaseName),
		fmt.Sprintf("USE %s", databaseName),
		fmt.Sprintf("REASSIGN OWNED BY root TO %s", ownerName),
		fmt.Sprintf("ALTER DATABASE %s OWNER TO root", databaseName),
		"ALTER SCHEMA public OWNER TO root",
		fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", databaseName, ownerName),
	}
	if scheduleURI != "" {
		statements = append(statements, fmt.Sprintf("BACKUP DATABASE %s INTO '%s'", databaseName, scheduleURI))
	}

	return statements, nil
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
// it has no schedule.  The control plane is scaled down so nothing writes to
// the database while it is replaced, and scaled back up afterwards, whether
// the restore succeeded or not.
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
	statements, err := DatabaseRestoreStatements(
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

	// record the replica counts to scale back to
	replicas, err := cpi.deploymentReplicas(kubeClient, namespace)
	if err != nil {
		return err
	}

	// stop everything that writes to the database; a scale-down that fails
	// part way may already have stopped some of it, so the control plane is
	// brought back before the failure is returned
	if err := cpi.scaleDownDeployments(kubeClient, namespace); err != nil {
		scaleDownErr := fmt.Errorf("failed to scale control plane down before the restore: %w", err)
		if err := cpi.scaleDeployments(kubeClient, namespace, replicas); err != nil {
			return errors.Join(scaleDownErr, err)
		}
		return scaleDownErr
	}

	// restore, then bring the control plane back either way
	restoreErr := cpi.runRestoreDatabaseJob(kubeClient, namespace, statements)
	if err := cpi.scaleDeployments(kubeClient, namespace, replicas); err != nil {
		return errors.Join(restoreErr, err)
	}
	if restoreErr != nil {
		return restoreErr
	}

	// wait for the API server to serve the restored database
	fmt.Printf("Info: waiting for rest-api deployment to become ready (timeout %s)\n", restApiDeploymentReadyTimeout)
	if err := cpi.waitForRestAPIReady(kubeClient, namespace, restApiDeploymentReadyTimeout); err != nil {
		return fmt.Errorf("rest-api did not become ready after the restore: %w", err)
	}

	return nil
}

// deploymentReplicas returns the replica count of each control plane
// Deployment a restore scales down.
func (cpi *ControlPlaneInstaller) deploymentReplicas(
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
		count, found, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
		if !found {
			// an unset count is one replica
			count = 1
		}
		replicas[deployment.GetName()] = count
	}

	return replicas, nil
}

// scaleDeployments sets each Deployment to its replica count.
func (cpi *ControlPlaneInstaller) scaleDeployments(
	kubeClient dynamic.Interface,
	namespace string,
	replicas map[string]int64,
) error {
	fmt.Println("Info: scaling control plane deployments back up")
	var scaleErrors []error
	for name, count := range replicas {
		patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, count))
		if _, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Patch(
			context.Background(),
			name,
			"application/strategic-merge-patch+json",
			patch,
			metav1.PatchOptions{},
		); err != nil && !k8serrors.IsNotFound(err) {
			scaleErrors = append(scaleErrors, fmt.Errorf("failed to scale deployment %s to %d: %w", name, count, err))
		}
	}

	return errors.Join(scaleErrors...)
}

// runRestoreDatabaseJob runs the restore statements in a one-shot Job with
// the database's own client, and removes the Job once it succeeds.
func (cpi *ControlPlaneInstaller) runRestoreDatabaseJob(
	kubeClient dynamic.Interface,
	namespace string,
	statements []string,
) error {
	// a leftover job is removed first, since its pod template cannot change
	if err := deleteJob(kubeClient, namespace, restoreDatabaseJobName); err != nil {
		return err
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
				"name":      restoreDatabaseJobName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"backoffLimit":          int64(restoreDatabaseJobBackoffLimit),
				"activeDeadlineSeconds": int64(restoreDatabaseTimeout.Seconds()),
				"template": map[string]interface{}{
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
		return fmt.Errorf("failed to create database restore job: %w", err)
	}

	if err := waitForRestoreJob(kubeClient, namespace); err != nil {
		return err
	}

	return deleteJob(kubeClient, namespace, restoreDatabaseJobName)
}

// waitForRestoreJob polls the restore Job until it succeeds or fails.  A
// failed restore Job is left in place so its pod logs can be read.
func waitForRestoreJob(kubeClient dynamic.Interface, namespace string) error {
	var jobFailed error
	attempts := int(restoreDatabaseTimeout/(5*time.Second)) + 1

	if err := util.Retry(attempts, 5, func() error {
		job, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
			context.Background(), restoreDatabaseJobName, metav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to read database restore job status: %w", err)
		}

		// a job past its backoff limit or its deadline reports a failed condition
		conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
		for _, item := range conditions {
			condition, ok := item.(map[string]interface{})
			if ok && condition["type"] == "Failed" && condition["status"] == "True" {
				jobFailed = fmt.Errorf(
					"database restore job %s failed (%v): inspect its pod logs in namespace %s; the database was not replaced unless the logs show the restore completed",
					restoreDatabaseJobName, condition["reason"], namespace,
				)
				return nil
			}
		}

		succeeded, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "succeeded")
		if succeeded > 0 {
			return nil
		}

		return fmt.Errorf("database restore job %s has not completed", restoreDatabaseJobName)
	}); err != nil {
		return fmt.Errorf("database restore did not complete: %w", err)
	}

	return jobFailed
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
