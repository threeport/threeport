package aws

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// awsEksKubernetesRuntimeInstanceCreated reconciles state for created AWS EKS
// runtimes by creating a new EKS cluster.
func awsEksKubernetesRuntimeInstanceCreated(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	// add log metadata
	reconLog := log.WithValues(
		"awsEksKubernetesRuntimeInstance", *awsEksKubernetesRuntimeInstance.ID,
		"awsEksKubernetesRuntimeInstance", *awsEksKubernetesRuntimeInstance.Name,
	)

	// check to make sure reconciliation is not being interrupted - if it is
	// return without error to exit reconciliation loop
	// TDOO: add alerts for interrupted reconciliation so humans can intervene
	if awsEksKubernetesRuntimeInstance.InterruptReconciliation != nil && *awsEksKubernetesRuntimeInstance.InterruptReconciliation {
		reconLog.Info("reconciliation interrupted")
		return nil
	}

	// get cluster definition and aws account info
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to retreive cluster definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeDefinition.AwsAccountID,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	// add log metadata
	reconLog = log.WithValues(
		"awsEksClsuterDefinitionRegion", *awsEksKubernetesRuntimeInstance.Region,
		"awsEksClsuterDefinitionZoneCount", *awsEksKubernetesRuntimeDefinition.ZoneCount,
		"awsEksClsuterDefinitionDefaultNodeGroupInstanceType", *awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInstanceType,
		"awsAccountAccessKeyID", *awsAccount.EncryptedAccessKeyID,
	)

	// decrypt access key id and secret access key
	accessKeyID, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.EncryptedAccessKeyID)
	if err != nil {
		return fmt.Errorf("failed to decrypt access key id: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.EncryptedSecretAccessKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret access key: %w", err)
	}

	// create AWS config
	awsConfig, err := resource.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
		"",
		*awsEksKubernetesRuntimeInstance.Region,
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create resource client
	resourceClient := resource.CreateResourceClient(awsConfig)

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// store inventory in database as it arrives on inventory channel
	go func() {
		for inventory := range *resourceClient.InventoryChan {
			inventoryJSON, err := resource.MarshalInventory(&inventory)
			if err != nil {
				reconLog.Error(err, "failed to marshal inventory")
			}
			dbInventory := datatypes.JSON(inventoryJSON)
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

	// delete eks cluster resources if AWS controller is terminated mid-resource
	// creation
	// TODO: add a wait group that prevents the AWS controller from terminating
	// until this process is complete
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		reconLog.Info("controller terminated mid resource creation, removing resources...")

		inventory, err := getInventory(r, awsEksKubernetesRuntimeInstance)
		if err != nil {
			reconLog.Error(err, "aws controller interrupted and failed to retrieve AWS resource inventory")
		}

		if err = resourceClient.DeleteResourceStack(inventory); err != nil {
			reconLog.Error(err, "failed to delete eks cluster resources")
		}
	}()

	clusterInfra := provider.KubernetesRuntimeInfraEKS{
		RuntimeInstanceName: *awsEksKubernetesRuntimeInstance.Name,
		AwsAccountID:        *awsAccount.AccountID,
		AwsConfig:           awsConfig,
		ResourceClient:      resourceClient,
	}

	// create control plane infra
	kubeConnectionInfo, err := clusterInfra.Create()
	if err != nil {
		// since we failed to complete cluster creation, delete it to remove any
		// dangling AWS resources
		createErr := fmt.Errorf("failed to create new threeport cluster: %w", err)
		inventory, invErr := getInventory(r, awsEksKubernetesRuntimeInstance)
		if err != nil {
			return fmt.Errorf("%w and failed to retrieve AWS resource inventory: %w", createErr, invErr)
		}
		if inventory != nil {
			clusterInfra.ResourceInventory = inventory
			if deleteErr := clusterInfra.Delete(); deleteErr != nil {
				// the infra creation AND deletion failed - there is some situation
				// that likely requires human intervention so we will stop
				// reconciliation here to prevent egregious infra creation on an
				// infinite loop
				interrupt := true
				awsEksKubernetesRuntimeInstance.InterruptReconciliation = &interrupt
				_, updateErr := client.UpdateAwsEksKubernetesRuntimeInstance(
					r.APIClient,
					r.APIServer,
					awsEksKubernetesRuntimeInstance,
				)
				if updateErr != nil {
					return fmt.Errorf("%w and failed to update eks runtime instance to interrupt reconciliation: %w", createErr, updateErr)
				}
				return fmt.Errorf("%w and failed to delete created infra: %w", createErr, deleteErr)
			}
		}
		return createErr
	}

	// get kubernetes runtime instance to update kube connection info
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance to update kube connection info: %w", err)
	}

	// encrypt connection token
	encryptedConnectionToken, err := encryption.Encrypt(r.EncryptionKey, kubeConnectionInfo.EKSToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt connection token: %w", err)
	}

	// update kube connection info
	kubeRuntimeReconciled := false
	kubernetesRuntimeInstance.APIEndpoint = &kubeConnectionInfo.APIEndpoint
	kubernetesRuntimeInstance.CACertificate = &kubeConnectionInfo.CACertificate
	kubernetesRuntimeInstance.EncryptedConnectionToken = &encryptedConnectionToken
	kubernetesRuntimeInstance.ConnectionTokenExpiration = &kubeConnectionInfo.EKSTokenExpiration
	kubernetesRuntimeInstance.Reconciled = &kubeRuntimeReconciled
	_, err = client.UpdateKubernetesRuntimeInstance(
		r.APIClient,
		r.APIServer,
		kubernetesRuntimeInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update kubernetes runtime instance with kube connection info: %w", err)
	}

	return nil
}

// awsEksKubernetesRuntimeInstanceUpdated reconciles state for updated AWS EKS
// runtimes.
func awsEksKubernetesRuntimeInstanceUpdated(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	return nil
}

// awsEksKubernetesRuntimeInstanceDeleted removes an AWS EKS runtime.
func awsEksKubernetesRuntimeInstanceDeleted(
	r *controller.Reconciler,
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	// get cluster definition and aws account info
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to retreive cluster definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsEksKubernetesRuntimeDefinition.AwsAccountID,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	// add log metadata
	reconLog := log.WithValues(
		"awsEksClsuterInstsanceRegion", *awsEksKubernetesRuntimeInstance.Region,
		"awsEksClsuterDefinitionZoneCount", *awsEksKubernetesRuntimeDefinition.ZoneCount,
		"awsEksClsuterDefinitionDefaultNodeGroupInstanceType", *awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInstanceType,
		"awsAccountAccessKeyID", *awsAccount.EncryptedAccessKeyID,
	)

	// create AWS config
	awsConfig, err := resource.LoadAWSConfigFromAPIKeys(
		*awsAccount.EncryptedAccessKeyID,
		*awsAccount.EncryptedSecretAccessKey,
		"",
		*awsEksKubernetesRuntimeInstance.Region,
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create resource client
	resourceClient := resource.CreateResourceClient(awsConfig)

	// set inventory channel to nil since we will not be updating the resource
	// inventory in the database - that object has been deleted
	resourceClient.InventoryChan = nil

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// TODO: add a wait group that prevents the AWS controller from terminating
	// until all resources are deleted

	var resourceInventory resource.ResourceInventory
	if awsEksKubernetesRuntimeInstance.ResourceInventory != nil {
		if err := resource.UnmarshalInventory(
			*awsEksKubernetesRuntimeInstance.ResourceInventory,
			&resourceInventory,
		); err != nil {
			return fmt.Errorf("failed to unmarshal resource inventory: %w", err)
		}
	}

	clusterInfra := provider.KubernetesRuntimeInfraEKS{
		RuntimeInstanceName: *awsEksKubernetesRuntimeInstance.Name,
		AwsAccountID:        *awsAccount.AccountID,
		AwsConfig:           awsConfig,
		ResourceClient:      resourceClient,
		ResourceInventory:   &resourceInventory,
	}

	// delete control plane infra
	if err := clusterInfra.Delete(); err != nil {
		return fmt.Errorf("failed to delete new threeport cluster: %w", err)
	}

	return nil
}

// getInventory takes an aws eks kubernetes runtime instance and retrieves the
// latest resource inventory from the threeport API then returns the inventory.
func getInventory(
	r *controller.Reconciler,
	eksRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
) (*resource.ResourceInventory, error) {
	// retrieve eks cluster instance
	latestAwsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*eksRuntimeInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster instance inventory from threeport API: %w", err)
	}

	// unmarshal the inventory into a ResourceInventory object
	var inventory resource.ResourceInventory
	if latestAwsEksKubernetesRuntimeInstance.ResourceInventory != nil {
		if err := resource.UnmarshalInventory(
			[]byte(*latestAwsEksKubernetesRuntimeInstance.ResourceInventory),
			&inventory,
		); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource inventory: %w", err)
		}
	}

	return &inventory, nil
}
