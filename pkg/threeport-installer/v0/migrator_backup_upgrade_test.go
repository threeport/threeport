package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// installedAPIServerDeployment returns the API server Deployment the
// installer writes today.
func installedAPIServerDeployment(t *testing.T) *unstructured.Unstructured {
	t.Helper()
	cpi := backupInstaller()
	cpi.Opts.GcpBackupBucket = ""
	kubeClient := installAPIServer(t, cpi)

	return getObject(t, kubeClient, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, ThreeportAPIServiceResourceName)
}

// asInstalledBeforeMigrationBackups returns a Deployment as the installer
// wrote it before the database migrator backed up before migrating: the
// migrator's certificates from the threeport user's secret alone, and no
// backup location.
func asInstalledBeforeMigrationBackups(t *testing.T, deployment *unstructured.Unstructured) *unstructured.Unstructured {
	t.Helper()
	old := deployment.DeepCopy()
	spec := old.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})

	var volumes []interface{}
	for _, volume := range spec["volumes"].([]interface{}) {
		name := volume.(map[string]interface{})["name"]
		if name != dbMigratorCertsVolumeName && name != DbBackupConfigName {
			volumes = append(volumes, volume)
		}
	}
	spec["volumes"] = volumes

	initContainers := spec["initContainers"].([]interface{})
	migrator := initContainers[indexNamed(initContainers, ThreeportDatabaseMigratorContainerName)].(map[string]interface{})
	var mounts []interface{}
	for _, item := range migrator["volumeMounts"].([]interface{}) {
		switch item.(map[string]interface{})["name"] {
		case DbBackupConfigName:
		case dbMigratorCertsVolumeName:
			mounts = append(mounts, map[string]interface{}{"name": dbThreeportCertSecretName, "mountPath": dbMigratorCertsPath})
		default:
			mounts = append(mounts, item)
		}
	}
	migrator["volumeMounts"] = mounts

	return old
}

// podSpecPart returns one field of a Deployment's pod spec.
func podSpecPart(t *testing.T, deployment *unstructured.Unstructured, field string) []interface{} {
	t.Helper()
	part, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", field)
	require.NoError(t, err)

	return part
}

// assertSameContainers checks two lists of containers are the same, their
// mounts in any order, which a pod does not depend on.
func assertSameContainers(t *testing.T, want, got []interface{}) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		wantContainer := want[i].(map[string]interface{})
		gotContainer := got[i].(map[string]interface{})
		assert.ElementsMatch(t, wantContainer["volumeMounts"], gotContainer["volumeMounts"],
			"the mounts of %s a fresh install has", wantContainer["name"])
		withoutMounts := func(container map[string]interface{}) map[string]interface{} {
			copied := map[string]interface{}{}
			for key, value := range container {
				if key != "volumeMounts" {
					copied[key] = value
				}
			}
			return copied
		}
		assert.Equal(t, withoutMounts(wantContainer), withoutMounts(gotContainer),
			"nothing else of %s is changed", wantContainer["name"])
	}
}

// TestAMigratorInstalledBeforeBackupsIsGivenWhatItBacksUpWith covers an
// upgrade of a control plane installed before the database migrator backed
// up before migrating. The upgrade moved its API server to the new image and
// nothing else, so the new migrator found no backup location, took that to
// mean there were no backups, and migrated without one - on a control plane
// whose scheduled backups said otherwise.
func TestAMigratorInstalledBeforeBackupsIsGivenWhatItBacksUpWith(t *testing.T) {
	installed := installedAPIServerDeployment(t)
	old := asInstalledBeforeMigrationBackups(t, installed)
	require.NotEqual(t, podSpecPart(t, installed, "initContainers"), podSpecPart(t, old, "initContainers"),
		"the Deployment from before has to differ, or this covers nothing")

	require.NoError(t, PrepareDatabaseMigratorForBackups(old))

	assert.ElementsMatch(t, podSpecPart(t, installed, "volumes"), podSpecPart(t, old, "volumes"),
		"the volumes a fresh install has")
	assertSameContainers(t, podSpecPart(t, installed, "initContainers"), podSpecPart(t, old, "initContainers"))
	assert.Equal(t, podSpecPart(t, installed, "containers"), podSpecPart(t, old, "containers"))

	t.Run("one that has them is left as it is", func(t *testing.T) {
		current := installedAPIServerDeployment(t)
		require.NoError(t, PrepareDatabaseMigratorForBackups(current))
		assert.Equal(t, installedAPIServerDeployment(t).Object, current.Object)
	})

	t.Run("a Deployment with no migrator is not an API server", func(t *testing.T) {
		deployment := installedAPIServerDeployment(t)
		require.NoError(t, unstructured.SetNestedSlice(deployment.Object, nil, "spec", "template", "spec", "initContainers"))
		assert.ErrorContains(t, PrepareDatabaseMigratorForBackups(deployment), ThreeportDatabaseMigratorContainerName)
	})
}
