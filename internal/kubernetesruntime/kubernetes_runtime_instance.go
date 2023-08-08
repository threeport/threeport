package kubernetesruntime

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/kubernetesruntime/mapping"
	"github.com/threeport/threeport/internal/threeport"
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
	// TODO: remove this once rebased onto Randy's changes that will remove the
	// notification from coming in if reconciled
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
		return fmt.Errorf("failed to get kubernetes runtime definition by ID: %w", err)
	}

	// create the provider-specific instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return nil
	case v0.KubernetesRuntimeInfraProviderEKS:
		// get AWS EKS runtime definition
		awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeDefinition.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get aws eks runtime definition by kubernetes runtime definition ID: %w", err)
		}

		// add AWS EKS runtime instance
		region, err := mapping.GetProviderRegionForLocation("aws", *kubernetesRuntimeInstance.Location)
		if err != nil {
			return fmt.Errorf("failed to map threeport location to AWS region: %w", err)
		}
		awsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: kubernetesRuntimeInstance.Name,
			},
			Region:                              &region,
			KubernetesRuntimeInstanceID:         kubernetesRuntimeInstance.ID,
			AwsEksKubernetesRuntimeDefinitionID: awsEksKubernetesRuntimeDefinition.ID,
		}
		_, err = client.CreateAwsEksKubernetesRuntimeInstance(
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

// kubernetesRuntimeInstanceUpdate reconciles state for a kubernetes
// runtime instance whenever it is changed.
func kubernetesRuntimeInstanceUpdated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	// check to see if we have API endpoint - no further reconciliation can
	// occur until we have that
	if kubernetesRuntimeInstance.APIEndpoint == nil {
		return nil
	}

	// install compute space control plane components
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		false,
		r.APIClient,
		r.APIServer,
	)
	if err != nil {
		return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	}

	// TODO: sort out an elegant way to pass the custom image info for
	// threeport-agent and other components
	if err := threeport.InstallComputeSpaceControlPlaneComponents(
		dynamicKubeClient,
		mapper,
		*kubernetesRuntimeInstance.Name,
	); err != nil {
		return fmt.Errorf("failed to insall compute space control plane components: %w", err)
	}

	return nil
}

// kubernetesRuntimeInstanceCreated reconciles state for a kubernetes
// runtime instance whenever it is removed.
func kubernetesRuntimeInstanceDeleted(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {
	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to kubernetes runtime definition by ID: %w", err)
	}

	// check for workload instances - delete if any present as related
	// infrastructure, such as cloud provider load balancers may prevent runtime
	// instance deletion
	workloadInstances, err := client.GetWorkloadInstancesByKubernetesRuntimeInstanceID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload instances running in kubernetes runtime: %w", err)
	}

	if len(*workloadInstances) > 0 {
		for _, wi := range *workloadInstances {
			_, err := client.DeleteWorkloadInstance(
				r.APIClient,
				r.APIServer,
				*wi.ID,
			)
			if err != nil {
				return fmt.Errorf("failed to delete workload instance with ID %d: %w", wi.ID, err)
			}
		}

		// TODO: wait for workload instances to be deleted before deleting runtime
		// without a dumb wait
		time.Sleep(time.Second * 300)

		// TODO: delete support services and control plane components
	}

	// delete kubernetes runtime instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return nil
	case v0.KubernetesRuntimeInfraProviderEKS:
		// get AWS EKS runtime instance
		awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get aws eks runtime instance by kubernetes runtime instance ID: %w", err)
		}

		// delete AWS EKS runtime instance
		_, err = client.DeleteAwsEksKubernetesRuntimeInstance(
			r.APIClient,
			r.APIServer,
			*awsEksKubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to delete aws eks runtime instance by ID: %w", err)
		}

		return nil
	}

	return nil
}
