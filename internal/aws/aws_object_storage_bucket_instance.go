package aws

import (
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// awsObjectStorageBucketInstanceCreated reconciles state for an AWS object
// storage bucket instance that has been created.
func awsObjectStorageBucketInstanceCreated(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsObjectStorageBucketInstanceID", *awsObjectStorageBucketInstance.ID,
		"awsObjectStorageBucketInstanceName", *awsObjectStorageBucketInstance.Name,
	)

	return 0, nil
}

// awsObjectStorageBucketInstanceUpdated reconciles state for changes to an AWS
// object storage bucket instance.
func awsObjectStorageBucketInstanceUpdated(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsObjectStorageBucketInstanceID", *awsObjectStorageBucketInstance.ID,
		"awsObjectStorageBucketInstanceName", *awsObjectStorageBucketInstance.Name,
	)

	return 0, nil
}

// awsObjectStorageBucketInstanceDeleted reconciles state when an AWS object
// storage bucket instance is deleted.
func awsObjectStorageBucketInstanceDeleted(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsObjectStorageBucketInstanceID", *awsObjectStorageBucketInstance.ID,
		"awsObjectStorageBucketInstanceName", *awsObjectStorageBucketInstance.Name,
	)

	return 0, nil
}
