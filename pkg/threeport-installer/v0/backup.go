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

// DatabaseBackupURI returns the location in a GCS bucket the database is backed up to.
// The default namespace keeps the collection written before each control plane had its own.
func DatabaseBackupURI(bucket, namespace string) string {
	// keep the original collection for the default namespace
	location := DatabaseName
	if namespace != "" && namespace != ControlPlaneNamespace {
		location = namespace + "/" + DatabaseName
	}
	return fmt.Sprintf("gs://%s/%s?AUTH=implicit", bucket, location)
}

// BackupScheduleBucket returns the bucket the migrator should schedule.
// A replacement leaves it empty because the schedule's first run is immediate.
func (cpi *ControlPlaneInstaller) BackupScheduleBucket(bucket string) string {
	// withhold the schedule when this install copies an existing database
	if bucket == "" || cpi.Opts.FromNamespace != "" {
		return ""
	}
	return bucket
}

// DatabaseBackupScheduleSQL returns the CREATE SCHEDULE statement for uri.
// The schedule runs immediately, keeps an existing schedule, and ignores backups already stored at uri.
func DatabaseBackupScheduleSQL(uri string) string {
	// ignore backups already at uri, or a restore that wrote one cannot create the schedule
	return fmt.Sprintf(
		"CREATE SCHEDULE IF NOT EXISTS %s FOR BACKUP DATABASE %s INTO '%s' RECURRING '%s' FULL BACKUP '%s' WITH SCHEDULE OPTIONS first_run = 'now', ignore_existing_backups;\n",
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

// InstallDatabaseBackupConfig creates the ConfigMap that tells the database
// migrator where to back the database up to before migrating, on a control
// plane with a backup bucket.  It is left in place when it already exists, so
// a reinstall, which is not told about the backups, keeps backing up before
// migrations.
func (cpi *ControlPlaneInstaller) InstallDatabaseBackupConfig(
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
				DbBackupURIFilename: DatabaseBackupURI(cpi.Opts.GcpBackupBucket, cpi.Opts.Namespace),
			},
		},
	}
	setPersistent(backupConfig)
	if err := cpi.CreateOrUpdateKubeResource(backupConfig, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create database backup config map: %w", err)
	}

	return nil
}

// dbMigratorCertsPath is where the database migrator finds its database
// client certificates.
const dbMigratorCertsPath = "/etc/threeport/db-certs"

// dbMigratorCertsVolume is the database migrator's client certificates: the
// threeport user's, which it connects as, and root's, which it backs the
// database up as before migrating, since only an admin may back up to cloud
// storage with the node's own identity.
func dbMigratorCertsVolume() map[string]interface{} {
	return map[string]interface{}{
		"name": dbMigratorCertsVolumeName,
		"projected": map[string]interface{}{
			"sources": []interface{}{
				map[string]interface{}{
					"secret": map[string]interface{}{"name": dbThreeportCertSecretName},
				},
				map[string]interface{}{
					"secret": map[string]interface{}{
						"name": dbRootCertSecretName,
						"items": []interface{}{
							map[string]interface{}{"key": "client.root.crt", "path": "client.root.crt"},
							map[string]interface{}{"key": "client.root.key", "path": "client.root.key"},
						},
					},
				},
			},
		},
	}
}

// dbMigratorCertsMount mounts dbMigratorCertsVolume where the migrator
// looks for its certificates.
func dbMigratorCertsMount() map[string]interface{} {
	return map[string]interface{}{"name": dbMigratorCertsVolumeName, "mountPath": dbMigratorCertsPath}
}

// dbBackupConfigVolume is where the database is backed up to, on a control
// plane that backs it up.  It is optional, so on any other the migrator finds
// the directory empty and the API server still starts.
func dbBackupConfigVolume() map[string]interface{} {
	return map[string]interface{}{
		"name": DbBackupConfigName,
		"configMap": map[string]interface{}{
			"name":     DbBackupConfigName,
			"optional": true,
		},
	}
}

// dbBackupConfigMount mounts dbBackupConfigVolume where the migrator reads it.
func dbBackupConfigMount() map[string]interface{} {
	return map[string]interface{}{"name": DbBackupConfigName, "mountPath": DbBackupConfigLocation}
}

// PrepareDatabaseMigratorForBackups gives the database migrator of an API
// server Deployment written before migrations were backed up what it needs
// to back up: the backup location, and root's certificate beside the
// threeport user's.  It leaves one that has them as it is.
//
// An upgrade that only moves such a Deployment to a new image starts a
// migrator that backs up before migrating with neither mounted.  It finds no
// backup location, takes that to mean the control plane has no backups, and
// migrates without one - on a control plane whose scheduled backups say it
// does.  Whatever brings that Deployment to a new image has to bring it in
// line first.
func PrepareDatabaseMigratorForBackups(deployment *unstructured.Unstructured) error {
	podSpec, found, err := unstructured.NestedFieldNoCopy(deployment.Object, "spec", "template", "spec")
	if err != nil || !found {
		return fmt.Errorf("deployment %s has no pod spec: %v", deployment.GetName(), err)
	}
	spec, ok := podSpec.(map[string]interface{})
	if !ok {
		return fmt.Errorf("deployment %s has a pod spec that is not an object", deployment.GetName())
	}

	// the volumes, added beside the ones the pod already has
	volumes, _ := spec["volumes"].([]interface{})
	for _, volume := range []map[string]interface{}{dbMigratorCertsVolume(), dbBackupConfigVolume()} {
		if indexNamed(volumes, volume["name"].(string)) < 0 {
			volumes = append(volumes, volume)
		}
	}
	spec["volumes"] = volumes

	// the migrator's mounts: its certificates where it already looks for
	// them, now from the volume that has root's too, and the backup location
	initContainers, _ := spec["initContainers"].([]interface{})
	index := indexNamed(initContainers, ThreeportDatabaseMigratorContainerName)
	if index < 0 {
		return fmt.Errorf(
			"deployment %s has no %s init container", deployment.GetName(), ThreeportDatabaseMigratorContainerName,
		)
	}
	migrator, ok := initContainers[index].(map[string]interface{})
	if !ok {
		return fmt.Errorf("the %s init container is not an object", ThreeportDatabaseMigratorContainerName)
	}
	mounts, _ := migrator["volumeMounts"].([]interface{})
	certs := -1
	for i, item := range mounts {
		if mount, ok := item.(map[string]interface{}); ok && mount["mountPath"] == dbMigratorCertsPath {
			certs = i
		}
	}
	if certs < 0 {
		mounts = append(mounts, dbMigratorCertsMount())
	} else {
		mounts[certs] = dbMigratorCertsMount()
	}
	if indexNamed(mounts, DbBackupConfigName) < 0 {
		mounts = append(mounts, dbBackupConfigMount())
	}
	migrator["volumeMounts"] = mounts

	return nil
}

// indexNamed returns the index of the item of a pod spec list - a volume, a
// container, a mount - with the given name, or -1.
func indexNamed(items []interface{}, name string) int {
	for i, item := range items {
		if object, ok := item.(map[string]interface{}); ok && object["name"] == name {
			return i
		}
	}

	return -1
}
