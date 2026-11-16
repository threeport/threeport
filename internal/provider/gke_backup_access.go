package provider

import (
	"context"
	"fmt"

	gcpstorage "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"google.golang.org/api/storage/v1"
)

// controllerBucketRole lets the Threeport service account manage access to the
// backup bucket, so the control plane controller can grant a new database
// access to it. It includes no object read.
const controllerBucketRole = "roles/storage.legacyBucketOwner"

// backupBucketName returns the name of the cluster's backup bucket.
func (i *KubernetesRuntimeInfraGKE) backupBucketName() string {
	// use the bucket this run provisioned, else the name an install gives it
	if i.BackupBucket != "" {
		return i.BackupBucket
	}
	return GkeBackupBucketName(i.RuntimeInstanceName, i.ProjectID)
}

// controllerBucketAccess grants the Threeport service account its role on the
// backup bucket. The bucket exists before the program runs: an install
// provisions it first, and an upgrade checks for it.
func (i *KubernetesRuntimeInfraGKE) controllerBucketAccess(
	ctx *pulumi.Context,
	gcpProvider pulumi.ProviderResource,
) error {
	_, err := gcpstorage.NewBucketIAMMember(
		ctx,
		fmt.Sprintf("%s-backup-bucket-controller", i.RuntimeInstanceName),
		&gcpstorage.BucketIAMMemberArgs{
			Bucket: pulumi.String(i.backupBucketName()),
			Role:   pulumi.String(controllerBucketRole),
			Member: pulumi.Sprintf("serviceAccount:%s", i.getServiceAccountEmail()),
		},
		pulumi.Provider(gcpProvider),
	)
	if err != nil {
		return fmt.Errorf("failed to grant the controller access to the backup bucket: %w", err)
	}

	return nil
}

// requireBackupBucket refuses an upgrade whose backup bucket does not exist,
// since the grant on it would fail when the stack is applied.
func (i *KubernetesRuntimeInfraGKE) requireBackupBucket() error {
	storageClientOpts := i.backupClientOpts(storage.DevstorageReadOnlyScope)
	storageService, err := storage.NewService(context.Background(), storageClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create storage service client: %w", err)
	}

	bucketName := i.backupBucketName()
	if _, err := storageService.Buckets.Get(bucketName).Do(); err != nil {
		return fmt.Errorf("failed to find backup bucket %s: %w", bucketName, err)
	}

	return nil
}
