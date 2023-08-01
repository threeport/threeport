package gateway

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// domainNameInstanceCreated performs reconciliation when a domain name instance
// has been created.
func domainNameInstanceCreated(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	infraProvider, err := client.GetInfraProviderByKubernetesRuntimeInstanceID(r.APIClient, r.APIServer, domainNameInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get infra provider: %w", err)
	}

	switch *infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		_, err := client.GetDnsManagementIamRoleArnByK8sRuntimeInst(r.APIClient, r.APIServer, domainNameInstance.KubernetesRuntimeInstanceID)
		if err != nil {
			return fmt.Errorf("failed to get dns management iam role arn: %w", err)
		}

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

}
