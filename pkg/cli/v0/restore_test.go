package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestRestoreUsesTheRecordedBucket covers where a restore finds its
// backups: the bucket recorded at install, unless another is given, and a
// clear error when there is none to use.
func TestRestoreUsesTheRecordedBucket(t *testing.T) {
	recorded := &ControlPlane{
		Name:              "prod",
		Provider:          v0.KubernetesRuntimeInfraProviderGKE,
		GKEProviderConfig: GKEProviderConfig{BackupBucket: "threeport-prod-backups-acme-prod"},
	}

	bucket, err := RestoreBackupBucket(recorded, "")
	require.NoError(t, err)
	assert.Equal(t, "threeport-prod-backups-acme-prod", bucket)

	bucket, err = RestoreBackupBucket(recorded, "copied-backups")
	require.NoError(t, err)
	assert.Equal(t, "copied-backups", bucket, "a given bucket wins over the recorded one")

	_, err = RestoreBackupBucket(&ControlPlane{Name: "dev", Provider: v0.KubernetesRuntimeInfraProviderGKE}, "")
	assert.ErrorIs(t, err, ErrNoBackupBucket)
	assert.ErrorContains(t, err, "--bucket")

	_, err = RestoreBackupBucket(&ControlPlane{Name: "eks", Provider: v0.KubernetesRuntimeInfraProviderEKS}, "")
	assert.ErrorIs(t, err, ErrNoBackupBucket)
}

// TestRestoreIsOnlyForGke covers the providers a restore reaches the cluster
// on: GKE only, refused before anything is touched elsewhere.
func TestRestoreIsOnlyForGke(t *testing.T) {
	err := RestoreControlPlaneDatabase(&ControlPlane{Name: "eks", Provider: v0.KubernetesRuntimeInfraProviderEKS}, "some-bucket", "LATEST")
	assert.ErrorContains(t, err, "supported for control planes on gke")
}
