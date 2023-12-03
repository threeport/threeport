package gateway

import (
	"errors"
	"fmt"
	"reflect"

	"strconv"

	"github.com/go-logr/logr"
	workloadutil "github.com/threeport/threeport/internal/workload/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// domainNameInstanceCreated performs reconciliation when a domain name instance
// has been created.
func domainNameInstanceCreated(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) (int64, error) {
	// ensure attached object reference exists
	err := client.EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		reflect.TypeOf(*domainNameInstance).String(),
		domainNameInstance.ID,
		domainNameInstance.WorkloadInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	// reconcile created domain name instance
	err = reconcileCreatedOrUpdatedDomainNameInstance(r, domainNameInstance, log)
	if err != nil {
		return 0, fmt.Errorf("failed to reconcile created domain name instance: %w", err)
	}

	return 0, nil
}

// domainNameInstanceUpdated performs reconciliation when a domain name instance
// has been updated.
func domainNameInstanceUpdated(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) (int64, error) {
	// reconcile updated domain name instance
	err := reconcileCreatedOrUpdatedDomainNameInstance(r, domainNameInstance, log)
	if err != nil {
		return 0, fmt.Errorf("failed to reconcile updated domain name instance: %w", err)
	}

	return 0, nil
}

// domainNameInstanceDeleted performs reconciliation when a domain name instance
// has been deleted.
func domainNameInstanceDeleted(
	r *controller.Reconciler,
	domainNameInstance *v0.DomainNameInstance,
	log *logr.Logger,
) (int64, error) {

	// check that deletion is scheduled - if not there's a problem
	if domainNameInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if domainNameInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.WorkloadInstanceID)
	if err != nil {
		if errors.Is(err, client.ErrorObjectNotFound) {
			// workload instance has already been deleted
			return 0, nil
		}
		log.Error(err, "failed to get workload instance")
		return 0, nil
	}

	// // get domain name definition
	// domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *domainNameInstance.DomainNameDefinitionID)
	// if err != nil {
	// 	log.Error(err, "failed to get domain name definition")
	// 	return 0, nil
	// }

	// // configure virtual service
	// virtualService, err := configureWorkloadResourceInstance(r, domainNameDefinition, workloadInstance)
	// if err != nil {
	// 	if errors.Is(err, client.ErrorObjectNotFound) {
	// 		// workload resource instance has already been deleted
	// 		return 0, nil
	// 	}
	// 	log.Error(err, "failed to configure virtual service")
	// 	return 0, nil
	// }

	// // update workload resource instance
	// _, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, virtualService)
	// if err != nil {
	// 	if errors.Is(err, client.ErrorObjectNotFound) {
	// 		// workload resource instance has already been deleted
	// 		return 0, nil
	// 	}
	// 	log.Error(err, "failed to create workload resource instance")
	// 	return 0, nil
	// }

	// trigger a reconciliation of the workload instance
	workloadInstanceReconciled := false
	workloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	if err != nil {
		if errors.Is(err, client.ErrorObjectNotFound) {
			// workload instance has already been deleted
			return 0, nil
		}
		log.Error(err, "failed to update workload instance")
		return 0, nil
	}

	return 0, nil
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

	return nil

	// // get workload instance
	// workloadInstance, err := client.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *domainNameInstance.WorkloadInstanceID)
	// if err != nil {
	// 	return fmt.Errorf("failed to get workload instance: %w", err)
	// }

	// // get domain name definition
	// domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *domainNameInstance.DomainNameDefinitionID)
	// if err != nil {
	// 	return fmt.Errorf("failed to get domain name definition: %w", err)
	// }

	// // configure virtual service
	// workloadResourceInstance, err := configureWorkloadResourceInstance(r, domainNameDefinition, workloadInstance)
	// if err != nil {
	// 	return fmt.Errorf("failed to configure virtual service: %w", err)
	// }

	// // update workload resource instance
	// _, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, workloadResourceInstance)
	// if err != nil {
	// 	return fmt.Errorf("failed to create workload resource instance: %w", err)
	// }

	// // trigger a reconciliation of the workload instance
	// workloadInstanceReconciled := false
	// workloadInstance.Reconciled = &workloadInstanceReconciled
	// _, err = client.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	// if err != nil {
	// 	return fmt.Errorf("failed to update workload instance: %w", err)
	// }

	// // mark domain name instance as reconciled
	// domainNameInstanceReconciled := true
	// domainNameInstance.Reconciled = &domainNameInstanceReconciled
	// _, err = client.UpdateDomainNameInstance(r.APIClient, r.APIServer, domainNameInstance)
	// if err != nil {
	// 	return fmt.Errorf("failed to update domain name instance: %w", err)
	// }

	// return nil
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

// getGlooEdgeNamespace gets the namespace of the gloo edge instance.
func getGlooEdgeNamespace(r *controller.Reconciler, workloadInstanceID *uint) (string, error) {

	// get gloo edge workload resource instance
	glooEdgeWorkloadResourceInstance, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(r.APIClient, r.APIServer, *workloadInstanceID)
	if err != nil {
		return "", fmt.Errorf("failed to get gloo edge workload resource instance: %w", err)
	}

	// unmarshal gloo edge custom resource
	glooEdge, err := workloadutil.UnmarshalUniqueWorkloadResourceInstance(glooEdgeWorkloadResourceInstance, "GlooEdge")
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal gloo edge workload resource instance: %w", err)
	}

	// get gateway namespace
	glooEdgeNamespace, found, err := unstructured.NestedString(glooEdge, "spec", "namespace")
	if err != nil || !found {
		return "", fmt.Errorf("failed to get namespace from gateway workload resource definition: %w", err)
	}

	return glooEdgeNamespace, nil
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

	// get gloo edge namespace
	glooEdgeNamespace, err := getGlooEdgeNamespace(r, kubernetesRuntimeInstance.GatewayControllerInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get gloo edge namespace: %w", err)
	}

	// get domain name definition
	domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *domainNameInstance.DomainNameDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get domain name definition: %w", err)
	}

	// generate external dns manifest based on infra provider
	var externalDnsManifest string
	kubernetesRuntimeInstanceID := strconv.Itoa(int(*domainNameInstance.KubernetesRuntimeInstanceID))
	switch *infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		resourceInventory, err := client.GetResourceInventoryByK8sRuntimeInst(r.APIClient, r.APIServer, domainNameInstance.KubernetesRuntimeInstanceID)
		if err != nil {
			return fmt.Errorf("failed to get dns management iam role arn: %w", err)
		}

		externalDnsManifest, err = createExternalDns(
			*domainNameDefinition.Domain,
			"route53",
			resourceInventory.DnsManagementRole.RoleArn,
			glooEdgeNamespace,
			kubernetesRuntimeInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to create external dns: %w", err)
		}

	case v0.KubernetesRuntimeInfraProviderKind:

		externalDnsManifest, err = createExternalDns(
			*domainNameDefinition.Domain,
			"none",
			"",
			glooEdgeNamespace,
			kubernetesRuntimeInstanceID,
		)
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
		YAMLDocument: &externalDnsManifest,
	}

	// create external dns controller workload definition
	createdWorkloadDef, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &externalDnsWorkloadDefinition)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return fmt.Errorf("failed to create external dns controller workload definition: %w", err)
	}

	// get external dns controller workload definition id
	var externalDnsWorkloadDefinitionId *uint
	if !errors.Is(err, client.ErrConflict) {
		externalDnsWorkloadDefinitionId = createdWorkloadDef.ID
	} else {
		existingWorkloadDef, err := client.GetWorkloadDefinitionByName(r.APIClient, r.APIServer, workloadDefName)
		if err != nil {
			return fmt.Errorf("failed to get existing workload definition: %w", err)
		}
		externalDnsWorkloadDefinitionId = existingWorkloadDef.ID
	}

	// create external dns workload instance
	externalDnsWorkloadInstance := v0.WorkloadInstance{
		Instance: v0.Instance{
			Name: util.StringPtr(fmt.Sprintf("%s-%s", workloadDefName, *kubernetesRuntimeInstance.Name)),
		},
		KubernetesRuntimeInstanceID: domainNameInstance.KubernetesRuntimeInstanceID,
		WorkloadDefinitionID:        externalDnsWorkloadDefinitionId,
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

// configureWorkloadResourceInstance configures the target workload
// resource instance for a domain name instance.
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
	workloadResourceInstance, err := workloadutil.GetUniqueWorkloadResourceInstance(workloadResourceInstances, "VirtualService")
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instance: %w", err)
	}

	// unmarshal service
	virtualServiceUnmarshaled, err := util.UnmarshalJSON(*workloadResourceInstance.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal service workload resource instance: %w", err)
	}

	// // if domainNameDefinition is not passed in, set domains to default value
	// if domainNameDefinition == nil {
	// 	unstructured.SetNestedStringSlice(
	// 		virtualServiceUnmarshaled,
	// 		[]string{"*"},
	// 		"spec",
	// 		"virtualHost",
	// 		"domains",
	// 	)
	// } else {
	// 	// otherwise, set domain

	// 	err = unstructured.SetNestedStringSlice(
	// 		virtualServiceUnmarshaled,
	// 		[]string{*domainNameDefinition.Domain},
	// 		"spec",
	// 		"virtualHost",
	// 		"domains",
	// 	)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to set virtual service name: %w", err)
	// 	}
	// }

	// marshal virtual service
	virtualServiceMarshaled, err := util.MarshalJSON(virtualServiceUnmarshaled)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal virtual service: %w", err)
	}

	// mark workload resource instance as not reconciled
	workloadResourceInstanceReconciled := false
	workloadResourceInstance.Reconciled = &workloadResourceInstanceReconciled
	workloadResourceInstance.JSONDefinition = &virtualServiceMarshaled

	return workloadResourceInstance, nil

}
