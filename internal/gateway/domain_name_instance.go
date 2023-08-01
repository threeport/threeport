package gateway

import (
	"errors"
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

	// reconcile created domain name instance
	err := reconcileCreatedOrUpdatedDomainNameInstance(r, domainNameInstance, log)
	if err != nil {
		return fmt.Errorf("failed to reconcile created domain name instance: %w", err)
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

	// reconcile updated domain name instance
	err := reconcileCreatedOrUpdatedDomainNameInstance(r, domainNameInstance, log)
	if err != nil {
		return fmt.Errorf("failed to reconcile updated domain name instance: %w", err)
	}

	return nil
}

// domainNameInstanceDeleted performs reconciliation when a domain name instance
// has been deleted.
func domainNameInstanceDeleted(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.WorkloadInstanceID)
	if err != nil {
		if errors.Is(err, client.ErrorObjectNotFound) {
			// workload instance has already been deleted
			return nil
		}
		return fmt.Errorf("failed to get workload instance: %w", err)
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *domainNameInstance.DomainNameDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get domain name definition: %w", err)
	}

	// configure virtual service
	virtualService, err := configureWorkloadResourceInstance(r, domainNameDefinition, workloadInstance)
	if err != nil {
		if errors.Is(err, client.ErrorObjectNotFound) {
			// workload resource instance has already been deleted
			return nil
		}
		return fmt.Errorf("failed to configure virtual service: %w", err)
	}

	// update workload resource instance
	_, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, virtualService)
	if err != nil {
		if errors.Is(err, client.ErrorObjectNotFound) {
			// workload resource instance has already been deleted
			return nil
		}
		return fmt.Errorf("failed to create workload resource instance: %w", err)
	}

	// trigger a reconciliation of the workload instance
	workloadInstanceReconciled := false
	workloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	if err != nil {
		if errors.Is(err, client.ErrorObjectNotFound) {
			// workload instance has already been deleted
			return nil
		}
		return fmt.Errorf("failed to update workload instance: %w", err)
	}

	return nil
}

// reconcileCreatedOrUpdatedDomainNameInstance performs reconciliation when a
// domain name instance has been created or updated.
func reconcileCreatedOrUpdatedDomainNameInstance(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	// validate threeport state
	err := validateThreeportStateExternalDns(r, domainNameInstance, log)
	if err != nil {
		return fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.WorkloadInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get workload instance: %w", err)
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *domainNameInstance.DomainNameDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get domain name definition: %w", err)
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

// validateThreeportStateExternalDns validates the state of the threeport API
// prior to reconciling a domain name instance.
func validateThreeportStateExternalDns(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) error {

	// ensure workload instance is reconciled
	if domainNameInstance.WorkloadInstanceID == nil {
		return fmt.Errorf("failed to determine if workload instance is reconciled, workload instance ID is nil")
	}
	workloadInstanceReconciled, err := client.ConfirmWorkloadInstanceReconciled(r, *domainNameInstance.WorkloadInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if workload instance is reconciled: %w", err)
	}
	if !workloadInstanceReconciled {
		return errors.New("workload instance not reconciled")
	}

	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// ensure gateway controller instance is reconciled
	if kubernetesRuntimeInstance.GatewayControllerInstanceID == nil {
		return fmt.Errorf("failed to determine if gateway controller instance is reconciled, gateway controller instance ID is nil")
	}
	externalDnsControllerInstanceReconciled, err := client.ConfirmWorkloadInstanceReconciled(r, *kubernetesRuntimeInstance.GatewayControllerInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if gateway controller instance is reconciled: %w", err)
	}
	if !externalDnsControllerInstanceReconciled {
		return errors.New("gateway controller instance not reconciled")
	}

	// ensure dns controller is deployed
	err = confirmDnsControllerDeployed(r, domainNameInstance, kubernetesRuntimeInstance, log)
	if err != nil {
		return fmt.Errorf("failed to confirm dns controller deployed: %w", err)
	}

	// ensure dns controller instance is reconciled
	externalDnsControllerInstanceReconciled, err = client.ConfirmWorkloadInstanceReconciled(r, *kubernetesRuntimeInstance.DnsControllerInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if external dns controller instance is reconciled: %w", err)
	}
	if !externalDnsControllerInstanceReconciled {
		return errors.New("external dns controller instance not reconciled")
	}

	return nil
}

// confirmDnsControllerDeployed confirms that a dns controller is deployed for
// the kubernetes runtime instance.
func confirmDnsControllerDeployed(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {

	// return if kubernetes runtime instance already has a dns controller instance
	if kubernetesRuntimeInstance.DnsControllerInstanceID != nil {
		return nil
	}

	// get infra provider
	infraProvider, err := client.GetInfraProviderByKubernetesRuntimeInstanceID(r.APIClient, r.APIServer, domainNameInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get infra provider: %w", err)
	}

	var externalDns string
	switch *infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		iamRoleArn, err := client.GetDnsManagementIamRoleArnByK8sRuntimeInst(r.APIClient, r.APIServer, domainNameInstance.KubernetesRuntimeInstanceID)
		if err != nil {
			return fmt.Errorf("failed to get dns management iam role arn: %w", err)
		}

		externalDns, err = createExternalDns(*iamRoleArn)
		if err != nil {
			return fmt.Errorf("failed to create external dns: %w", err)
		}

	default:
		break
	}

	// create gateway controller workload definition
	workloadDefName := "external-dns"
	externalDnsWorkloadDefinition := v0.WorkloadDefinition{
		Definition:   v0.Definition{Name: &workloadDefName},
		YAMLDocument: &externalDns,
	}

	// create external dns controller workload definition
	createdWorkloadDef, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &externalDnsWorkloadDefinition)
	if err != nil {
		return fmt.Errorf("failed to create external dns controller workload definition: %w", err)
	}

	// create external dns workload instance
	externalDnsWorkloadInstance := v0.WorkloadInstance{
		Instance:                    v0.Instance{Name: &workloadDefName},
		KubernetesRuntimeInstanceID: domainNameInstance.KubernetesRuntimeInstanceID,
		WorkloadDefinitionID:        createdWorkloadDef.ID,
	}
	createdExternalDnsWorkloadInstance, err := client.CreateWorkloadInstance(r.APIClient, r.APIServer, &externalDnsWorkloadInstance)
	if err != nil {
		return fmt.Errorf("failed to create external dns controller workload instance: %w", err)
	}

	// update kubernetes runtime instance with gateway controller instance id
	kubernetesRuntimeInstance.DnsControllerInstanceID = createdExternalDnsWorkloadInstance.ID
	_, err = client.UpdateKubernetesRuntimeInstance(r.APIClient, r.APIServer, kubernetesRuntimeInstance)
	if err != nil {
		return fmt.Errorf("failed to update kubernetes runtime instance with external dns controller instance id: %w", err)
	}

	log.V(1).Info(
		"external dns deployed",
		"workloadInstanceID", externalDnsWorkloadInstance.ID,
	)

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

	// if domainNameDefinition is not passed in, remove hostname annotation
	if domainNameDefinition == nil {
		unstructured.RemoveNestedField(
			virtualServiceUnmarshaled,
			"metadata",
			"annotations",
			"external-dns.alpha.kubernetes.io/hostname",
		)
	} else {
		// otherwise, set hostname annotation

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
