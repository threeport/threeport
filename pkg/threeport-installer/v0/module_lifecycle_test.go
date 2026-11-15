package v0

import (
	"testing"

	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestModuleInstallJobUsesTheWorkloadControllerAccount checks the install
// job runs as the account that can create cluster objects.
func TestModuleInstallJobUsesTheWorkloadControllerAccount(t *testing.T) {
	// build the job and read its pod account
	namespace := "example"
	tag := "abc"
	job := moduleInstallJob(namespace, "example-module", &v0.ModuleApi{
		ImageNamespace: &namespace,
		ImageTag:       &tag,
	})
	spec, ok := job.Object["spec"].(map[string]interface{})
	require.True(t, ok)
	template, ok := spec["template"].(map[string]interface{})
	require.True(t, ok)
	pod, ok := template["spec"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, ThreeportKubernetesWorkloadControllerName, pod["serviceAccountName"])
}
