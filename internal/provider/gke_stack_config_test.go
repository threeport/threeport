package provider

import (
	"os"
	"path/filepath"
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
	// source unless a test writes one
	home := t.TempDir()
	t.Setenv("HOME", home)
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
		gcloudConfig := filepath.Join(home, ".config", "gcloud", "configurations")
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
