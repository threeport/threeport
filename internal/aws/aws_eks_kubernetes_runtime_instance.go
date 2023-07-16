package aws

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/go-logr/logr"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// awsEksKubernetesRuntimeInstanceCreated reconciles state for created AWS EKS clusters by
// creating a new EKS cluster.
func awsEksKubernetesRuntimeInstanceCreated(
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
		"awsEksClsuterDefinitionRegion", *awsEksKubernetesRuntimeInstance.Region,
		"awsEksClsuterDefinitionZoneCount", *awsEksKubernetesRuntimeDefinition.ZoneCount,
		"awsEksClsuterDefinitionDefaultNodeGroupInstanceType", *awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInstanceType,
		"awsAccountAccessKeyID", *awsAccount.AccessKeyID,
	)

	// create AWS config
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(*awsEksKubernetesRuntimeInstance.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				*awsAccount.AccessKeyID,
				*awsAccount.SecretAccessKey,
				"",
			),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create resource client
	resourceClient := resource.CreateResourceClient(&awsConfig)

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
		// retrieve eks cluster instance
		latestAwsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
			r.APIClient,
			r.APIServer,
			*awsEksKubernetesRuntimeInstance.ID,
		)
		if err != nil {
			reconLog.Error(err, "failed to get EKS cluster instance inventory from threeport API")
		}

		// unmarshal the inventory into an ResourceInventory object
		var inventory resource.ResourceInventory
		if err := resource.UnmarshalInventory(
			[]byte(*latestAwsEksKubernetesRuntimeInstance.ResourceInventory),
			&inventory,
		); err != nil {
			reconLog.Error(err, "failed to unmarshal resource inventory")
		}

		if err = resourceClient.DeleteResourceStack(&inventory); err != nil {
			reconLog.Error(err, "failed to delete eks cluster resources")
		}
	}()

	clusterInfra := provider.KubernetesRuntimeInfraEKS{
		ThreeportInstanceName: *awsEksKubernetesRuntimeInstance.Name,
		AwsAccountID:          *awsAccount.AccountID,
		AwsConfig:             awsConfig,
		ResourceClient:        *resourceClient,
	}

	// create control plane infra
	kubeConnectionInfo, err := clusterInfra.Create()
	if err != nil {
		// since we failed to complete cluster creation, delete it to remove any
		// dangling AWS resources
		_ = clusterInfra.Delete()
		return fmt.Errorf("failed to create new threeport cluster: %w", err)
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

	// update kube connection info
	kubernetesRuntimeInstance.APIEndpoint = &kubeConnectionInfo.APIEndpoint
	kubernetesRuntimeInstance.CACertificate = &kubeConnectionInfo.CACertificate
	kubernetesRuntimeInstance.ConnectionToken = &kubeConnectionInfo.EKSToken
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
		"awsEksClsuterInstsanceResourceInventory", *awsEksKubernetesRuntimeInstance.ResourceInventory,
		"awsEksClsuterDefinitionZoneCount", *awsEksKubernetesRuntimeDefinition.ZoneCount,
		"awsEksClsuterDefinitionDefaultNodeGroupInstanceType", *awsEksKubernetesRuntimeDefinition.DefaultNodeGroupInstanceType,
		"awsAccountAccessKeyID", *awsAccount.AccessKeyID,
	)

	// create AWS config
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(*awsEksKubernetesRuntimeInstance.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				*awsAccount.AccessKeyID,
				*awsAccount.SecretAccessKey,
				"",
			),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create resource client
	resourceClient := resource.CreateResourceClient(&awsConfig)

	// log messages from channel in resource client on goroutine
	go func() {
		for msg := range *resourceClient.MessageChan {
			reconLog.Info(msg)
		}
	}()

	// not storing inventory in database as the AwsEKSKubernetesRuntimeInstance
	// object has been deleted.

	// TODO: add a wait group that prevents the AWS controller from terminating
	// until all resources are deleted

	// unmarshal inventory into resource inventory object for eks-cluster lib
	var resourceInventory resource.ResourceInventory
	fmt.Printf("$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$")
	reconLog.Info("wat")

	inventoryBytes := []byte(*awsEksKubernetesRuntimeInstance.ResourceInventory)
	if err := resource.UnmarshalInventory(inventoryBytes, &resourceInventory); err != nil {
		return fmt.Errorf("failed to unmarshal resource inventory from aws eks kubernetes runtime instance: %w", err)
	}

	clusterInfra := provider.KubernetesRuntimeInfraEKS{
		ThreeportInstanceName: *awsEksKubernetesRuntimeInstance.Name,
		AwsAccountID:          *awsAccount.AccountID,
		AwsConfig:             awsConfig,
		ResourceClient:        *resourceClient,
		ResourceInventory:     resourceInventory,
	}

	// create control plane infra
	if err := clusterInfra.Delete(); err != nil {
		return fmt.Errorf("failed to delete new threeport cluster: %w", err)
	}

	return nil
}
