package aws

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	//"github.com/nukleros/eks-cluster/pkg/api"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

func awsEksClusterInstanceCreated(
	r *controller.Reconciler,
	awsEksClusterInstance *v0.AwsEksClusterInstance,
	log *logr.Logger,
) error {
	// get cluster definition and aws account info
	awsEksClusterDefinition, err := client.GetAwsEksClusterDefinitionByID(
		r.APIClient,
		r.APIServer,
		*awsEksClusterInstance.AwsEksClusterDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to retreive cluster definition by ID: %w", err)
	}
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*awsEksClusterInstance.AwsAccountID,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	// create AWS config
	awsConfig, err := LoadAWSConfigFromAPIKeys(
		awsAccount.AccessKeyID,
		awsAccount.SecretAccessKey,
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// create resource client
	resourceClient, err := api.CreateResourceClientForWorkload(awsConfig)
	if err != nil {
		return fmt.Errof("failed to create EKS resource creation client: %w", err)
	}

	// log messages from channel in resource client on goroutine

	// store inventory in database as they arrive on inventory channel

	controlPlaneInfra := provider.ControlPlaneInfraEKS{
		ThreeportInstanceName: awsEksClusterInstance.Name,
		AwsAccountID:          awsAccount.AccountID,
		AwsConfig:             awsConfig,
		ResourceClient:        resourceClient,
	}

	sigs := make(chan os.Signal)
	kubeConnectionInfo, err := controlPlaneInfra.Create("", sigs)

	// create control plane infra
	kubeConnectionInfo, err := controlPlanInfra.Create("", sigs)
	//if err != nil {
	//	_ = controlPlaneInfra.Delete
	//}

	reconLog := log.WithValues(
		"awsEksClsuterDefinitionRegion", *awsEksClusterDefinition.Region,
		"awsEksClsuterDefinitionZoneCount", *awsEksClusterDefinition.ZoneCount,
		"awsEksClsuterDefinitionDefaultNodeGroupInstanceType", *awsEksClusterDefinition.DefaultNodeGroupInstanceType,
		"awsAccountAccessKeyID", *awsAccount.AccessKeyID,
	)
	reconLog.Info("info collected")

	return nil
}

func awsEksClusterInstanceDeleted(
	r *controller.Reconciler,
	awsEksClusterInstance *v0.AwsEksClusterInstance,
	log *logr.Logger,
) error {
	return nil
}
