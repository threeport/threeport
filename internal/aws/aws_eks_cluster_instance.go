package aws

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"

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

	controlPlaneInfra := provider.ControlPlaneInfraEKS{
		ThreeportInstanceName: awsEksClusterInstance.Name,
		AwsAccessKeyID:        awsAccount.AccessKeyID,
		AwsSecretAccessKey:    awsAccount.SecretAccessKey,
	}

	sigs := make(chan os.Signal)
	kubeConnectionInfo, err := controlPlaneInfra.Create("", sigs)

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
