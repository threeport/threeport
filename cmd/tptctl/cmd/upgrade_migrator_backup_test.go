package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

const upgradeTestNamespace = "threeport-control-plane"

var (
	deploymentsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	configMapsGVR  = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
)

// rolledOutDeployment returns a Deployment whose rollout is complete, with
// the given pod spec.
func rolledOutDeployment(name string, podSpec map[string]interface{}) *unstructured.Unstructured {
	deployment := deploymentWithStatus(name, 1, 1, 1, 1, 1, 1)
	deployment.Object["spec"].(map[string]interface{})["template"] = map[string]interface{}{"spec": podSpec}

	return deployment
}

// apiServerFromBeforeMigrationBackups is the API server Deployment as the
// installer wrote it before the database migrator backed up: the migrator
// mounts the threeport user's certificate alone, and no backup location.
func apiServerFromBeforeMigrationBackups() *unstructured.Unstructured {
	return rolledOutDeployment(installer.ThreeportRestApi.ServiceResourceName, map[string]interface{}{
		"initContainers": []interface{}{
			map[string]interface{}{
				"name":  installer.ThreeportDatabaseMigratorContainerName,
				"image": "ghcr.io/threeport/threeport-database-migrator:v0.6.0",
				"volumeMounts": []interface{}{
					map[string]interface{}{"name": "db-threeport-cert", "mountPath": "/etc/threeport/db-certs"},
				},
			},
		},
		"containers": []interface{}{
			container(installer.ThreeportApiServerContainerName, "ghcr.io/threeport/threeport-rest-api:v0.6.0"),
		},
		"volumes": []interface{}{
			map[string]interface{}{"name": "db-threeport-cert", "secret": map[string]interface{}{"secretName": "db-threeport-cert"}},
		},
	})
}

// upgradeTestMapper maps what the upgrade reads and writes.
func upgradeTestMapper() *meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}, {Group: "apps", Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	restMapper := meta.RESTMapper(mapper)

	return &restMapper
}

// migratorMounts returns where the upgraded API server's migrator mounts
// each volume, and the pod's volumes by name.
func migratorMounts(t *testing.T, kubeClient *dynamicfake.FakeDynamicClient) (map[string]string, map[string]bool) {
	t.Helper()
	deployment, err := kubeClient.Resource(deploymentsGVR).Namespace(upgradeTestNamespace).Get(
		context.Background(), installer.ThreeportRestApi.ServiceResourceName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/threeport/threeport-database-migrator:v0.7.0",
		imageOf(t, deployment, "initContainers", installer.ThreeportDatabaseMigratorContainerName))

	initContainers, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "initContainers")
	require.NoError(t, err)
	migrator, err := installer.ContainerNamed(initContainers, installer.ThreeportDatabaseMigratorContainerName)
	require.NoError(t, err)
	mounts := map[string]string{}
	for _, item := range migrator["volumeMounts"].([]interface{}) {
		mount := item.(map[string]interface{})
		mounts[mount["name"].(string)] = mount["mountPath"].(string)
	}

	volumes, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "volumes")
	require.NoError(t, err)
	names := map[string]bool{}
	for _, item := range volumes {
		names[item.(map[string]interface{})["name"].(string)] = true
	}

	return mounts, names
}

// TestTheUpgradeGivesTheMigratorWhatItBacksUpWith covers an upgrade of a
// control plane installed before the database migrator backed up before
// migrating. The upgrade moved the API server to the new image and nothing
// else, so the new migrator found no backup location and migrated without a
// backup, even on a control plane whose scheduled backups go to a bucket.
func TestTheUpgradeGivesTheMigratorWhatItBacksUpWith(t *testing.T) {
	originalAttempts, originalPoll := installer.RolloutAttempts, installer.RolloutPollSeconds
	installer.RolloutAttempts, installer.RolloutPollSeconds = 1, 0
	t.Cleanup(func() { installer.RolloutAttempts, installer.RolloutPollSeconds = originalAttempts, originalPoll })

	upgrade := func(t *testing.T, bucket string) *dynamicfake.FakeDynamicClient {
		t.Helper()
		kubeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(),
			map[schema.GroupVersionResource]string{deploymentsGVR: "DeploymentList", configMapsGVR: "ConfigMapList"},
			apiServerFromBeforeMigrationBackups(),
			rolledOutDeployment(installer.ThreeportAgentDeployName, map[string]interface{}{
				"containers": []interface{}{
					container(installer.ThreeportAgentContainerName, "ghcr.io/threeport/threeport-agent:v0.6.0"),
				},
			}),
		)
		require.NoError(t, installer.UpdateDeploymentImageTag("v0.7.0", "", upgradeTestNamespace, bucket, kubeClient, upgradeTestMapper(), nil, true))

		return kubeClient
	}

	t.Run("on a control plane with scheduled backups", func(t *testing.T) {
		kubeClient := upgrade(t, "threeport-prod-backups-acme-prod")

		backupConfig, err := kubeClient.Resource(configMapsGVR).Namespace(upgradeTestNamespace).Get(
			context.Background(), installer.DbBackupConfigName, metav1.GetOptions{},
		)
		require.NoError(t, err, "the migrator is told where to back up")
		uri, _, _ := unstructured.NestedString(backupConfig.Object, "data", installer.DbBackupURIFilename)
		assert.Equal(t, installer.DatabaseBackupURI("threeport-prod-backups-acme-prod", ""), uri)

		mounts, volumes := migratorMounts(t, kubeClient)
		assert.Equal(t, installer.DbBackupConfigLocation, mounts[installer.DbBackupConfigName])
		assert.Equal(t, "/etc/threeport/db-certs", mounts["db-migrator-certs"], "with root's certificate beside the threeport user's")
		assert.True(t, volumes[installer.DbBackupConfigName])
		assert.True(t, volumes["db-migrator-certs"])

		// the location exists before any pod on the new image can start
		configMapCreated, apiServerUpdated := -1, -1
		for i, action := range kubeClient.Actions() {
			switch {
			case action.Matches("create", "configmaps"):
				configMapCreated = i
			case action.Matches("update", "deployments") && apiServerUpdated < 0:
				apiServerUpdated = i
			}
		}
		require.GreaterOrEqual(t, configMapCreated, 0)
		assert.Less(t, configMapCreated, apiServerUpdated)
	})

	t.Run("on a control plane without backups", func(t *testing.T) {
		kubeClient := upgrade(t, "")

		_, err := kubeClient.Resource(configMapsGVR).Namespace(upgradeTestNamespace).Get(
			context.Background(), installer.DbBackupConfigName, metav1.GetOptions{},
		)
		assert.Error(t, err, "there is nowhere to back up to")

		// mounted all the same, as a fresh install mounts them, so a backup
		// location added later reaches the migrator
		mounts, _ := migratorMounts(t, kubeClient)
		assert.Equal(t, installer.DbBackupConfigLocation, mounts[installer.DbBackupConfigName])
		assert.Equal(t, "/etc/threeport/db-certs", mounts["db-migrator-certs"])
	})
}
