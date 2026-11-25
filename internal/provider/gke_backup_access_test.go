package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gkeBucketIAMMemberToken = "gcp:storage/bucketIAMMember:BucketIAMMember"

// TestGkeControllerManagesTheBackupBucketAccess covers the grant on the backup
// bucket for the Threeport service account: made when the cluster has backups,
// naming the bucket, the role, and the account.
func TestGkeControllerManagesTheBackupBucketAccess(t *testing.T) {
	// run the program for a cluster with backups
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:  PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		ProvisionBackups: true,
	}
	mocks := runGkeProgram(t, i)

	// assert the grant names the bucket, the role, and the Threeport service account
	grants := mocks.ofType(gkeBucketIAMMemberToken)
	require.Len(t, grants, 1)
	assert.Equal(t, GkeBackupBucketName("threeport-test", "test-project"), grants[0].inputs["bucket"])
	assert.Equal(t, "roles/storage.legacyBucketOwner", grants[0].inputs["role"])
	assert.Equal(t, "serviceAccount:"+i.getServiceAccountEmail(), grants[0].inputs["member"])
}

// TestGkeMakesNoBackupBucketGrantWithoutBackups covers a cluster with no backup
// bucket, which has nothing to grant access to.
func TestGkeMakesNoBackupBucketGrantWithoutBackups(t *testing.T) {
	// run the program for a cluster without backups
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
	}
	mocks := runGkeProgram(t, i)

	// assert no grant is made
	assert.Empty(t, mocks.ofType(gkeBucketIAMMemberToken))
}
