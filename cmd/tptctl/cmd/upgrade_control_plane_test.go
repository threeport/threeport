package cmd

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// deploymentWith builds the part of a deployment this command walks.
func deploymentWith(initContainers, containers []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"initContainers": initContainers,
					"containers":     containers,
				},
			},
		},
	}}
}

func container(name, image string) map[string]interface{} {
	return map[string]interface{}{"name": name, "image": image}
}

// imageOf reads back what a named container ended up pointing at.
func imageOf(t *testing.T, deployment *unstructured.Unstructured, key, name string) string {
	t.Helper()
	list, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", key)
	require.NoError(t, err)
	require.True(t, found)

	found_, err := containerNamed(list, name)
	require.NoError(t, err)

	return found_["image"].(string)
}

// TestRetaggingFindsContainersByNameNotPosition covers the failure the
// positions produced.
//
// The containers were looked up by index: 1 for the database migrator, 1 for
// the agent's own container, 0 for everything else. A position is not a
// property of a container, it is a property of whatever else is in the list -
// so a sidecar, or one of the additional init containers the installer
// already supports, moved the thing being retagged. The upgrade then retagged
// something else, or ran off the end of the list and panicked.
func TestRetaggingFindsContainersByNameNotPosition(t *testing.T) {
	t.Run("the api server is retagged with its init containers reordered", func(t *testing.T) {
		deployment := deploymentWith(
			[]interface{}{
				// the migrator is no longer second
				container(installer.ThreeportDatabaseMigratorContainerName, "ghcr.io/threeport/threeport-database-migrator:v0.6.0"),
				container("db-init", "ghcr.io/threeport/threeport-database-migrator:v0.6.0"),
				container("something-a-module-added", "example.com/other:v1"),
			},
			[]interface{}{
				container("a-sidecar", "example.com/sidecar:v1"),
				container(installer.ThreeportApiServerContainerName, "ghcr.io/threeport/threeport-rest-api:v0.6.0"),
			},
		)

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", installer.ThreeportRestApi.Name)
		require.NoError(t, err)
		assert.Equal(
			t, "ghcr.io/threeport/threeport-database-migrator:v0.7.0",
			imageOf(t, deployment, "initContainers", installer.ThreeportDatabaseMigratorContainerName),
		)
		assert.Equal(
			t, "ghcr.io/threeport/threeport-rest-api:v0.7.0",
			imageOf(t, deployment, "containers", installer.ThreeportApiServerContainerName),
		)
		assert.Equal(
			t, "example.com/sidecar:v1",
			imageOf(t, deployment, "containers", "a-sidecar"),
			"a container this command does not own must be left alone",
		)
	})

	t.Run("the agent is retagged with its containers reordered", func(t *testing.T) {
		deployment := deploymentWith(nil, []interface{}{
			// manager first, where the index expected the proxy
			container(installer.ThreeportAgentContainerName, "ghcr.io/threeport/threeport-agent:v0.6.0"),
			container("kube-rbac-proxy", "ghcr.io/kube-rbac-proxy/kube-rbac-proxy:v0.22.0"),
		})

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", installer.ThreeportAgent.Name)
		require.NoError(t, err)
		assert.Equal(
			t, "ghcr.io/threeport/threeport-agent:v0.7.0",
			imageOf(t, deployment, "containers", installer.ThreeportAgentContainerName),
		)
		assert.Equal(
			t, "ghcr.io/kube-rbac-proxy/kube-rbac-proxy:v0.22.0",
			imageOf(t, deployment, "containers", "kube-rbac-proxy"),
			"the vendored sidecar is not threeport's to retag",
		)
	})

	t.Run("a controller is retagged by its own name", func(t *testing.T) {
		deployment := deploymentWith(nil, []interface{}{
			container("kubernetes-workload-controller", "ghcr.io/threeport/threeport-kubernetes-workload-controller:v0.6.0"),
		})

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", "kubernetes-workload-controller")
		require.NoError(t, err)
		assert.Equal(
			t, "ghcr.io/threeport/threeport-kubernetes-workload-controller:v0.7.0",
			imageOf(t, deployment, "containers", "kubernetes-workload-controller"),
		)
	})
}

// TestRetaggingReportsWhatItCouldNotFind covers the shapes that used to
// panic.
//
// An index into a shorter list, and a type assertion without the comma, both
// end the process. An upgrade that cannot find what it was going to change
// should say so and leave the cluster as it was.
func TestRetaggingReportsWhatItCouldNotFind(t *testing.T) {
	t.Run("a container that is not there names what is", func(t *testing.T) {
		deployment := deploymentWith(nil, []interface{}{
			container("something-else", "example.com/other:v1"),
		})

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", "kubernetes-workload-controller")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kubernetes-workload-controller")
		assert.Contains(t, err.Error(), "something-else", "the error says what the pod does have")
	})

	t.Run("an init container list shorter than the index used to be", func(t *testing.T) {
		deployment := deploymentWith(
			[]interface{}{container("db-init", "example.com/init:v1")},
			[]interface{}{container(installer.ThreeportApiServerContainerName, "example.com/api:v1")},
		)

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", installer.ThreeportRestApi.Name)
		require.Error(t, err, "indexing position 1 of a one-element list is a panic, not an error")
		assert.Contains(t, err.Error(), installer.ThreeportDatabaseMigratorContainerName)
	})

	t.Run("a container with no image", func(t *testing.T) {
		deployment := deploymentWith(nil, []interface{}{
			map[string]interface{}{"name": "kubernetes-workload-controller"},
		})

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", "kubernetes-workload-controller")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no image to retag")
	})

	t.Run("a container whose image is not a string", func(t *testing.T) {
		deployment := deploymentWith(nil, []interface{}{
			map[string]interface{}{"name": "kubernetes-workload-controller", "image": 42},
		})

		_, err := updateImageTagInDeployment(deployment, "v0.7.0", "kubernetes-workload-controller")
		require.Error(t, err, "an unchecked assertion here was a panic")
	})
}

// TestTheContainerNamesMatchWhatTheInstallerWrites covers the two packages
// agreeing.
//
// This command finds a container by the name the installer gave it. If those
// two ever stopped matching, the upgrade would report that it could not find
// what to retag - which is at least loud, but only because they are constants
// shared between the packages rather than strings written twice.
func TestTheContainerNamesMatchWhatTheInstallerWrites(t *testing.T) {
	for component, expected := range map[string]string{
		installer.ThreeportRestApi.Name:  installer.ThreeportApiServerContainerName,
		installer.ThreeportAgent.Name:    installer.ThreeportAgentContainerName,
		"kubernetes-workload-controller": "kubernetes-workload-controller",
	} {
		assert.Equal(
			t, expected, imageContainerName(component),
			"%s carries its image in a container named %s", component, expected,
		)
	}

	assert.NotEqual(
		t, installer.ThreeportRestApi.Name, installer.ThreeportApiServerContainerName,
		"these differ, which is the reason this mapping exists rather than using the component name",
	)
	assert.NotEqual(
		t, installer.ThreeportAgent.Name, installer.ThreeportAgentContainerName,
		"and so do these",
	)
}

// deploymentWithStatus builds a deployment the rollout wait can read.
func deploymentWithStatus(name string, generation, observed, desired, updated, total, available int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":       name,
			"namespace":  "threeport-control-plane",
			"generation": generation,
		},
		"spec": map[string]interface{}{"replicas": desired},
		"status": map[string]interface{}{
			"observedGeneration": observed,
			"updatedReplicas":    updated,
			"replicas":           total,
			"availableReplicas":  available,
		},
	}}
}

// TestTheUpgradeWaitsForTheRolloutRatherThanTheApiCall covers what "success"
// used to mean.
//
// Update returning no error means the API server accepted the new spec, not
// that anything is running it. The command reported success there, so an
// upgrade printed that it worked while the new pods were still pulling,
// crash looping, or failing to start at all.
func TestTheUpgradeWaitsForTheRolloutRatherThanTheApiCall(t *testing.T) {
	resource := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	listKinds := map[schema.GroupVersionResource]string{resource: "DeploymentList"}

	// seconds, not minutes, for the cases that are meant to give up
	originalAttempts, originalPoll := rolloutAttempts, rolloutPollSeconds
	rolloutAttempts, rolloutPollSeconds = 2, 1
	t.Cleanup(func() { rolloutAttempts, rolloutPollSeconds = originalAttempts, originalPoll })

	for name, test := range map[string]struct {
		deployment *unstructured.Unstructured
		wantErr    string
	}{
		"every replica is on the new image and available": {
			// generation observed, one desired, one updated, one total, one available
			deployment: deploymentWithStatus("done", 2, 2, 1, 1, 1, 1),
		},
		"the controller has not seen the new spec yet": {
			deployment: deploymentWithStatus("unobserved", 2, 1, 1, 1, 1, 1),
			wantErr:    "not been observed at generation 2",
		},
		"a replica has not been updated yet": {
			deployment: deploymentWithStatus("mid-rollout", 2, 2, 2, 1, 1, 1),
			wantErr:    "1 of 2 replica(s) on the new image",
		},
		// the case a readyReplicas check cannot see: with one replica and the
		// default rolling update, kubernetes creates the new pod while the old
		// one is still running and ready. updatedReplicas is 1 and
		// readyReplicas is 1 - but the one that is ready is the old one.
		"the old pod is still up while the new one will not start": {
			deployment: deploymentWithStatus("surging", 2, 2, 1, 1, 2, 1),
			wantErr:    "still has 1 replica(s) on the old image",
		},
		"the new pod replaced the old one but is not available": {
			deployment: deploymentWithStatus("crashlooping", 2, 2, 1, 1, 1, 0),
			wantErr:    "0 of 1 new replica(s) available",
		},
	} {
		t.Run(name, func(t *testing.T) {
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
				runtime.NewScheme(), listKinds, test.deployment,
			)

			err := waitForRollout(
				client, resource, "threeport-control-plane", test.deployment.GetName(),
				[]string{"ghcr.io/threeport/threeport-rest-api:v0.6.0"},
			)
			if test.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err, "an upgrade that reports success here reports it too early")
			assert.Contains(t, err.Error(), test.wantErr)
			assert.Contains(t, err.Error(), test.deployment.GetName(), "the operator needs to know which one")
			assert.Contains(
				t, err.Error(), "ghcr.io/threeport/threeport-rest-api:v0.6.0",
				"the advice to go back is only advice if it says what to go back to",
			)
		})
	}
}
