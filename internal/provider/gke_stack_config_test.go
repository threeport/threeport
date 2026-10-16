package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadGCPConfigRecordsTheResolvedLocation covers the stack config when the
// caller names neither the project nor the region.
//
// Those are resolved from the environment or the gcloud configuration after the
// infra is constructed, so a stack config derived at construction records
// nothing while the cluster is built somewhere definite. A delete reads the
// stack back to find the cluster: given empty values it falls through to
// whatever the environment says then, which may have moved or be gone, and the
// cluster is left running.
func TestLoadGCPConfigRecordsTheResolvedLocation(t *testing.T) {
	// a home with no gcloud configuration, so the environment is the only
	// source unless a test writes one. Both variables are set because
	// os.UserHomeDir reads a different one per platform, and leaving the
	// other would send the loader to the developer's real configuration.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, unrelated := range []string{
		"GOOGLE_CLOUD_PROJECT",
		"CLOUDSDK_CORE_PROJECT",
		"GCLOUD_PROJECT",
		"CLOUDSDK_COMPUTE_REGION",
		"GOOGLE_REGION",
	} {
		t.Setenv(unrelated, "")
	}

	t.Run("resolved from the environment", func(t *testing.T) {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "project-from-the-environment")
		t.Setenv("CLOUDSDK_COMPUTE_REGION", "us-west1")

		infra := (&KubernetesRuntimeInfraGKE{}).InitPulumiProject()
		require.NoError(t, infra.loadGCPConfig())

		assert.Equal(t, "project-from-the-environment", infra.StackConfigs["gcp:project"])
		assert.Equal(t, "us-west1", infra.StackConfigs["gcp:region"])
	})

	t.Run("resolved from the gcloud configuration", func(t *testing.T) {
		// asking the loader where it reads from, rather than naming the path
		// again here, so a fixture cannot drift to somewhere nothing reads
		configDir, err := gcloudConfigDir()
		require.NoError(t, err)
		// however deep that is on this platform, it has to be under the
		// temporary home, or the fixture is not what gets read
		require.True(
			t, strings.HasPrefix(configDir, home),
			"the loader reads %s, outside the test's home %s", configDir, home,
		)

		gcloudConfig := filepath.Join(configDir, "configurations")
		require.NoError(t, os.MkdirAll(gcloudConfig, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(gcloudConfig, "config_default"),
			[]byte("[core]\nproject = project-from-gcloud\n[compute]\nregion = europe-west2\n"),
			0600,
		))

		infra := (&KubernetesRuntimeInfraGKE{}).InitPulumiProject()
		require.NoError(t, infra.loadGCPConfig())

		assert.Equal(t, "project-from-gcloud", infra.StackConfigs["gcp:project"])
		assert.Equal(t, "europe-west2", infra.StackConfigs["gcp:region"])
	})
}
