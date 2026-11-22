package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// moduleInstallPod returns the pod spec and the install container of a job.
func moduleInstallPod(t *testing.T, kebab, tag string) (map[string]interface{}, map[string]interface{}) {
	t.Helper()

	// build the job for a child in the primary-test namespace
	imageNamespace := "registry.example"
	job := moduleInstallJob("primary-test", kebab, &v0.ModuleApi{
		ImageNamespace: &imageNamespace,
		ImageTag:       &tag,
	}, "", 1)
	spec, ok := job.Object["spec"].(map[string]interface{})
	require.True(t, ok)
	template, ok := spec["template"].(map[string]interface{})
	require.True(t, ok)
	pod, ok := template["spec"].(map[string]interface{})
	require.True(t, ok)
	containers, ok := pod["containers"].([]interface{})
	require.True(t, ok)
	container, ok := containers[0].(map[string]interface{})
	require.True(t, ok)

	return pod, container
}

// TestModuleInstallJobRunsAsTheParentControllerAccount covers the job that
// installs a module on a child: it runs as the account the parent control plane
// controller already uses to create cluster objects, so the install needs no
// binding of its own.
func TestModuleInstallJobRunsAsTheParentControllerAccount(t *testing.T) {
	// build the job and read its pod
	pod, container := moduleInstallPod(t, "example", "abc")

	// assert the pod runs as the control plane controller account
	require.Equal(t, ThreeportControlPlaneControllerName, pod["serviceAccountName"])

	// assert the container starts the module binary, which the plugin image has no entrypoint for
	require.Equal(t, []interface{}{"/example"}, container["command"])
	require.Equal(t, "/my_module", modulePluginCommand("my-module"))
	require.Equal(t, []interface{}{
		"install",
		"--control-plane-image-namespace",
		"registry.example",
		"--control-plane-image-tag",
		"abc",
	}, container["args"])

	// assert the container runs as root, so the plugin can write its config directory
	securityContext, ok := container["securityContext"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, int64(0), securityContext["runAsUser"])
}

// TestModuleInstallJobNamesAreUniquePerChild covers two control planes installing
// the same module: each gets its own job name, and a long name still fits.
func TestModuleInstallJobNamesAreUniquePerChild(t *testing.T) {
	// assert two namespaces do not share a job name
	require.NotEqual(t, moduleInstallJobName("example", "primary-a"), moduleInstallJobName("example", "primary-b"))
	require.NotEqual(t, moduleInstallSecretName("example", "primary-a"), moduleInstallSecretName("example", "primary-b"))

	// assert a long module name and the default namespace stay within the secret limit
	name := moduleInstallJobName(strings.Repeat("a", 26), "threeport-control-plane")
	require.LessOrEqual(t, len(moduleInstallSecretName(strings.Repeat("a", 26), "threeport-control-plane")), moduleInstallNameLimit)
	require.LessOrEqual(t, len(name), moduleInstallNameLimit)
}

// TestModuleInstallJobHasADeadline checks the install job fails on its own when
// it never finishes, which a pod that cannot pull its image would not.
func TestModuleInstallJobHasADeadline(t *testing.T) {
	// build the job and read its deadline
	namespace := "example"
	tag := "abc"
	job := moduleInstallJob("primary-test", "example-module", &v0.ModuleApi{
		ImageNamespace: &namespace,
		ImageTag:       &tag,
	}, "", 1)
	spec, ok := job.Object["spec"].(map[string]interface{})
	require.True(t, ok)

	// assert the deadline is set to the install limit
	require.Equal(t, moduleInstallDeadlineSeconds, spec["activeDeadlineSeconds"])
}
