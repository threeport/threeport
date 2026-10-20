package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
)

// The database a control plane backs up and the schedule it does it on.
const (
	// DatabaseName is the database that holds every API object.
	DatabaseName = database.ThreeportDatabaseName

	// DatabaseBackupScheduleLabel names the backup schedule in CockroachDB,
	// and makes creating it again a no-op.
	DatabaseBackupScheduleLabel = "threeport_api_backup"

	// DatabaseBackupRecurrence is how often an incremental backup is taken.
	DatabaseBackupRecurrence = "@hourly"

	// DatabaseFullBackupRecurrence is how often a full backup is taken, which
	// the incremental backups until the next one build on.
	DatabaseFullBackupRecurrence = "@daily"
)

// DatabaseBackupURI returns the location in a GCS bucket the database is
// backed up to.  The database authenticates as the GCP service account its
// Kubernetes service account is bound to, rather than with a key.
func DatabaseBackupURI(bucket string) string {
	return fmt.Sprintf("gs://%s/%s?AUTH=implicit", bucket, DatabaseName)
}

// DatabaseBackupScheduleSQL returns the statement that schedules the
// database's backups into the given URI: an incremental backup every hour
// on top of a full one every day, the first taken as soon as the schedule
// is created.  It does nothing when the schedule already exists.
func DatabaseBackupScheduleSQL(uri string) string {
	return fmt.Sprintf(
		"CREATE SCHEDULE IF NOT EXISTS %s FOR BACKUP DATABASE %s INTO '%s' RECURRING '%s' FULL BACKUP '%s' WITH SCHEDULE OPTIONS first_run = 'now';\n",
		DatabaseBackupScheduleLabel,
		DatabaseName,
		uri,
		DatabaseBackupRecurrence,
		DatabaseFullBackupRecurrence,
	)
}

// installDatabaseServiceAccount creates the Kubernetes service account the
// database runs as.  On a control plane that backs its database up, it is
// annotated to act as the GCP service account that writes the backups.
//
// It is left in place when it already exists, so a reinstall, which is not
// told about the backups, does not remove the annotation the database writes
// its backups with.
func (cpi *ControlPlaneInstaller) installDatabaseServiceAccount(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	metadata := map[string]interface{}{
		"name":      DatabaseServiceAccountName,
		"namespace": cpi.Opts.Namespace,
	}
	if cpi.Opts.GcpBackupServiceAccountEmail != "" {
		metadata["annotations"] = map[string]interface{}{
			"iam.gke.io/gcp-service-account": cpi.Opts.GcpBackupServiceAccountEmail,
		}
	}

	serviceAccount := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata":   metadata,
			// the database never calls the Kubernetes API
			"automountServiceAccountToken": false,
		},
	}
	setPersistent(serviceAccount)
	if err := cpi.CreateOrUpdateKubeResource(serviceAccount, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create database service account: %w", err)
	}

	return nil
}
