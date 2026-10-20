package v0

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// DbBackupConfigName is the ConfigMap that tells the database migrator
	// where to back the database up to before migrating.
	DbBackupConfigName = "db-backup"

	// DbBackupConfigLocation is where the database migrator mounts it.
	DbBackupConfigLocation = "/etc/threeport/db-backup"

	// DbBackupURIFilename is the key holding the backup location.
	DbBackupURIFilename = "uri"

	// dbMigratorCertsVolumeName is the volume of database client certificates
	// the database migrator connects with.
	dbMigratorCertsVolumeName = "db-migrator-certs"
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
// database runs as.  On a control plane that backs its database up, the
// backup bucket grants this account access by its UID.
//
// It is left in place when it already exists, so a reinstall keeps the UID
// the bucket grants access to.
func (cpi *ControlPlaneInstaller) installDatabaseServiceAccount(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	serviceAccount := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name":      DatabaseServiceAccountName,
				"namespace": cpi.Opts.Namespace,
			},
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

// DatabaseServiceAccountUID returns the UID of the Kubernetes service account
// the database runs as.
func (cpi *ControlPlaneInstaller) DatabaseServiceAccountUID(kubeClient dynamic.Interface) (string, error) {
	serviceAccount, err := kubeClient.Resource(serviceAccountGVR).Namespace(cpi.Opts.Namespace).Get(
		context.Background(), DatabaseServiceAccountName, metav1.GetOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to get the database service account: %w", err)
	}
	if serviceAccount.GetUID() == "" {
		return "", errors.New("the database service account has no UID")
	}

	return string(serviceAccount.GetUID()), nil
}

// serviceAccountGVR is the core v1 serviceaccounts resource.
var serviceAccountGVR = schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}

// installDatabaseBackupConfig creates the ConfigMap that tells the database
// migrator where to back the database up to before migrating, on a control
// plane with a backup bucket.  It is left in place when it already exists, so
// a reinstall, which is not told about the backups, keeps backing up before
// migrations.
func (cpi *ControlPlaneInstaller) installDatabaseBackupConfig(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	if cpi.Opts.GcpBackupBucket == "" {
		return nil
	}

	backupConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      DbBackupConfigName,
				"namespace": cpi.Opts.Namespace,
			},
			"data": map[string]interface{}{
				DbBackupURIFilename: DatabaseBackupURI(cpi.Opts.GcpBackupBucket),
			},
		},
	}
	setPersistent(backupConfig)
	if err := cpi.CreateOrUpdateKubeResource(backupConfig, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create database backup config map: %w", err)
	}

	return nil
}
