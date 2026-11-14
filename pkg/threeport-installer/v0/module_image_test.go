package v0

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestModuleKebab checks the image-name segment of a module API name.
func TestModuleKebab(t *testing.T) {
	// accept a module API name and reject anything else
	assert.Equal(t, "acme", ModuleKebab("example.com/acme-module-api"))
	assert.Equal(t, "", ModuleKebab("example.com/threeport-api"))
	assert.Equal(t, "", ModuleKebab(""))
}

// TestModuleImages checks the image references a retarget writes.
func TestModuleImages(t *testing.T) {
	// build each image and leave it empty when a part is missing
	assert.Equal(
		t,
		"repo.example/threeport-acme-rest-api:v1",
		ModuleRestAPIImage("repo.example", "acme", "v1"),
	)
	assert.Equal(
		t,
		"repo.example/threeport-acme-database-migrator:v1",
		ModuleMigratorImage("repo.example", "acme", "v1"),
	)
	assert.Equal(
		t,
		"repo.example/threeport-acme-widget-controller:v1",
		ModuleControllerImage(
			"repo.example",
			"threeport-acme/threeport-acme-widget-controller",
			"v1",
		),
	)
	assert.Equal(
		t,
		"repo.example/threeport-acme-tptctl:v1",
		ModuleTptctlImage("repo.example", "acme", "v1"),
	)
	assert.Equal(t, "", ModuleRestAPIImage("", "acme", "v1"))
	assert.Equal(t, "", ModuleRestAPIImage("repo.example", "acme", ""))
	assert.Equal(t, "threeport-acme-api-server", ModuleAPIDeploymentName("acme"))
	assert.Equal(
		t,
		"acme-widget-controller",
		ModuleControllerContainerName("ns/threeport-acme-widget-controller"),
	)
}

// TestModuleCopiedSecretNames checks the secrets copied into a module namespace.
func TestModuleCopiedSecretNames(t *testing.T) {
	// match the installer secret list
	assert.Equal(t, []string{
		dbRootCertSecretName,
		dbThreeportCertSecretName,
		EncryptionKeySecret,
		controllerConfigSecretName,
		ThreeportApiConfigSecret,
		NatsCACertSecretName,
	}, ModuleCopiedSecretNames())
}

// TestModuleApiConfigSecret checks the module API config secret contents.
func TestModuleApiConfigSecret(t *testing.T) {
	// render the endpoint, namespace, and auth flag
	secret := ModuleApiConfigSecret("threeport-acme", "threeport-acme-api-server", true)
	config, found, err := unstructured.NestedString(secret.Object, "stringData", "env")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, `MODULE_API_ENDPOINT=threeport-acme-api-server.threeport-acme.svc.cluster.local
MODULE_NAMESPACE=threeport-acme
THREEPORT_AUTH_ENABLED=true
`, config)
	assert.Equal(t, ModuleApiConfigSecretName, secret.GetName())
	assert.Equal(t, "threeport-acme", secret.GetNamespace())
}

// TestSetNamedContainerImageLeavesAnEmptyImage checks that a blank image is not written.
func TestSetNamedContainerImageLeavesAnEmptyImage(t *testing.T) {
	// leave both containers on their current images
	deployment := imageRetargetDeployment()

	require.NoError(t, SetNamedContainerImage(deployment, "containers", moduleAPIContainerName, ""))
	require.NoError(t, SetNamedContainerImage(deployment, "initContainers", moduleMigratorContainerName, ""))

	assert.Equal(t, "old-api", containerImage(t, deployment, "containers", moduleAPIContainerName))
	assert.Equal(t, "old-migrator", containerImage(t, deployment, "initContainers", moduleMigratorContainerName))
}

// TestSetNamedContainerImageReplacesTheNamedContainer checks an image change and a restart.
func TestSetNamedContainerImageReplacesTheNamedContainer(t *testing.T) {
	// replace the named containers and stamp the restart annotation
	deployment := imageRetargetDeployment()

	require.NoError(t, SetNamedContainerImage(
		deployment,
		"containers",
		moduleAPIContainerName,
		"repo.example/threeport-acme-rest-api:v2",
	))
	require.NoError(t, SetNamedContainerImage(
		deployment,
		"initContainers",
		moduleMigratorContainerName,
		"repo.example/threeport-acme-database-migrator:v2",
	))
	require.NoError(t, RestartDeployment(deployment, time.Date(2026, 11, 12, 15, 4, 5, 0, time.UTC)))

	assert.Equal(
		t,
		"repo.example/threeport-acme-rest-api:v2",
		containerImage(t, deployment, "containers", moduleAPIContainerName),
	)
	assert.Equal(
		t,
		"repo.example/threeport-acme-database-migrator:v2",
		containerImage(t, deployment, "initContainers", moduleMigratorContainerName),
	)
	assert.Equal(t, "cockroach:old", containerImage(t, deployment, "initContainers", "db-init"))
	annotations, _, err := unstructured.NestedStringMap(
		deployment.Object,
		"spec",
		"template",
		"metadata",
		"annotations",
	)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-12T15:04:05Z", annotations[moduleRestartAnnotation])
}

// imageRetargetDeployment returns an API deployment with the containers a retarget edits.
func imageRetargetDeployment() *unstructured.Unstructured {
	// include the migrator, the API, and the database init container
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "threeport-acme-api-server",
			"namespace": "threeport-acme",
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{},
				"spec": map[string]interface{}{
					"initContainers": []interface{}{
						map[string]interface{}{"name": "db-init", "image": "cockroach:old"},
						map[string]interface{}{"name": moduleMigratorContainerName, "image": "old-migrator"},
					},
					"containers": []interface{}{
						map[string]interface{}{"name": moduleAPIContainerName, "image": "old-api"},
					},
				},
			},
		},
	}}
}

// containerImage returns the image of one container.
func containerImage(t *testing.T, deployment *unstructured.Unstructured, listKey, name string) string {
	t.Helper()

	// read the named container
	containers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", listKey)
	require.NoError(t, err)
	require.True(t, found)
	for _, item := range containers {
		container := item.(map[string]interface{})
		if container["name"] == name {
			return container["image"].(string)
		}
	}
	t.Fatalf("container %s not found", name)
	return ""
}
