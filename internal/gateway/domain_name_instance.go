package gateway

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *domainNameInstance.DomainNameDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get domain name definition: %w", err)
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.WorkloadInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get workload instance: %w", err)
	}

	// configure virtual service
	virtualService, err := configureWorkloadResourceInstance(r, domainNameDefinition, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to configure virtual service: %w", err)
	}

	// update workload resource instance
	_, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, virtualService)
	if err != nil {
		return fmt.Errorf("failed to create workload resource instance: %w", err)
	}

	// trigger a reconciliation of the workload instance
	workloadInstanceReconciled := false
	workloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to update workload instance: %w", err)
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

// configureWorkloadResourceInstance configures the dns endpoint for a domain name instance.
func configureWorkloadResourceInstance(
	r *controller.Reconciler,
	domainNameDefinition *v0.DomainNameDefinition,
	workloadInstance *v0.WorkloadInstance,
) (*v0.WorkloadResourceInstance, error) {

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(r.APIClient, r.APIServer, *workloadInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances: %w", err)
	}

	// get workload resource instance
	workloadResourceInstance, err := util.GetUniqueWorkloadResourceInstance(workloadResourceInstances, "VirtualService")
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instance: %w", err)
	}

	// unmarshal service
	virtualServiceUnmarshaled, err := util.UnmarshalJSON(*workloadResourceInstance.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal service workload resource instance: %w", err)
	}

	// unmarshal service name
	err = unstructured.SetNestedField(
		virtualServiceUnmarshaled,
		domainNameDefinition.Domain,
		"metadata",
		"annotations",
		"external-dns.alpha.kubernetes.io/hostname",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set virtual service name: %w", err)
	}

	// marshal virtual service
	virtualServiceMarshaled, err := util.MarshalJSON(virtualServiceUnmarshaled)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal virtual service: %w", err)
	}

	workloadResourceInstanceReconciled := false
	workloadResourceInstance.Reconciled = &workloadResourceInstanceReconciled
	workloadResourceInstance.JSONDefinition = &virtualServiceMarshaled

	return workloadResourceInstance, nil

	// endpoints := []interface{}{
	// 	map[string]interface{}{
	// 		"dnsName":    domainNameDefinition.Domain,
	// 		"recordTTL":  180,
	// 		"recordType": "A",
	// 		"targets": []interface{}{
	// 			"192.168.99.216",
	// 		},
	// 	},
	// }

}
