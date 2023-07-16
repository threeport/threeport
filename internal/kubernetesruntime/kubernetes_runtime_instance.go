package kubernetesruntime

import (
	"fmt"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// kubernetesRuntimeInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func kubernetesRuntimeInstanceCreated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	// if a cluster instance is created by another mechanism and being
	// registered in the system with Reconciled=true, there's no need to do
	// anything - return immediately without error
	if *kubernetesRuntimeInstance.Reconciled {
		return nil
	}

	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to kubernetes runtime definition by ID: %w", err)
	}

	// get AWS EKS runtime definition
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeDefinition.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get AWS EKS runtime definition by kubernetes runtime definition ID: %w", err)
	}

	// create the provider-specific instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return nil
	case v0.KubernetesRuntimeInfraProviderEKS:
		region := "us-east-2" // TODO: add mapping
		awsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: kubernetesRuntimeInstance.Name,
			},
			Region:                              &region,
			KubernetesRuntimeInstanceID:         kubernetesRuntimeInstance.ID,
			AwsEksKubernetesRuntimeDefinitionID: awsEksKubernetesRuntimeDefinition.ID,
		}
		_, err := client.CreateAwsEksKubernetesRuntimeInstance(
			r.APIClient,
			r.APIServer,
			&awsEksKubernetesRuntimeInstance,
		)
		if err != nil {
			return fmt.Errorf("failed to create EKS kubernetes runtime instance: %w", err)
		}
	}

	return nil
}

// kubernetesRuntimeInstanceCreated reconciles state for a kubernetes
// runtime instance whenever it is changed.
func kubernetesRuntimeInstanceUpdated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	return nil
}

// kubernetesRuntimeInstanceCreated reconciles state for a kubernetes
// runtime instance whenever it is removed.
func kubernetesRuntimeInstanceDeleted(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	return nil
}
