package kubernetesruntime

import (
	"fmt"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/kubernetesruntime/mapping"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// kubernetesRuntimeDefinitionCreated reconciles state for a new kubernetes
// runtime definition.
func kubernetesRuntimeDefinitionCreated(
	r *controller.Reconciler,
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
	log *logr.Logger,
) error {
	// if a cluster definition is created by another mechanism and being
	// registered in the system with Reconciled=true, there's no need to do
	// anything - return immediately without error
	if *kubernetesRuntimeDefinition.Reconciled == true {
		return nil
	}
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return nil
	case v0.KubernetesRuntimeInfraProviderEKS:
		// look up AWS account
		var awsAccount v0.AwsAccount
		if kubernetesRuntimeDefinition.InfraProviderAccountName != nil {
			// look up account by account name
			account, err := client.GetAwsAccountByName(
				r.APIClient,
				r.APIServer,
				*kubernetesRuntimeDefinition.InfraProviderAccountName,
			)
			if err != nil {
				return fmt.Errorf("failed to get AWS account by account name: %w", err)
			}
			awsAccount = *account
		} else {
			// look up default account
			account, err := client.GetAwsAccountByDefaultAccount(
				r.APIClient,
				r.APIServer,
			)
			if err != nil {
				return fmt.Errorf("failed to AWS account by ID: %w", err)
			}
			awsAccount = *account
		}

		// create an AWS EKS cluster definition
		var zoneCount int
		if *kubernetesRuntimeDefinition.HighAvailability {
			zoneCount = 3
		} else {
			zoneCount = 2
		}
		nodeGroupInstanceType, err := mapping.GetMachineType(
			"aws",
			*kubernetesRuntimeDefinition.NodeProfile,
			*kubernetesRuntimeDefinition.NodeSize,
		)
		if err != nil {
			return fmt.Errorf("failed to map node size and profile to AWS machine type: %w", err)
		}
		defaultNodeGroupInitialSize := 2
		defaultNodeGroupMinSize := 0
		awsEksKubernetesRuntimeDefinition := v0.AwsEksKubernetesRuntimeDefinition{
			Definition: v0.Definition{
				Name: kubernetesRuntimeDefinition.Name,
			},
			AwsAccountID:                  awsAccount.ID,
			ZoneCount:                     &zoneCount,
			DefaultNodeGroupInstanceType:  &nodeGroupInstanceType,
			DefaultNodeGroupInitialSize:   &defaultNodeGroupInitialSize,
			DefaultNodeGroupMinimumSize:   &defaultNodeGroupMinSize,
			DefaultNodeGroupMaximumSize:   kubernetesRuntimeDefinition.NodeMaximum,
			KubernetesRuntimeDefinitionID: kubernetesRuntimeDefinition.ID,
		}
		_, err = client.CreateAwsEksKubernetesRuntimeDefinition(
			r.APIClient,
			r.APIServer,
			&awsEksKubernetesRuntimeDefinition,
		)
		if err != nil {
			return fmt.Errorf("failed to create new AWS EKS kubernetes runtime: %w", err)
		}
	}

	return nil
}

// kubernetesRuntimeDefinitionCreated reconciles state for a kubernetes
// runtime definition whenever it is changed.
func kubernetesRuntimeDefinitionUpdated(
	r *controller.Reconciler,
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
	log *logr.Logger,
) error {
	return nil
}

// kubernetesRuntimeDefinitionCreated reconciles state for a kubernetes
// runtime definition whenever it is removed.
func kubernetesRuntimeDefinitionDeleted(
	r *controller.Reconciler,
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
	log *logr.Logger,
) error {
	return nil
}
