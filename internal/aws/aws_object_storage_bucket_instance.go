package aws

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	awsclient "github.com/nukleros/aws-builder/pkg/client"
	"github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/s3"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gorm.io/datatypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	kubejson "k8s.io/apimachinery/pkg/util/json"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const s3BucketNameConfigMapKey = "s3BucketName"

// requiredAwsObjectStorageBucketInstanceObjects holds all the required
// threeport objects needed to reconcile state for AWS object storage buckets.
type requiredAwsObjectStorageBucketInstanceObjects struct {
	AwsObjectStorageBucketDefinition v0.AwsObjectStorageBucketDefinition
	AwsAccount                       v0.AwsAccount
	WorkloadInstance                 v0.WorkloadInstance
	KubernetesRuntimeInstance        v0.KubernetesRuntimeInstance
	AwsEksKubernetesRuntimeInstance  v0.AwsEksKubernetesRuntimeInstance
}

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

	// check to make sure reconciliation is not being interrupted - if it is
	// return without error to exit reconciliation loop
	// TDOO: add alerts for interrupted reconciliation so humans can intervene
	if awsObjectStorageBucketInstance.InterruptReconciliation != nil && *awsObjectStorageBucketInstance.InterruptReconciliation {
		reconLog.Info("reconciliation interrupted")
		return 0, nil
	}

	// ensure attached object reference exists
	err := client.EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		reflect.TypeOf(*awsObjectStorageBucketInstance).String(),
		awsObjectStorageBucketInstance.ID,
		awsObjectStorageBucketInstance.WorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	// get required objects from the threeport API
	requiredObjects, err := getRequiredS3Objects(r, awsObjectStorageBucketInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get required objects for AWS object storage bucket instance reconciliation: %w", err)
	}

	// decrypt access key id and secret access key
	accessKeyID, err := encryption.Decrypt(r.EncryptionKey, *requiredObjects.AwsAccount.AccessKeyID)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt access key id: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(r.EncryptionKey, *requiredObjects.AwsAccount.SecretAccessKey)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt secret access key: %w", err)
	}

	// create AWS config
	awsConfig, err := config.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
		"",
		*requiredObjects.AwsEksKubernetesRuntimeInstance.Region,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create AWS resource client
	resourceClient := awsclient.CreateResourceClient(awsConfig)

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store inventory in database as it arrives on inventory channel
	invChan := make(chan s3.S3Inventory)
	go func() {
		for inventory := range invChan {
			inventoryJson, err := inventory.Marshal()
			if err != nil {
				reconLog.Error(err, "failed to marshal resource inventory")
			}
			dbInventory := datatypes.JSON(inventoryJson)
			objectStorageInstanceWithInventory := v0.AwsObjectStorageBucketInstance{
				Common: v0.Common{
					ID: awsObjectStorageBucketInstance.ID,
				},
				ResourceInventory: &dbInventory,
			}
			_, err = client.UpdateAwsObjectStorageBucketInstance(
				r.APIClient,
				r.APIServer,
				&objectStorageInstanceWithInventory,
			)
			if err != nil {
				reconLog.Error(err, "failed to update object storage bucket instance inventory")
			}
		}
	}()

	// create S3 client
	s3Client := s3.S3Client{
		*resourceClient,
		&invChan,
	}

	// extract kubernetes runtime resource inventory
	runtimeInventoryJson := requiredObjects.AwsEksKubernetesRuntimeInstance.ResourceInventory
	var runtimeInventory resource.ResourceInventory
	if err := resource.UnmarshalInventory([]byte(*runtimeInventoryJson), &runtimeInventory); err != nil {
		return 0, fmt.Errorf("failed to unmarshal AWS EKS kubernetes runtime inventory: %w", err)
	}

	// get workload namespace and workload service account
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		r.APIClient,
		r.APIServer,
		*requiredObjects.WorkloadInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to workload resource instances for workload using S3 bucket: %w", err)
	}
	var namespaces []string
	var serviceAccountObject unstructured.Unstructured
	var serviceAccountWri v0.WorkloadResourceInstance
	serviceAccountFound := false
	for _, wri := range *workloadResourceInstances {
		unstructuredObj := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubejson.Unmarshal(*wri.JSONDefinition, &unstructuredObj); err != nil {
			return 0, fmt.Errorf("failed to unmarshal kubernetes resource JSON to unstructured object: %w", err)
		}
		// namespace
		namespace := unstructuredObj.GetNamespace()
		if namespace != "" {
			if !util.StringSliceContains(namespaces, namespace, true) {
				namespaces = append(namespaces, namespace)
			}
		}
		// service account
		if unstructuredObj.GetKind() == "ServiceAccount" && unstructuredObj.GetName() == *requiredObjects.AwsObjectStorageBucketDefinition.WorkloadServiceAccountName {
			serviceAccountFound = true
			serviceAccountObject = *unstructuredObj
			serviceAccountWri = wri
		}
	}
	if len(namespaces) == 0 {
		return 0, errors.New("could not find any namespaces in workload resource instances")
	}
	if len(namespaces) > 1 {
		return 0, errors.New("multiple namespaces found in workload resource instances")
	}
	workloadNamespace := namespaces[0]
	if !serviceAccountFound {
		return 0, fmt.Errorf("no service account found with name %s", *requiredObjects.AwsObjectStorageBucketDefinition.WorkloadServiceAccountName)
	}

	// create S3 config
	s3Config := s3.S3Config{
		AwsAccount:           *requiredObjects.AwsAccount.AccountID,
		Region:               awsConfig.Region,
		Name:                 *awsObjectStorageBucketInstance.Name,
		VpcIdReadWriteAccess: runtimeInventory.VPCID,
		PublicReadAccess:     *requiredObjects.AwsObjectStorageBucketDefinition.PublicReadAccess,
		WorkloadReadWriteAccess: s3.WorkloadAccess{
			ServiceAccountName:      *requiredObjects.AwsObjectStorageBucketDefinition.WorkloadServiceAccountName,
			ServiceAccountNamespace: workloadNamespace,
			OidcUrl:                 runtimeInventory.Cluster.OIDCProviderURL,
		},
		Tags: provider.ThreeportProviderTags(),
	}

	// create S3 bucket
	if err := s3Client.CreateS3ResourceStack(&s3Config); err != nil {
		if deleteErr := getS3InventoryAndDelete(r, &s3Client, awsObjectStorageBucketInstance); deleteErr != nil {
			// interrupt reconciliation
			interrupt := true
			awsObjectStorageBucketInstance.InterruptReconciliation = &interrupt
			_, updateErr := client.UpdateAwsObjectStorageBucketInstance(
				r.APIClient,
				r.APIServer,
				awsObjectStorageBucketInstance,
			)
			if updateErr != nil {
				return 0, fmt.Errorf("failed to create S3 resource stack: %w and failed to delete S3 resource stack: %w and failed to update S3 storage bucket instance to interrupt reconciliation: %w", err, deleteErr, updateErr)
			}
			reconLog.Info("reconciliation interrupted after failed create and delete of resource stack")
			return 0, fmt.Errorf("failed to create S3 resource stack: %w and failed to delete S3 resource stack: %w", err, deleteErr)
		}
		reconLog.Info("created resources deleted after error")
		return 0, fmt.Errorf("failed to create S3 resource stack: %w", err)
	}

	// get the S3 bucket name and role name from S3 resource inventory
	invRetrieveAttempts := 0
	invRetrieveAttemptsMax := 6
	invRetrieveDurationSeconds := 5
	invRetrieved := false
	var s3BucketName string
	var s3RoleName string
	for invRetrieveAttempts < invRetrieveAttemptsMax {
		s3Inventory, err := getS3Inventory(r, awsObjectStorageBucketInstance)
		if err != nil {
			reconLog.Error(err, "failed to retrieve AWS relational database inventory")
		} else if s3Inventory.BucketName != "" && s3Inventory.Role.RoleName != "" {
			s3BucketName = s3Inventory.BucketName
			s3RoleName = s3Inventory.Role.RoleName
			invRetrieved = true
			break
		}
		invRetrieveAttempts += 1
		time.Sleep(time.Second * time.Duration(invRetrieveDurationSeconds))
	}
	if !invRetrieved {
		return 0, fmt.Errorf(
			"failed to retrieve S3 inventory info after %d seconds",
			invRetrieveAttemptsMax*invRetrieveDurationSeconds,
		)
	}

	// update workload resources to enable connection to S3 bucket
	if err := updateS3ClientWorkloadConnection(
		r,
		requiredObjects,
		workloadResourceInstances,
		&serviceAccountWri,
		&serviceAccountObject,
		workloadNamespace,
		s3BucketName,
		s3RoleName,
		&reconLog,
	); err != nil {
		// delete resources
		if deleteErr := getS3InventoryAndDelete(r, &s3Client, awsObjectStorageBucketInstance); deleteErr != nil {
			// interrupt reconciliation
			interrupt := true
			awsObjectStorageBucketInstance.InterruptReconciliation = &interrupt
			_, updateErr := client.UpdateAwsObjectStorageBucketInstance(
				r.APIClient,
				r.APIServer,
				awsObjectStorageBucketInstance,
			)
			if updateErr != nil {
				return 0, fmt.Errorf("failed to update workload connection: %w and failed to delete S3 resource stack: %w and failed to update S3 storage bucket instance to interrupt reconciliation: %w", err, deleteErr, updateErr)
			}
			reconLog.Info("reconciliation interrupted after failed create and delete of resource stack")
			return 0, fmt.Errorf("failed to update workload connection: %w and failed to delete S3 resource stack: %w", err, deleteErr)
		}
		reconLog.Info("created resources deleted after error")
		return 0, fmt.Errorf("failed to update workload connection: %w", err)
	}

	return 0, nil
}

// awsObjectStorageBucketInstanceUpdated reconciles state for changes to an AWS
// object storage bucket instance.
func awsObjectStorageBucketInstanceUpdated(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
	log *logr.Logger,
) (int64, error) {
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

	// check that deletion is scheduled - if not there's a problem
	if awsObjectStorageBucketInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification received but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if awsObjectStorageBucketInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// check to see if previously acknowledged - nil means it has not be
	// acknowledged
	if awsObjectStorageBucketInstance.DeletionAcknowledged != nil {
		// deletion has been acknowledged, check deletion
		deleted, err := checkS3Deleted(r, awsObjectStorageBucketInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to check if S3 bucket resource are deleted: %w", err)
		}
		if !deleted {
			// return a custom requeue of 60 seconds to re-check resources again
			return 5, nil
		}
		// resources have been deleted - confirm deletion and delete in database
		deletionReconciled := true
		deletionTimestamp := time.Now().UTC()
		deletedObjectStorageBucketInstance := v0.AwsObjectStorageBucketInstance{
			Common: v0.Common{
				ID: awsObjectStorageBucketInstance.ID,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled:        &deletionReconciled,
				DeletionConfirmed: &deletionTimestamp,
			},
		}
		_, err = client.UpdateAwsObjectStorageBucketInstance(
			r.APIClient,
			r.APIServer,
			&deletedObjectStorageBucketInstance,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to confirm deletion of AWS relational database resources in threeport API: %w", err)
		}
		_, err = client.DeleteAwsObjectStorageBucketInstance(
			r.APIClient,
			r.APIServer,
			*awsObjectStorageBucketInstance.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to delete AWS S3 bucket in threeport API: %w", err)
		}

		return 0, nil
	}

	// acknowledge deletion scheduled
	timestamp := time.Now().UTC()
	awsObjectStorageBucketInstance.DeletionAcknowledged = &timestamp
	_, err := client.UpdateAwsObjectStorageBucketInstance(
		r.APIClient,
		r.APIServer,
		awsObjectStorageBucketInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set deletion acknowledged timestamp: %w", err)
	}

	// get required objects from the threeport API
	requiredObjects, err := getRequiredS3Objects(r, awsObjectStorageBucketInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get required objects for AWS object storage bucket instance reconciliation: %w", err)
	}

	// decrypt access key id and secret access key
	accessKeyID, err := encryption.Decrypt(r.EncryptionKey, *requiredObjects.AwsAccount.AccessKeyID)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt access key id: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(r.EncryptionKey, *requiredObjects.AwsAccount.SecretAccessKey)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt secret access key: %w", err)
	}

	// create AWS config
	awsConfig, err := config.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
		"",
		*requiredObjects.AwsEksKubernetesRuntimeInstance.Region,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create S3 resource client
	resourceClient := awsclient.CreateResourceClient(awsConfig)

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store inventory in database as it arrives on inventory channel
	invChan := make(chan s3.S3Inventory)
	go func() {
		for inventory := range invChan {
			inventoryJson, err := inventory.Marshal()
			if err != nil {
				reconLog.Error(err, "failed to marshal resource inventory")
			}
			dbInventory := datatypes.JSON(inventoryJson)
			relationalDatabaseInstanceWithInventory := v0.AwsObjectStorageBucketInstance{
				Common: v0.Common{
					ID: awsObjectStorageBucketInstance.ID,
				},
				ResourceInventory: &dbInventory,
			}
			_, err = client.UpdateAwsObjectStorageBucketInstance(
				r.APIClient,
				r.APIServer,
				&relationalDatabaseInstanceWithInventory,
			)
			if err != nil {
				reconLog.Error(err, "failed to update S3 bucket inventory")
			}
		}
	}()

	// create S3 client
	s3Client := s3.S3Client{
		*resourceClient,
		&invChan,
	}

	// get S3 inventory
	s3Inventory, err := getS3Inventory(r, awsObjectStorageBucketInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve AWS relational database inventory for deletion")
	}

	// delete S3 bucket
	go func() {
		if err := deleteS3Bucket(&s3Client, s3Inventory); err != nil {
			reconLog.Error(err, "failed to delete S3 resources")
		}
	}()

	// S3 bucket deletion initiated, return custom requeue to check resources
	// in 10 seconds
	return 10, nil
}

// deleteS3Bucket deletes the AWS Resources for an S3 bucket.
func deleteS3Bucket(
	s3Client *s3.S3Client,
	s3Inventory *s3.S3Inventory,
) error {
	if err := s3Client.DeleteS3ResourceStack(s3Inventory); err != nil {
		return fmt.Errorf("failed to delete S3 resource stack: %w", err)
	}

	return nil
}

// getS3Inventory retrieves the inventory from the threeport API for an AWS
// S3 bucket.
func getS3Inventory(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
) (*s3.S3Inventory, error) {
	// retrieve latest S3 bucket from DB
	latestAwsObjectStorageBucketInstance, err := client.GetAwsObjectStorageBucketInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsObjectStorageBucketInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 bucket inventory from threeport API: %w", err)
	}

	// unmarshal inventory into S3Inventory object
	var inventory s3.S3Inventory
	if latestAwsObjectStorageBucketInstance.ResourceInventory != nil {
		if err := inventory.Unmarshal(*latestAwsObjectStorageBucketInstance.ResourceInventory); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource inventory for S3 bucket: %w", err)
		}
	}

	return &inventory, nil
}

// getS3InventoryAndDelete retrieves the latest inventory for an S3 bucket
// resource stack and deletes it.
func getS3InventoryAndDelete(
	r *controller.Reconciler,
	s3Client *s3.S3Client,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
) error {
	inventory, err := getS3Inventory(r, awsObjectStorageBucketInstance)
	if err != nil {
		return fmt.Errorf("failed to get S3 inventory to deleted it: %w", err)
	}

	if err := deleteS3Bucket(s3Client, inventory); err != nil {
		return fmt.Errorf("failed to delete S3 resources: %w", err)
	}

	return nil
}

// checkS3Deleted checks the inventory for an AWS S3 bucket
// and returns true if the final resource has been removed from the inventory.
// Otherwise it returns false, indicating there are still resources to be
// deleted.
func checkS3Deleted(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
) (bool, error) {
	inventory, err := getS3Inventory(r, awsObjectStorageBucketInstance)
	if err != nil {
		return false, fmt.Errorf("failed to get S3 bucket's AWS resource inventory: %w", err)
	}

	// the S3 bucket's IAM policy is the last thing to be deleted - if its
	// ARN is removed, the resource stack is deleted
	if inventory.PolicyArn == "" {
		return true, nil
	}

	return false, nil
}

// getRequiredS3Objectsgets the related objects from the threeport API that are
// needed for reconciling state for AWS object storage bucket instances.
func getRequiredS3Objects(
	r *controller.Reconciler,
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
) (*requiredAwsObjectStorageBucketInstanceObjects, error) {
	awsObjectStorageBucketDef, err := client.GetAwsObjectStorageBucketDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsObjectStorageBucketInstance.AwsObjectStorageBucketDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS object storage definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsObjectStorageBucketDef.AwsAccountID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS Account by ID: %w", err)
	}
	workloadInstance, err := client.GetWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsObjectStorageBucketInstance.WorkloadInstanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve associated workload for database by ID: %w", err)
	}
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime instance for workload associated with database: %w", err)
	}
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS EKS kubernetes runtime instance hosting workload associated with database: %w", err)
	}

	return &requiredAwsObjectStorageBucketInstanceObjects{
		AwsObjectStorageBucketDefinition: *awsObjectStorageBucketDef,
		AwsAccount:                       *awsAccount,
		WorkloadInstance:                 *workloadInstance,
		KubernetesRuntimeInstance:        *kubernetesRuntimeInstance,
		AwsEksKubernetesRuntimeInstance:  *awsEksKubernetesRuntimeInstance,
	}, nil
}

// updateS3ClientWorkloadConnection updates the workload resources to enable
// connection to the S3 bucket.
func updateS3ClientWorkloadConnection(
	r *controller.Reconciler,
	requiredObjects *requiredAwsObjectStorageBucketInstanceObjects,
	workloadResourceInstances *[]v0.WorkloadResourceInstance,
	serviceAccountWri *v0.WorkloadResourceInstance,
	serviceAccountObject *unstructured.Unstructured,
	workloadNamespace string,
	s3BucketName string,
	s3RoleName string,
	log *logr.Logger,
) error {
	// update workload service account resource instance to add annotation that
	// will enable permission to manage S3 bucket
	var annotations map[string]string
	annotations = serviceAccountObject.GetAnnotations()
	if annotations != nil {
		annotations["eks.amazonaws.com/role-arn"] = fmt.Sprintf(
			"arn:aws:iam::%s:role/%s",
			*requiredObjects.AwsAccount.AccountID,
			s3RoleName,
		)
	} else {
		annotations = map[string]string{
			"eks.amazonaws.com/role-arn": fmt.Sprintf(
				"arn:aws:iam::%s:role/%s",
				*requiredObjects.AwsAccount.AccountID,
				s3RoleName,
			),
		}
	}
	serviceAccountObject.SetAnnotations(annotations)
	serviceAccountJson, err := serviceAccountObject.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal JSON from updated service account object: %s", err)
	}
	serviceAccountJsonDef := datatypes.JSON(serviceAccountJson)
	serviceAccountWriReconciled := false
	serviceAccountWri.JSONDefinition = &serviceAccountJsonDef
	serviceAccountWri.Reconciled = &serviceAccountWriReconciled
	_, err = client.UpdateWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		serviceAccountWri,
	)
	if err != nil {
		return fmt.Errorf("failed to update service account workload resource instance in threeport API: %w", err)
	}

	// create a config map to provide S3 bucket name to workload
	configMapName := fmt.Sprintf(
		"%s-s3-config-%s",
		*requiredObjects.WorkloadInstance.Name,
		util.RandomAlphaNumericString(6),
	)
	workloadConfigMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: workloadNamespace,
		},
		Data: map[string]string{
			s3BucketNameConfigMapKey: s3BucketName,
		},
	}

	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, nil, nil, json.SerializerOptions{
		Yaml:   false,
		Pretty: false,
		Strict: true,
	})
	encodedConfigMap, err := runtime.Encode(serializer, workloadConfigMap)
	if err != nil {
		return fmt.Errorf("failed to encode bucket config map for workload: %w", err)
	}

	// create workload resource instance for config map
	configMapJsonDef := datatypes.JSON(encodedConfigMap)
	configMapWri := v0.WorkloadResourceInstance{
		JSONDefinition:     &configMapJsonDef,
		WorkloadInstanceID: requiredObjects.WorkloadInstance.ID,
	}
	_, err = client.CreateWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		&configMapWri,
	)
	if err != nil {
		return fmt.Errorf("failed to create workload resource instance for workload S3 bucket config map: %w", err)
	}

	// trigger reconciliation of the workload instance to update service acocunt
	// and create configmap
	workloadInstanceReconciled := false
	requiredObjects.WorkloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&requiredObjects.WorkloadInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update workload instance to trigger reconcilation of service account: %w", err)
	}

	// wait for workload to be reconciled to ensure service account is updated
	// the service account must be updated before any pods are restarted so they
	// pick up the permissions for the bucket
	workloadReconciledAttempts := 0
	workloadReconciledAttemptsMax := 12
	workloadReconciledDurationSeconds := 5
	workloadReconciled := false
	for workloadReconciledAttempts < workloadReconciledAttemptsMax {
		latestWorkloadInstance, err := client.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *requiredObjects.WorkloadInstance.ID)
		if err != nil {
			log.Error(err, "failed to get workload instance while waiting for reconciliation")
		} else if *latestWorkloadInstance.Reconciled {
			workloadReconciled = true
			break
		}
		workloadReconciledAttempts += 1
		time.Sleep(time.Second * time.Duration(workloadReconciledDurationSeconds))
	}
	if !workloadReconciled {
		return fmt.Errorf(
			"failed to confirm workload instance %s reconciled after %d seconds",
			*requiredObjects.WorkloadInstance.Name,
			workloadReconciledAttemptsMax*workloadReconciledDurationSeconds,
		)
	}

	// update the workload resource instances for Pods or pod abstraction kinds
	// (Deployments, StatefulSets etc.) to apply:
	// 1) delete if a Pod kind
	// 2) update to apply the env variable
	// 3) update the workload resource instance and mark unreconciled so the
	// workload gets updated
	for _, wri := range *workloadResourceInstances {
		unstructuredObj := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubejson.Unmarshal(*wri.JSONDefinition, &unstructuredObj); err != nil {
			return fmt.Errorf("failed to unmarshal kubernetes resource JSON to unstructured object: %w", err)
		}

		// check for pod abstraction kinds like Deployment, StatefulSet etc.
		podAbstractionKind := false
		for _, kind := range kube.GetPodAbstractionKinds() {
			if unstructuredObj.GetKind() == kind {
				podAbstractionKind = true
			}
		}

		// if a Pod resource we need to delete it (will be re-created)
		if unstructuredObj.GetKind() == "Pod" {
			// delete the pod
			kubeClient, mapper, err := kube.GetClient(
				&requiredObjects.KubernetesRuntimeInstance,
				true,
				r.APIClient,
				r.APIServer,
				r.EncryptionKey,
			)
			if err != nil {
				return fmt.Errorf(
					"failed to get Kubernetes client and mapper to delete pod %s: %w", unstructuredObj.GetName(), err,
				)
			}
			if err := kube.DeleteResource(
				unstructuredObj,
				kubeClient,
				*mapper,
			); err != nil {
				return fmt.Errorf("failed to delete pod %s: %w", unstructuredObj.GetName(), err)
			}
		}

		// update the workload resource instance
		if unstructuredObj.GetKind() == "Pod" || podAbstractionKind {
			// update the pod object to set env var
			unstructuredObj, err := setPodS3EnvConfig(
				unstructuredObj,
				*requiredObjects.AwsObjectStorageBucketDefinition.WorkloadBucketEnvVar,
				configMapName,
				s3BucketNameConfigMapKey,
			)
			// update the pod's WRI with new resource config and mark
			// unreconciled
			wriJson, err := unstructuredObj.MarshalJSON()
			if err != nil {
				return fmt.Errorf("failed to marshal JSON from updated workload resource instance object: %s", err)
			}
			wriJsonDef := datatypes.JSON(wriJson)
			wri.JSONDefinition = &wriJsonDef
			wriReconciled := false
			wri.Reconciled = &wriReconciled
			_, err = client.UpdateWorkloadResourceInstance(
				r.APIClient,
				r.APIServer,
				&wri,
			)
			if err != nil {
				return fmt.Errorf("failed to update pod resource that required restart and reconciliation: %w", err)
			}
		}
	}

	// mark workload instance as unreconciled to trigger reconciliation once
	// more to update the pods to mount the config map and assume the new
	// service account that now has access to the S3 bucket
	workloadInstanceReconciled = false
	requiredObjects.WorkloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(
		r.APIClient,
		r.APIServer,
		&requiredObjects.WorkloadInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update workload instance to trigger reconcilation of workload resource instances: %w", err)
	}

	return nil
}

// setPodS3EnvConfig updates a Job, Replicaset, Deployment, StatefulSet or
// DaemonSet resources to set an environment variable mounted from a ConfigMap
// on all containers.
func setPodS3EnvConfig(
	unstructuredObj *unstructured.Unstructured,
	envVar string,
	configMapName string,
	configMapKey string,
) (*unstructured.Unstructured, error) {
	// create environment entry to add
	s3EnvConfig := map[string]interface{}{
		"name": envVar,
		"valueFrom": map[string]interface{}{
			"configMapKeyRef": map[string]interface{}{
				"name": configMapName,
				"key":  configMapKey,
			},
		},
	}

	// get containers
	var containers []interface{}
	if unstructuredObj.GetKind() == "Pod" {
		ctrs, found, err := unstructured.NestedSlice(
			unstructuredObj.Object,
			"spec",
			"containers",
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to find valid container slice in %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}
		if !found {
			return nil, fmt.Errorf(
				"no containers found in %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}
		containers = ctrs
	} else {
		ctrs, found, err := unstructured.NestedSlice(
			unstructuredObj.Object,
			"spec",
			"template",
			"spec",
			"containers",
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to find valid container slice in %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}
		if !found {
			return nil, fmt.Errorf(
				"no containers found in %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}
		containers = ctrs
	}

	// iterate over containers and update env
	for i, container := range containers {
		ctr, ok := container.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf(
				"failed to cast container into map[string]interface{} for %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}

		existingEnv, _, err := unstructured.NestedSlice(ctr, "env")
		if err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve env config from container for %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}
		existingEnv = append(existingEnv, s3EnvConfig)

		containerMap, ok := container.(map[string]interface{})
		containerMap["env"] = existingEnv
		containers[i] = containerMap
	}

	// replaced existing container slice with updated version
	if unstructuredObj.GetKind() == "Pod" {
		if err := unstructured.SetNestedSlice(
			unstructuredObj.Object,
			containers,
			"spec",
			"containers",
		); err != nil {
			return nil, fmt.Errorf(
				"failed to set updated containers for pod resource %s", unstructuredObj.GetName(),
			)
		}
	} else {
		if err := unstructured.SetNestedSlice(
			unstructuredObj.Object,
			containers,
			"spec",
			"template",
			"spec",
			"containers",
		); err != nil {
			return nil, fmt.Errorf(
				"failed to set updated containers for %s resource %s", unstructuredObj.GetKind(), unstructuredObj.GetName(),
			)
		}
	}

	return unstructuredObj, nil
}
