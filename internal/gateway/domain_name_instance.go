package gateway

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/nukleros/eks-cluster/pkg/resource"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// domainNameInstanceCreated performs reconciliation when a domain name instance
// has been created.
func domainNameInstanceCreated(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	// get kubernetes runtime instance
	kri, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// get kubernetes runtime definition
	krd, err := client.GetKubernetesRuntimeDefinitionByID(r.APIClient, r.APIServer, *kri.KubernetesRuntimeDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
	}

	// get infra provider
	infraProvider := *krd.InfraProvider

	switch infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		// get dns management role arn
		aekri, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(r.APIClient, r.APIServer, *domainNameInstance.KubernetesRuntimeInstanceID)
		if err != nil {
			return fmt.Errorf("failed to get aws eks kubernetes runtime instance: %w", err)
		}

		// unmarshal the inventory into an ResourceInventory object
		var inventory resource.ResourceInventory
		err = resource.UnmarshalInventory(
			[]byte(*aekri.ResourceInventory),
			&inventory,
		)
		if err != nil {
			return fmt.Errorf("failed to unmarshal resource inventory: %w", err)
		}

		iamRoleArn := inventory.DNSManagementRole.RoleARN

	default:
		break
	}

	return nil
}

// domainNameInstanceUpdated performs reconciliation when a domain name instance
// has been updated.
func domainNameInstanceUpdated(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	return nil
}

// domainNameInstanceDeleted performs reconciliation when a domain name instance
// has been deleted.
func domainNameInstanceDeleted(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	return nil
}
