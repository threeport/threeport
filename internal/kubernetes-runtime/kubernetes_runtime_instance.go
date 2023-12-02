package kubernetesruntime

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// kubernetesRuntimeInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func kubernetesRuntimeInstanceCreated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get kubernetes runtime definition by ID: %w", err)
	}

	// create the provider-specific instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return 0, nil
	case v0.KubernetesRuntimeInfraProviderEKS:
		// get AWS EKS runtime definition
		awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeDefinition.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get aws eks runtime definition by kubernetes runtime definition ID: %w", err)
		}

		// add AWS EKS runtime instance
		region, err := mapping.GetProviderRegionForLocation(v0.KubernetesRuntimeInfraProviderEKS, *kubernetesRuntimeInstance.Location)
		if err != nil {
			return 0, fmt.Errorf("failed to map threeport location to AWS region: %w", err)
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
			return 0, fmt.Errorf("failed to create EKS kubernetes runtime instance: %w", err)
		}
	}

	return 0, nil
}

// kubernetesRuntimeInstanceUpdated reconciles state for a kubernetes
// runtime instance whenever it is changed.
func kubernetesRuntimeInstanceUpdated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// check to see if we have API endpoint - no further reconciliation can
	// occur until we have that
	if kubernetesRuntimeInstance.APIEndpoint == nil {
		return 0, nil
	}

	// check to see if kubernetes runtime is being deleted - if so no updates
	// required
	if kubernetesRuntimeInstance.DeletionScheduled != nil {
		return 0, nil
	}

	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve kubernetes runtime definition by ID: %w", err)
	}

	// get kube client to install compute space control plane components
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		false,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	}

	// TODO: sort out an elegant way to pass the custom image info for
	// install compute space control plane components
	var agentImage string
	if kubernetesRuntimeInstance.ThreeportAgentImage != nil {
		agentImage = *kubernetesRuntimeInstance.ThreeportAgentImage
	}

	cpi := threeport.NewInstaller()

	if agentImage != "" {
		agentRegistry, _, agentTag, err := util.ParseImage(agentImage)
		if err != nil {
			return 0, fmt.Errorf("failed to parse custom threeport agent image: %w", err)
		}

		cpi.Opts.AgentInfo.ImageRepo = agentRegistry
		cpi.Opts.AgentInfo.ImageTag = agentTag
	}

	// threeport control plane components
	if err := cpi.InstallComputeSpaceControlPlaneComponents(
		dynamicKubeClient,
		mapper,
		*kubernetesRuntimeInstance.Name,
	); err != nil {
		return 0, fmt.Errorf("failed to insall compute space control plane components: %w", err)
	}

	if *kubernetesRuntimeDefinition.InfraProvider == v0.KubernetesRuntimeInfraProviderEKS {
		// get aws account
		awsAccount, err := client.GetAwsAccountByName(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeDefinition.InfraProviderAccountName,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get AWS account by name: %w", err)
		}

		// system components e.g. cluster-autoscaler
		if err := threeport.InstallThreeportSystemServices(
			dynamicKubeClient,
			mapper,
			*kubernetesRuntimeDefinition.InfraProvider,
			*kubernetesRuntimeInstance.Name,
			*awsAccount.AccountID,
		); err != nil {
			return 0, fmt.Errorf("failed to install system services: %w", err)
		}
	}

	return 0, nil
}

// kubernetesRuntimeInstanceDeleted reconciles state for a kubernetes
// runtime instance whenever it is removed.
func kubernetesRuntimeInstanceDeleted(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if kubernetesRuntimeInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if kubernetesRuntimeInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve kubernetes runtime definition by ID: %w", err)
	}

	// TODO: delete support services svc to elliminate ELB

	// delete kubernetes runtime instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// get AWS EKS runtime instance
		awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get aws eks runtime instance by kubernetes runtime instance ID: %w", err)
		}

		// delete AWS EKS runtime instance
		_, err = client.DeleteAwsEksKubernetesRuntimeInstance(
			r.APIClient,
			r.APIServer,
			*awsEksKubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to delete aws eks runtime instance by ID: %w", err)
		}
	}

	return 0, nil
}
