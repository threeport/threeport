package kubernetesruntime

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
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
) (int64, error) {
	// if a cluster definition is created by another mechanism and being
	// registered in the system with Reconciled=true, there's no need to do
	// anything - return immediately without error
	if *kubernetesRuntimeDefinition.Reconciled == true {
		return 0, nil
	}
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return 0, nil
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
				return 0, fmt.Errorf("failed to get AWS account by account name: %w", err)
			}
			awsAccount = *account
		} else {
			// look up default account
			account, err := client.GetAwsAccountByDefaultAccount(
				r.APIClient,
				r.APIServer,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to AWS account by ID: %w", err)
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
			return 0, fmt.Errorf("failed to map node size and profile to AWS machine type: %w", err)
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
			return 0, fmt.Errorf("failed to create new AWS EKS kubernetes runtime: %w", err)
		}
	}

	return 0, nil
}

// kubernetesRuntimeDefinitionCreated reconciles state for a kubernetes
// runtime definition whenever it is changed.
func kubernetesRuntimeDefinitionUpdated(
	r *controller.Reconciler,
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// kubernetesRuntimeDefinitionCreated reconciles state for a kubernetes
// runtime definition whenever it is removed.
func kubernetesRuntimeDefinitionDeleted(
	r *controller.Reconciler,
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if kubernetesRuntimeDefinition.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if kubernetesRuntimeDefinition.DeletionConfirmed != nil {
		return 0, nil
	}

	// delete the kubernetes runtime definition that was scheduled for deletion
	deletionReconciled := true
	deletionTimestamp := time.Now().UTC()
	deletedKubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
		Common: v0.Common{
			ID: kubernetesRuntimeDefinition.ID,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled:           &deletionReconciled,
			DeletionAcknowledged: &deletionTimestamp,
			DeletionConfirmed:    &deletionTimestamp,
		},
	}
	_, err := client.UpdateKubernetesRuntimeDefinition(
		r.APIClient,
		r.APIServer,
		&deletedKubernetesRuntimeDefinition,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to confirm deletion of kubernetes runtime definition in threeport API: %w", err)
	}
	_, err = client.DeleteKubernetesRuntimeDefinition(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeDefinition.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete kubernetes runtime definition in threeport API: %w", err)
	}

	return 0, nil
}
