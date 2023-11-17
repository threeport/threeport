package aws

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	aws_client "github.com/nukleros/aws-builder/pkg/client"
	"github.com/nukleros/aws-builder/pkg/eks"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

const staleAckDurationSeconds = 240

// awsEksKubernetesRuntimeInstanceCreated reconciles state for created AWS EKS
// runtimes by creating a new EKS cluster.
func awsEksKubernetesRuntimeInstanceCreated(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsEksKubernetesRuntimeInstanceID", *awsEksKubernetesRuntimeInstance.ID,
		"awsEksKubernetesRuntimeInstanceName", *awsEksKubernetesRuntimeInstance.Name,
	)

	// call the API to ensure we have the most up-to-date version of the EKS
	// runtime instance
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest version of AWS EKS kubernetes runtime instance: %w", err)
	}

	// check to see if reconciled - it should not be, but if so we should do
	// nothing more
	if awsEksKubernetesRuntimeInstance.CreationConfirmed != nil {
		return 0, nil
	}

	// check to see if previously acknowledged - not nil means it has been
	// previously acknowledged
	if awsEksKubernetesRuntimeInstance.CreationAcknowledged != nil && !*awsEksKubernetesRuntimeInstance.CreationFailed {
		// creation acknowledged - check to see if creation is complete
		creationComplete, err := checkEksCreated(r, awsEksKubernetesRuntimeInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to check if EKS cluster infra resources have been created: %w", err)
		}
		if creationComplete {
			// get cluster definition and aws account info
			awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
				r.APIClient,
				r.APIServer,
				*awsEksKubernetesRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to retreive cluster definition by ID: %w", err)
			}
			awsAccount, err := client.GetAwsAccountByID(
				r.APIClient,
				r.APIServer,
				*awsEksKubernetesRuntimeDefinition.AwsAccountID,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
			}

			awsConfig, err := kube.GetAwsConfigFromAwsAccount(
				r.EncryptionKey,
				*awsEksKubernetesRuntimeInstance.Region,
				awsAccount,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to create AWS config: %w", err)
			}

			// get kubernetes cluster connection info
			clusterInfra := provider.KubernetesRuntimeInfraEKS{
				RuntimeInstanceName: *awsEksKubernetesRuntimeInstance.Name,
				AwsConfig:           awsConfig,
			}
			kubeConnectionInfo, err := clusterInfra.GetConnection()
			if err != nil {
				return 0, fmt.Errorf("failed to Kubernetes API connection info: %w", err)
			}

			// get kubernetes runtime instance to update kube connection info
			kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
				r.APIClient,
				r.APIServer,
				*awsEksKubernetesRuntimeInstance.KubernetesRuntimeInstanceID,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to get kubernetes runtime instance to update kube connection info: %w", err)
			}

			// update kube connection info
			kubeRuntimeReconciled := false
			kubernetesRuntimeInstance.APIEndpoint = &kubeConnectionInfo.APIEndpoint
			kubernetesRuntimeInstance.CACertificate = &kubeConnectionInfo.CACertificate
			kubernetesRuntimeInstance.ConnectionToken = &kubeConnectionInfo.EKSToken
			kubernetesRuntimeInstance.ConnectionTokenExpiration = &kubeConnectionInfo.EKSTokenExpiration
			kubernetesRuntimeInstance.Reconciled = &kubeRuntimeReconciled
			_, err = client.UpdateKubernetesRuntimeInstance(
				r.APIClient,
				r.APIServer,
				kubernetesRuntimeInstance,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to update kubernetes runtime instance with kube connection info: %w", err)
			}

			// confirm creation and set reconciled to true
			creationReconciled := true
			creationTimestamp := time.Now().UTC()
			createdAwsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
				Common: v0.Common{
					ID: awsEksKubernetesRuntimeInstance.ID,
				},
				Reconciliation: v0.Reconciliation{
					Reconciled:        &creationReconciled,
					CreationConfirmed: &creationTimestamp,
				},
			}
			_, err = client.UpdateAwsEksKubernetesRuntimeInstance(
				r.APIClient,
				r.APIServer,
				&createdAwsEksKubernetesRuntimeInstance,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to confirm creation of EKS cluster infra resources: %w", err)
			}

			return 0, nil
		}

		// check duration since last acknowledged
		stale := checkStaleEksAck(*awsEksKubernetesRuntimeInstance.CreationAcknowledged)
		if !stale {
			return 90, nil
		}
	}

	// one of the following is true at this point:
	// 1. creation has not been acknowledged - new create request
	// 2. creation has previously failed - time to retry
	// 3. the last acknowledgement is stale - creation was interrupted
	// in each case we will attempt to create/resume creation

	// acknowledge creation and set creation failure to false
	creationAckTimestamp := time.Now().UTC()
	creationFailed := false
	awsEksKubernetesRuntimeInstance.CreationAcknowledged = &creationAckTimestamp
	awsEksKubernetesRuntimeInstance.CreationFailed = &creationFailed
	_, err = client.UpdateAwsEksKubernetesRuntimeInstance(
		r.APIClient,
		r.APIServer,
		awsEksKubernetesRuntimeInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set creation acknowledged timestamp: %w", err)
	}

	// get cluster definition and aws account info
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retreive cluster definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeDefinition.AwsAccountID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	// add log metadata
	reconLog = log.WithValues(
		"awsEksClusterDefinitionRegion", *awsEksKubernetesRuntimeInstance.Region,
		"awsEksClusterDefinitionZoneCount", *awsEksKubernetesRuntimeDefinition.ZoneCount,
		"awsEksClusterDefinitionDefaultNodeGroupInstanceType", *awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInstanceType,
	)

	awsConfig, err := kube.GetAwsConfigFromAwsAccount(r.EncryptionKey, *awsEksKubernetesRuntimeInstance.Region, awsAccount)
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS config: %w", err)
	}

	// create resource client
	inventoryChan := make(chan eks.EksInventory)
	eksClient := eks.EksClient{
		*aws_client.CreateResourceClient(awsConfig),
		&inventoryChan,
	}

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *eksClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store inventory in database as it arrives on inventory channel
	go func() {
		for inventory := range inventoryChan {
			inventoryJson, err := inventory.Marshal()
			if err != nil {
				reconLog.Error(err, "failed to marshal inventory")
			}
			dbInventory := datatypes.JSON(inventoryJson)
			eksK8sInstanceWithInventory := v0.AwsEksKubernetesRuntimeInstance{
				Common: v0.Common{
					ID: awsEksKubernetesRuntimeInstance.ID,
				},
				ResourceInventory: &dbInventory,
			}
			_, err = client.UpdateAwsEksKubernetesRuntimeInstance(
				r.APIClient,
				r.APIServer,
				&eksK8sInstanceWithInventory,
			)
			if err != nil {
				reconLog.Error(err, "failed to update EKS cluster instance inventory")
			}
		}
	}()

	clusterInfra := provider.KubernetesRuntimeInfraEKS{
		RuntimeInstanceName:          *awsEksKubernetesRuntimeInstance.Name,
		AwsAccountID:                 *awsAccount.AccountID,
		AwsConfig:                    awsConfig,
		ResourceClient:               &eksClient,
		ZoneCount:                    int32(*awsEksKubernetesRuntimeDefinition.ZoneCount),
		DefaultNodeGroupInstanceType: *awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialNodes: int32(*awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInitialSize),
		DefaultNodeGroupMinNodes:     int32(*awsEksKubernetesRuntimeDefinition.DefaultNodeGroupMinimumSize),
		DefaultNodeGroupMaxNodes:     int32(*awsEksKubernetesRuntimeDefinition.DefaultNodeGroupMaximumSize),
	}

	go createInfra(
		r,
		&clusterInfra,
		awsEksKubernetesRuntimeInstance,
		&reconLog,
		awsEksKubernetesRuntimeInstance.ResourceInventory,
	)

	return 90, nil
}

// awsEksKubernetesRuntimeInstanceUpdated reconciles state for updated AWS EKS
// runtimes.
func awsEksKubernetesRuntimeInstanceUpdated(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// awsEksKubernetesRuntimeInstanceDeleted removes an AWS EKS runtime.
func awsEksKubernetesRuntimeInstanceDeleted(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// add log metadata
	reconLog := log.WithValues(
		"awsEksKubernetesRuntimeInstanceID", *awsEksKubernetesRuntimeInstance.ID,
		"awsEksKubernetesRuntimeInstanceName", *awsEksKubernetesRuntimeInstance.Name,
	)

	// check that deletion is scheduled - if not there's a problem
	if awsEksKubernetesRuntimeInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if awsEksKubernetesRuntimeInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// check to see if previously acknowledged - nil means it has not been
	// acknowledged
	if awsEksKubernetesRuntimeInstance.DeletionAcknowledged != nil {
		// deletion has been acknowledged, check deletion
		deleted, err := checkDeleted(r, awsEksKubernetesRuntimeInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to check if EKS cluster infra resources are deleted: %w", err)
		}
		if !deleted {
			// return a custom requeue of 60 seconds to re-check resources again
			return 60, nil
		}

		return 0, nil
	}

	// acknowledge deletion scheduled
	timestamp := time.Now().UTC()
	awsEksKubernetesRuntimeInstance.DeletionAcknowledged = &timestamp
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(
		r.APIClient,
		r.APIServer,
		awsEksKubernetesRuntimeInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set deletion acknowledge timestamp: %w", err)
	}

	// get cluster definition and aws account info
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retreive cluster definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeDefinition.AwsAccountID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	awsConfig, err := kube.GetAwsConfigFromAwsAccount(r.EncryptionKey, *awsEksKubernetesRuntimeInstance.Region, awsAccount)
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS config: %w", err)
	}

	// create EKS resource client
	inventoryChan := make(chan eks.EksInventory)
	eksClient := eks.EksClient{
		*aws_client.CreateResourceClient(awsConfig),
		&inventoryChan,
	}

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *eksClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store updated inventory in database as it arrives on inventory channel
	go func() {
		for inventory := range *eksClient.InventoryChan {
			inventoryJson, err := inventory.Marshal()
			if err != nil {
				reconLog.Error(err, "failed to marshal inventory")
			}
			dbInventory := datatypes.JSON(inventoryJson)
			eksK8sInstanceWithInventory := v0.AwsEksKubernetesRuntimeInstance{
				Common: v0.Common{
					ID: awsEksKubernetesRuntimeInstance.ID,
				},
				ResourceInventory: &dbInventory,
			}
			_, err = client.UpdateAwsEksKubernetesRuntimeInstance(
				r.APIClient,
				r.APIServer,
				&eksK8sInstanceWithInventory,
			)
			if err != nil {
				reconLog.Error(err, "failed to update EKS cluster instance inventory")
			}
		}
	}()

	// get EKS cluster's resource inventory to delete
	var resourceInventory eks.EksInventory
	if awsEksKubernetesRuntimeInstance.ResourceInventory != nil {
		if err := resourceInventory.Unmarshal(
			*awsEksKubernetesRuntimeInstance.ResourceInventory,
		); err != nil {
			return 0, fmt.Errorf("failed to unmarshal resource inventory: %w", err)
		}
	}

	// construct the infra object for deletion
	clusterInfra := provider.KubernetesRuntimeInfraEKS{
		RuntimeInstanceName: *awsEksKubernetesRuntimeInstance.Name,
		AwsAccountID:        *awsAccount.AccountID,
		AwsConfig:           awsConfig,
		ResourceClient:      &eksClient,
		ResourceInventory:   &resourceInventory,
	}

	go deleteInfra(&clusterInfra, &reconLog)

	// cluster infra resource deletion started, return custom requeue to check
	// resources in 5 min
	return 300, nil
}

// getInventory takes an aws eks kubernetes runtime instance and retrieves the
// latest resource inventory from the threeport API then returns the inventory.
func getInventory(
	r *controller.Reconciler,
	eksRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
) (*eks.EksInventory, error) {
	// retrieve eks cluster instance
	latestAwsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*eksRuntimeInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster instance inventory from threeport API: %w", err)
	}

	// unmarshal the inventory into an EksInventory object
	var inventory eks.EksInventory
	if latestAwsEksKubernetesRuntimeInstance.ResourceInventory != nil {
		if err := inventory.Unmarshal(
			*latestAwsEksKubernetesRuntimeInstance.ResourceInventory,
		); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource inventory: %w", err)
		}
	}

	return &inventory, nil
}

// createInfra creates the AWS resources for an EKS cluster.  If it fails, it
// calls the API to persist that fact.  If it fails to call the API, it retries
// that call every 10 seconds until it succeeds.
func createInfra(
	r *controller.Reconciler,
	clusterInfra *provider.KubernetesRuntimeInfraEKS,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
	resourceInventory *datatypes.JSON,
) {
	// unmarshal resource inventory
	var inventory eks.EksInventory
	if resourceInventory != nil {
		if err := inventory.Unmarshal(*resourceInventory); err != nil {
			log.Error(err, "failed to unmarshal existing inventory")
			persistCreateFailure(
				r,
				*awsEksKubernetesRuntimeInstance.ID,
				log,
			)
		}
	}
	clusterInfra.ExistingResourceInventory = &inventory

	// refresh the creation acknowledgement until this function returns
	quitChan := make(chan bool)
	go refreshAcknowledgement(
		r,
		awsEksKubernetesRuntimeInstance,
		quitChan,
		log,
	)
	defer func() {
		quitChan <- true
	}()

	_, err := clusterInfra.Create()
	if err != nil {
		log.Error(err, "failed to create EKS cluster infra")
		persistCreateFailure(
			r,
			*awsEksKubernetesRuntimeInstance.ID,
			log,
		)
	}
}

// refreshAcknowledgement refreshes the creation acknowledged timestamp every 60
// seconds until told to quit
func refreshAcknowledgement(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	quitChan chan bool,
	log *logr.Logger,
) {
	for {
		select {
		case <-quitChan:
			return
		default:
			// refresh the acknowledgement timestamp
			refreshAckTimestamp := time.Now().UTC()
			awsEksKubernetesRuntimeInstance.CreationAcknowledged = &refreshAckTimestamp
			_, err := client.UpdateAwsEksKubernetesRuntimeInstance(
				r.APIClient,
				r.APIServer,
				awsEksKubernetesRuntimeInstance,
			)
			if err != nil {
				log.Error(err, "failed to refresh creation acknowledged timestamp")
			}

			time.Sleep(time.Second * 60)
		}
	}
}

// deleteInfra deletes the EKS cluster resources in AWS.
func deleteInfra(clusterInfra *provider.KubernetesRuntimeInfraEKS, log *logr.Logger) {
	if err := clusterInfra.Delete(); err != nil {
		log.Error(err, "failed to delete EKS cluster infra")
	}
}

// checkEksCreated checks to see if all of an EKS cluster's AWS resources have
// been created.
func checkEksCreated(
	r *controller.Reconciler,
	eksRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
) (bool, error) {
	inventory, err := getInventory(r, eksRuntimeInstance)
	if err != nil {
		return false, fmt.Errorf("failed to get EKS cluster's AWS resource inventory for creation check: %w", err)
	}

	// the cluster addon is the final thing to be created - if it is set to
	// true, all resources have been created
	if inventory.ClusterAddon {
		return true, nil
	}

	return false, nil
}

// checkStaleEksAck checks to see if the creation acknowledged timestamp on an
// AWS EKS cluster has gone stale indicating the creation process was
// interrupted.
func checkStaleEksAck(creationAcknowledged time.Time) bool {
	duration := time.Now().UTC().Sub(creationAcknowledged)
	if duration.Seconds() > staleAckDurationSeconds {
		return true
	}

	return false
}

// checkDeleted checks to see if all of an EKS cluster's AWS resources have been
// removed.
func checkDeleted(
	r *controller.Reconciler,
	eksRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
) (bool, error) {
	inventory, err := getInventory(r, eksRuntimeInstance)
	if err != nil {
		return false, fmt.Errorf("failed to get EKS cluster's AWS resource inventory for deletion check: %w", err)
	}

	// the VPC is the last thing to be deleted - if it's ID is removed, all
	// resources are deleted
	if inventory.VpcId == "" {
		return true, nil
	}

	return false, nil
}

// persistCreateFailure calls the threeport API to set CreationFailed to true to
// notify subsequent reconciliation loops that creation failed.  If the call to
// the API fails, it is retried every 10 seconds until it succeeds.
func persistCreateFailure(
	r *controller.Reconciler,
	eksRuntimeInstanceId uint,
	log *logr.Logger,
) {
	failurePersisted := false
	for !failurePersisted {
		creationFailed := true
		failedAwsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
			Common: v0.Common{
				ID: &eksRuntimeInstanceId,
			},
			Reconciliation: v0.Reconciliation{
				CreationFailed: &creationFailed,
			},
		}
		_, err := client.UpdateAwsEksKubernetesRuntimeInstance(
			r.APIClient,
			r.APIServer,
			&failedAwsEksKubernetesRuntimeInstance,
		)
		if err != nil {
			log.Error(err, "failed to persist failure of EKS cluster infra resource creation - retrying in 10 sec")
			time.Sleep(time.Second * 10)
			continue
		}

		failurePersisted = true
	}
}
