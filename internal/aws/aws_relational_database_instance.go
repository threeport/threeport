package aws

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	awsclient "github.com/nukleros/aws-builder/pkg/client"
	"github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/rds"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gorm.io/datatypes"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	"github.com/threeport/threeport/internal/aws/mapping"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

const dbConnectionSecretName = "wordpress-db-connection"

func awsRelationalDatabaseInstanceCreated(
	r *controller.Reconciler,
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsRelationalDatabaseInstanceID", *awsRelationalDatabaseInstance.ID,
		"awsRelationalDatabaseInstanceName", *awsRelationalDatabaseInstance.Name,
	)

	// get required objects from the threeport API
	awsRelationalDatabaseDef, awsAccount, workloadInstance, awsEksKubernetesRuntimeInstance, err := getRequiredRdsObjects(r, awsRelationalDatabaseInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get required objects for AWS relational database instance creation reconciliation: %w", err)
	}

	// decrypt access key id and secret access key
	accessKeyID, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.AccessKeyID)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt access key id: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.SecretAccessKey)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt secret access key: %w", err)
	}

	// create AWS config
	awsConfig, err := config.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
		"",
		*awsEksKubernetesRuntimeInstance.Region,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create RDS resource client
	resourceClient := awsclient.CreateResourceClient(awsConfig)

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store inventory in database as it arrives on inventory channel
	invChan := make(chan rds.RdsInventory)
	go func() {
		for inventory := range invChan {
			inventoryJson, err := inventory.Marshal()
			if err != nil {
				reconLog.Error(err, "failed to marshal resource inventory")
			}
			dbInventory := datatypes.JSON(inventoryJson)
			relationalDatabaseInstanceWithInventory := v0.AwsRelationalDatabaseInstance{
				Common: v0.Common{
					ID: awsRelationalDatabaseInstance.ID,
				},
				ResourceInventory: &dbInventory,
			}
			_, err = client.UpdateAwsRelationalDatabaseInstance(
				r.APIClient,
				r.APIServer,
				&relationalDatabaseInstanceWithInventory,
			)
			if err != nil {
				reconLog.Error(err, "failed to update relational database instance inventory")
			}
		}
	}()

	// create RDS client
	rdsClient := rds.RdsClient{
		*resourceClient,
		&invChan,
	}

	// get machine class
	machineClass, err := mapping.GetProviderMachineClassForMachineSize("aws", *awsRelationalDatabaseDef.MachineSize)
	if err != nil {
		return 0, fmt.Errorf("failed to map machine size to provider machine class for database: %w", err)
	}

	// extract kubernetes runtime resource inventory
	runtimeInventoryJson := awsEksKubernetesRuntimeInstance.ResourceInventory
	var runtimeInventory resource.ResourceInventory
	if err := resource.UnmarshalInventory([]byte(*runtimeInventoryJson), &runtimeInventory); err != nil {
		return 0, fmt.Errorf("failed to unmarshal AWS EKS kubernetes runtime inventory: %w", err)
	}

	// create RDS config
	dbPort := int32(*awsRelationalDatabaseDef.DatabasePort)
	storageGb := int32(*awsRelationalDatabaseDef.StorageGb)
	backupDays := int32(*awsRelationalDatabaseDef.BackupDays)
	dbUser := util.RandomString(12)
	dbPassword := util.RandomString(32)
	rdsConfig := rds.RdsConfig{
		AwsAccount:            *awsAccount.AccountID,
		Region:                awsConfig.Region,
		VpcId:                 runtimeInventory.VPCID,
		SubnetIds:             runtimeInventory.SubnetIDs,
		SourceSecurityGroupId: runtimeInventory.SecurityGroupID,
		Name:                  *awsRelationalDatabaseInstance.Name,
		DbName:                *awsRelationalDatabaseDef.DatabaseName,
		Class:                 machineClass,
		Engine:                *awsRelationalDatabaseDef.Engine,
		EngineVersion:         *awsRelationalDatabaseDef.EngineVersion,
		DbPort:                dbPort,
		StorageGb:             storageGb,
		BackupDays:            backupDays,
		DbUser:                dbUser,
		DbUserPassword:        dbPassword,
	}

	if err := rdsClient.CreateRdsResourceStack(&rdsConfig); err != nil {
		return 0, fmt.Errorf("failed to create RDS resource stack: %w", err)
	}

	// ensure attached object reference exists
	err = client.EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		//reflect.TypeOf(*gatewayInstance).String(),
		reflect.TypeOf(*awsRelationalDatabaseInstance).String(),
		//gatewayInstance.ID,
		awsRelationalDatabaseInstance.ID,
		//gatewayInstance.WorkloadInstanceID,
		awsRelationalDatabaseInstance.WorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	// get workload namespace
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to workload resource instances for workload using RDS instance: %w", err)
	}
	var namespaces []string
	for _, wri := range *workloadResourceInstances {
		namespace, err := kube.GetNamespaceFromJSON(*wri.JSONDefinition)
		if err != nil {
			return 0, fmt.Errorf("failed to get namespace from workload resource instance JSON definition: %w", err)
		}
		if namespace != "" {
			if !util.StringSliceContains(namespaces, namespace, true) {
				namespaces = append(namespaces, namespace)
			}
		}
	}
	if len(namespaces) == 0 {
		return 0, errors.New("could not find any namespaces in workload resource instances")
	}
	if len(namespaces) > 1 {
		return 0, errors.New("multiple namespaces found in workload resource instances")
	}
	workloadNamespace := namespaces[0]

	// extract RDS inventory from database
	latestAwsRelationalDatabaseInstance, err := client.GetAwsRelationalDatabaseInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsRelationalDatabaseInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve relational database instance with inventory: %w", err)
	}
	var rdsInventory rds.RdsInventory
	if err := rdsInventory.Unmarshal(*latestAwsRelationalDatabaseInstance.ResourceInventory); err != nil {
		return 0, fmt.Errorf("failed to unmarshal RDS inventory: %w", err)
	}

	// create DB connection secret for workload
	data := map[string][]byte{
		"db-endpoint": []byte(rdsInventory.RdsInstanceEndpoint),
		"db-port":     []byte(strconv.Itoa(*awsRelationalDatabaseDef.DatabasePort)),
		"db-name":     []byte(*awsRelationalDatabaseDef.DatabaseName),
		"db-user":     []byte(dbUser),
		"db-password": []byte(dbPassword),
	}
	dbConnSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbConnectionSecretName,
			Namespace: workloadNamespace,
		},
		Data: data,
		Type: v1.SecretTypeOpaque,
	}
	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, nil, nil, json.SerializerOptions{
		Yaml:   false,
		Pretty: false,
		Strict: true,
	})
	encodedSecret, err := runtime.Encode(serializer, dbConnSecret)
	if err != nil {
		return 0, fmt.Errorf("failed to encode DB connection secret for workload: %w", err)
	}

	// create workload resource instance
	jsonDef := datatypes.JSON(encodedSecret)
	workloadResourceInstance := v0.WorkloadResourceInstance{
		JSONDefinition:     &jsonDef,
		WorkloadInstanceID: workloadInstance.ID,
	}
	_, err = client.CreateWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		&workloadResourceInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create workload resource instance for database connection secret: %w", err)
	}

	// trigger reconciliation of the workload instance
	workloadInstanceReconciled := false
	workloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(
		r.APIClient,
		r.APIServer,
		workloadInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update workload instance to trigger reconcilation: %w", err)
	}

	return 0, nil
}

func awsRelationalDatabaseInstanceUpdated(
	r *controller.Reconciler,
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

func awsRelationalDatabaseInstanceDeleted(
	r *controller.Reconciler,
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsRelationalDatabaseInstanceID", *awsRelationalDatabaseInstance.ID,
		"awsRelationalDatabaseInstanceName", *awsRelationalDatabaseInstance.Name,
	)

	// check that deletion is scheduled - if not there's a problem
	if awsRelationalDatabaseInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification received but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if awsRelationalDatabaseInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// check to see if previously acknowledged - nil means it has not be
	// acknowledged
	if awsRelationalDatabaseInstance.DeletionAcknowledged != nil {
		// deletion has been acknowledged, check deletion
		deleted, err := checkRdsDeleted(r, awsRelationalDatabaseInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to check if RDS instance resource are deleted: %w", err)
		}
		if !deleted {
			// return a custom requeue of 60 seconds to re-check resources again
			return 60, nil
		}
		// resources have been deleted - confirm deletion and delete in database
		deletionReconciled := true
		deletionTimestamp := time.Now().UTC()
		deletedRelationalDatabaseInstance := v0.AwsRelationalDatabaseInstance{
			Common: v0.Common{
				ID: awsRelationalDatabaseInstance.ID,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled:        &deletionReconciled,
				DeletionConfirmed: &deletionTimestamp,
			},
		}
		_, err = client.UpdateAwsRelationalDatabaseInstance(
			r.APIClient,
			r.APIServer,
			&deletedRelationalDatabaseInstance,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to confirm deletion of AWS relational database resources in threeport API: %w", err)
		}
		_, err = client.DeleteAwsRelationalDatabaseInstance(
			r.APIClient,
			r.APIServer,
			*awsRelationalDatabaseInstance.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to delete AWS relational database instance in threeport API: %w", err)
		}

		return 0, nil
	}

	// acknowledge deletion scheduled
	timestamp := time.Now().UTC()
	awsRelationalDatabaseInstance.DeletionAcknowledged = &timestamp
	_, err := client.UpdateAwsRelationalDatabaseInstance(
		r.APIClient,
		r.APIServer,
		awsRelationalDatabaseInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set deletion acknowledged timestamp: %w", err)
	}

	// get required objects from the threeport API
	_, awsAccount, _, awsEksKubernetesRuntimeInstance, err := getRequiredRdsObjects(r, awsRelationalDatabaseInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get required objects for AWS relational database instance creation reconciliation: %w", err)
	}

	// decrypt access key id and secret access key
	accessKeyID, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.AccessKeyID)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt access key id: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.SecretAccessKey)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt secret access key: %w", err)
	}

	// create AWS config
	awsConfig, err := config.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
		"",
		*awsEksKubernetesRuntimeInstance.Region,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create RDS resource client
	resourceClient := awsclient.CreateResourceClient(awsConfig)

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store inventory in database as it arrives on inventory channel
	invChan := make(chan rds.RdsInventory)
	go func() {
		for inventory := range invChan {
			inventoryJson, err := inventory.Marshal()
			if err != nil {
				reconLog.Error(err, "failed to marshal resource inventory")
			}
			dbInventory := datatypes.JSON(inventoryJson)
			relationalDatabaseInstanceWithInventory := v0.AwsRelationalDatabaseInstance{
				Common: v0.Common{
					ID: awsRelationalDatabaseInstance.ID,
				},
				ResourceInventory: &dbInventory,
			}
			_, err = client.UpdateAwsRelationalDatabaseInstance(
				r.APIClient,
				r.APIServer,
				&relationalDatabaseInstanceWithInventory,
			)
			if err != nil {
				reconLog.Error(err, "failed to update relational database instance inventory")
			}
		}
	}()

	// create RDS client
	rdsClient := rds.RdsClient{
		*resourceClient,
		&invChan,
	}

	// get RDS inventory
	var rdsInventory rds.RdsInventory
	if err := rdsInventory.Unmarshal(*awsRelationalDatabaseInstance.ResourceInventory); err != nil {
		return 0, fmt.Errorf("failed to unmarshal RDS inventory: %w", err)
	}

	// delete RDS instance
	go deleteRdsInstance(&rdsClient, &rdsInventory, &reconLog)

	// RDS instance deletion initiated, return custom requeue to check resources
	// in 3 min
	return 180, nil
}

func deleteRdsInstance(
	rdsClient *rds.RdsClient,
	rdsInventory *rds.RdsInventory,
	log *logr.Logger,
) {
	if err := rdsClient.DeleteRdsResourceStack(rdsInventory); err != nil {
		log.Error(err, "failed to delete RDS resource stack")
	}
}

func getRdsInventory(
	r *controller.Reconciler,
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
) (*rds.RdsInventory, error) {
	// retrieve latest relational database instance from DB
	latestAwsRelationalDatabaseInstance, err := client.GetAwsRelationalDatabaseInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsRelationalDatabaseInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get RDS instance inventory from threeport API: %w", err)
	}

	// unmarshal inventory into RdsInventory object
	var inventory rds.RdsInventory
	if latestAwsRelationalDatabaseInstance.ResourceInventory != nil {
		if err := inventory.Unmarshal(*latestAwsRelationalDatabaseInstance.ResourceInventory); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource inventory for RDS instance: %w", err)
		}
	}

	return &inventory, nil
}

func checkRdsDeleted(
	r *controller.Reconciler,
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
) (bool, error) {
	inventory, err := getRdsInventory(r, awsRelationalDatabaseInstance)
	if err != nil {
		return false, fmt.Errorf("failed to get RDS instance's AWS resource inventory: %w", err)
	}

	// the RDS instance security group is the last thing to be deleted - if its
	// ID is removed, the resource stack is deleted
	if inventory.SecurityGroupId == "" {
		return true, nil
	}

	return false, nil
}

func getRequiredRdsObjects(
	r *controller.Reconciler,
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
) (
	*v0.AwsRelationalDatabaseDefinition,
	*v0.AwsAccount,
	*v0.WorkloadInstance,
	*v0.AwsEksKubernetesRuntimeInstance,
	error,
) {
	awsRelationalDatabaseDef, err := client.GetAwsRelationalDatabaseDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsRelationalDatabaseInstance.AwsRelationalDatabaseDefinitionID,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to retrieve AWS relational database definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsRelationalDatabaseDef.AwsAccountID,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}
	workloadInstance, err := client.GetWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsRelationalDatabaseInstance.WorkloadInstanceID,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to retrieve associated workload for database by ID: %w", err)
	}
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get kubernetes runtime instance for workload associated with database: %w", err)
	}
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.ID,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get AWS EKS kubernetes runtime instance hosting workload associated with database: %w", err)
	}

	return awsRelationalDatabaseDef, awsAccount, workloadInstance, awsEksKubernetesRuntimeInstance, nil
}
