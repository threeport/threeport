package gateway

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/iancoleman/strcase"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	runtime "github.com/threeport/threeport/internal/kubernetes-runtime"
	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	workloadutil "github.com/threeport/threeport/internal/workload/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
	client "github.com/threeport/threeport/pkg/client/v0"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// gatewayInstanceCreated performs reconciliation when a gateway instance
// has been created.
func gatewayInstanceCreated(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	log *logr.Logger,
) (int64, error) {

	// ensure attached object reference exists
	err := client_v1.EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		util.TypeName(v1.WorkloadInstance{}),
		gatewayInstance.WorkloadInstanceID,
		util.TypeName(*gatewayInstance),
		gatewayInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	// initialize threeport object references
	kubernetesRuntimeInstance, gatewayDefinition, workloadInstance, err := getThreeportObjects(r, gatewayInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// validate threeport state before deploying virtual service
	err = validateThreeportState(r, gatewayDefinition, gatewayInstance, workloadInstance, kubernetesRuntimeInstance, log)
	if err != nil {
		return 0, fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// configure workload resource instances
	workloadResourceInstances, err := configureGatewayWorkloadResourceInstances(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to configure workload resource instances: %w", err)
	}

	// create workload resource instances
	for _, workloadResourceInstance := range *workloadResourceInstances {
		_, err := client.CreateWorkloadResourceInstance(r.APIClient, r.APIServer, &workloadResourceInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to create workload resource instance: %w", err)
		}
	}

	// trigger a reconciliation of the workload instance
	workloadInstance.Reconciled = util.Ptr(false)
	_, err = client_v1.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to update workload instance: %w", err)
	}

	// update gateway instance
	gatewayInstance.Reconciled = util.Ptr(true)
	_, err = client.UpdateGatewayInstance(r.APIClient, r.APIServer, gatewayInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to update gateway instance: %w", err)
	}

	log.V(1).Info(
		"gateway instance created",
		"gatewayResourceInstanceID", gatewayInstance.ID,
	)

	return 0, nil
}

// gatewayInstanceUpdated performs reconciliation when a gateway instance
// has been updated
func gatewayInstanceUpdated(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	log *logr.Logger,
) (int64, error) {

	// initialize threeport object references
	kubernetesRuntimeInstance, gatewayDefinition, workloadInstance, err := getThreeportObjects(r, gatewayInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// validate threeport state before deploying virtual service
	err = validateThreeportState(r, gatewayDefinition, gatewayInstance, workloadInstance, kubernetesRuntimeInstance, log)
	if err != nil {
		return 0, fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// configure workload resource instances
	updatedWorkloadResourceInstances, err := configureGatewayWorkloadResourceInstances(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to configure workload resource instances: %w", err)
	}

	// get workload resource instances
	existingWorkloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(r.APIClient, r.APIServer, *gatewayInstance.WorkloadInstanceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload resource instances: %w", err)
	}

	// get gateway instance objects
	gatewayInstanceObjects, err := getGatewayInstanceObjects(r, gatewayInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get gateway instance objects: %w", err)
	}

	for _, resource := range gatewayInstanceObjects {

		// get workload resource instance for virtual service
		existingWorkloadResourceInstance, err := workloadutil.GetUniqueWorkloadResourceInstance(existingWorkloadResourceInstances, resource)
		if err != nil {
			return 0, fmt.Errorf("failed to get workload resource instance: %w", err)
		}

		// get workload resource instance for virtual service
		updatedWorkloadResourceInstance, err := workloadutil.GetUniqueWorkloadResourceInstance(updatedWorkloadResourceInstances, resource)
		if err != nil {
			return 0, fmt.Errorf("failed to get workload resource instance: %w", err)
		}

		// update the workload resource instance
		workloadResourceInstanceReconciled := false
		existingWorkloadResourceInstance.Reconciled = &workloadResourceInstanceReconciled
		existingWorkloadResourceInstance.JSONDefinition = updatedWorkloadResourceInstance.JSONDefinition
		_, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, existingWorkloadResourceInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to update workload resource instance: %w", err)
		}
	}

	// trigger a reconciliation of the workload instance
	workloadInstanceReconciled := false
	workloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client_v1.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to update workload instance: %w", err)
	}

	// update gateway instance
	gatewayInstanceReconciled := true
	gatewayInstance.Reconciled = &gatewayInstanceReconciled
	_, err = client.UpdateGatewayInstance(r.APIClient, r.APIServer, gatewayInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to update gateway instance: %w", err)
	}

	log.V(1).Info(
		"gateway instance updated",
		"gatewayResourceInstanceID", gatewayInstance.ID,
	)

	return 0, nil
}

// gatewayInstanceDeleted performs reconciliation when a gateway instance
// has been deleted
func gatewayInstanceDeleted(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if gatewayInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if confirmed - it should not be, but if so we should do no
	// more
	if gatewayInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(r.APIClient, r.APIServer, *gatewayInstance.WorkloadInstanceID)
	if err != nil {
		if errors.Is(err, client.ErrObjectNotFound) {
			// workload instance has already been deleted
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get workload resource instances: %w", err)
	}

	// get gateway instance objects
	gatewayInstanceObjects, err := getGatewayInstanceObjects(r, gatewayInstance)
	if err != nil {
		if errors.Is(err, client.ErrObjectNotFound) {
			// workload instance has already been deleted
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get gateway instance objects: %w", err)
	}

	for _, resource := range gatewayInstanceObjects {

		// get workload resource instance for virtual service
		workloadResourceInstance, err := workloadutil.GetUniqueWorkloadResourceInstance(workloadResourceInstances, resource)
		if err != nil {
			// workload instance has already been deleted
			return 0, nil
		}

		// schedule workload resource instance for deletion
		workloadResourceInstance = &v0.WorkloadResourceInstance{
			Common:               v0.Common{ID: workloadResourceInstance.ID},
			ScheduledForDeletion: util.Ptr(time.Now().UTC()),
			Reconciled:           util.Ptr(false),
		}
		_, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, workloadResourceInstance)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				// workload resource instance has already been deleted
				return 0, nil
			}
			return 0, fmt.Errorf("failed to update workload resource instance: %w", err)
		}
	}

	// trigger a reconciliation of the workload instance
	if gatewayInstance.WorkloadInstanceID == nil {
		return 0, fmt.Errorf("failed to delete workload instance, workloadInstanceID is nil")
	}
	workloadInstance := &v1.WorkloadInstance{
		Common:         v0.Common{ID: gatewayInstance.WorkloadInstanceID},
		Reconciliation: v0.Reconciliation{Reconciled: util.Ptr(false)},
	}
	_, err = client_v1.UpdateWorkloadInstance(r.APIClient, r.APIServer, workloadInstance)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to update workload instance: %w", err)
	}

	return 0, nil
}

// getThreeportobjects returns all threeport objects required for
// gateway instance reconciliation
func getThreeportObjects(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
) (*v0.KubernetesRuntimeInstance, *v0.GatewayDefinition, *v1.WorkloadInstance, error) {

	// get kubernetes runtime instance
	if gatewayInstance.KubernetesRuntimeInstanceID == nil {
		return nil, nil, nil, errors.New("kubernetes runtime instance ID is nil")
	}
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, *gatewayInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get gateway kubernetes runtime instance by ID: %w", err)
	}

	// get gateway definition
	if gatewayInstance.GatewayDefinitionID == nil {
		return nil, nil, nil, fmt.Errorf("gateway definition ID is nil")
	}
	gatewayDefinition, err := client.GetGatewayDefinitionByID(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	}

	// get workload instance
	if gatewayInstance.WorkloadInstanceID == nil {
		return nil, nil, nil, fmt.Errorf("workload instance ID is nil")
	}
	workloadInstance, err := client_v1.GetWorkloadInstanceByID(r.APIClient, r.APIServer, *gatewayInstance.WorkloadInstanceID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get workload instance: %w", err)
	}

	return kubernetesRuntimeInstance, gatewayDefinition, workloadInstance, nil
}

// validateThreeportState validates the state of the threeport API
// prior to reconciling a gateway instance
func validateThreeportState(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	gatewayInstance *v0.GatewayInstance,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {

	// ensure gateway and workload definition are reconciled
	definitionsReconciled, err := confirmDefinitionsReconciled(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if workload definition is reconciled: %w", err)
	}
	if !definitionsReconciled {
		return errors.New("workload definition not reconciled")
	}

	// ensure workload instance is reconciled
	if gatewayInstance.WorkloadInstanceID == nil {
		return fmt.Errorf("failed to determine if workload instance is reconciled, workload instance ID is nil")
	}
	workloadInstanceReconciled, err := client.ConfirmWorkloadInstanceReconciled(r, *gatewayInstance.WorkloadInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if workload instance is reconciled: %w", err)
	}
	if !workloadInstanceReconciled {
		return errors.New("workload instance not reconciled")
	}

	// ensure gateway controller is deployed
	err = confirmGatewayControllerDeployed(r, kubernetesRuntimeInstance, log)
	if err != nil {
		return fmt.Errorf("failed to reconcile gateway controller: %w", err)
	}

	// ensure gateway controller instance is reconciled
	if kubernetesRuntimeInstance.GatewayControllerInstanceID == nil {
		return fmt.Errorf("failed to determine if gateway controller instance is reconciled, gateway controller instance ID is nil")
	}
	gatewayControllerInstanceReconciled, err := client.ConfirmWorkloadInstanceReconciled(r, *kubernetesRuntimeInstance.GatewayControllerInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if gateway controller instance is reconciled: %w", err)
	}
	if !gatewayControllerInstanceReconciled {
		return errors.New("gateway controller instance not reconciled")
	}

	// confirm requested ports are exposed
	err = confirmGatewayPortsExposed(r, gatewayInstance, kubernetesRuntimeInstance, gatewayDefinition, log)
	if err != nil {
		return fmt.Errorf("failed to confirm requested port is exposed: %w", err)
	}

	return nil
}

// confirmDefinitionsReconciled confirms the gateway definition related to a
// gateway instance is reconciled.
func confirmDefinitionsReconciled(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
) (bool, error) {

	// get gateway definition
	if gatewayInstance.GatewayDefinitionID == nil {
		return false, fmt.Errorf("failed to get gateway definition from gateway instance, gateway definition ID is nil")
	}
	gatewayDefinition, err := client.GetGatewayDefinitionByID(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return false, fmt.Errorf("failed to get gateway definition by workload definition ID: %w", err)
	}

	// if the gateway definition is not reconciled, return false
	if gatewayDefinition.Reconciled != nil && !*gatewayDefinition.Reconciled {
		return false, nil
	}

	// get workload definition
	if gatewayDefinition.WorkloadDefinitionID == nil {
		return false, fmt.Errorf("failed to get workload definition from gateway definition, workload definition ID is nil")
	}
	workloadDefinition, err := client.GetWorkloadDefinitionByID(r.APIClient, r.APIServer, *gatewayDefinition.WorkloadDefinitionID)
	if err != nil {
		return false, fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}

	// if the workload definition is not reconciled, return false
	if workloadDefinition.Reconciled != nil && !*workloadDefinition.Reconciled {
		return false, nil
	}

	return true, nil
}

// confirmGatewayControllerDeployed confirms the gateway controller is deployed,
// and if not, deploys it.
func confirmGatewayControllerDeployed(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {

	// return if kubernetes runtime instance already has a gateway controller instance
	if kubernetesRuntimeInstance.GatewayControllerInstanceID != nil {
		return nil
	}

	// generate gloo edge manifest
	glooEdge, err := getGlooEdgeYaml()
	if err != nil {
		return fmt.Errorf("failed to create gloo edge resource: %w", err)
	}

	// get infra provider
	infraProvider, err := client.GetInfraProviderByKubernetesRuntimeInstanceID(r.APIClient, r.APIServer, kubernetesRuntimeInstance.ID)
	if err != nil {
		return fmt.Errorf("failed to get infra provider: %w", err)
	}

	// generate cert manager manifest based on infra provider
	var certManager string
	switch *infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		resourceInventory, err := client.GetResourceInventoryByK8sRuntimeInst(r.APIClient, r.APIServer, kubernetesRuntimeInstance.ID)
		if err != nil {
			return fmt.Errorf("failed to get dns management iam role arn: %w", err)
		}

		certManager, err = getCertManagerYaml(resourceInventory.Dns01ChallengeRole.RoleArn)
		if err != nil {
			return fmt.Errorf("failed to create cert manager resource: %w", err)
		}

	case v0.KubernetesRuntimeInfraProviderKind:

		certManager, err = getCertManagerYaml("")
		if err != nil {
			return fmt.Errorf("failed to create cert manager resource: %w", err)
		}

	default:
		break
	}

	// concatenate gloo edge, support services collection, and cert manager manifests
	manifest := fmt.Sprintf("---\n%s\n---\n%s\n", certManager, glooEdge)

	// create gateway controller workload definition
	workloadDefName := fmt.Sprintf("%s-%s", "gloo-edge", *kubernetesRuntimeInstance.Name)
	glooEdgeWorkloadDefinition := v0.WorkloadDefinition{
		Definition:   v0.Definition{Name: &workloadDefName},
		YAMLDocument: &manifest,
	}

	// create gateway controller workload definition
	createdWorkloadDef, err := client.CreateWorkloadDefinition(r.APIClient, r.APIServer, &glooEdgeWorkloadDefinition)
	if err != nil {
		return fmt.Errorf("failed to create gateway controller workload definition: %w", err)
	}

	// create gateway workload instance
	glooEdgeWorkloadInstance := v1.WorkloadInstance{
		Instance:                    v0.Instance{Name: &workloadDefName},
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		WorkloadDefinitionID:        createdWorkloadDef.ID,
	}
	createdGlooEdgeWorkloadInstance, err := client_v1.CreateWorkloadInstance(r.APIClient, r.APIServer, &glooEdgeWorkloadInstance)
	if err != nil {
		return fmt.Errorf("failed to create gateway controller workload instance: %w", err)
	}

	// update kubernetes runtime instance with gateway controller instance id
	kubernetesRuntimeInstance.GatewayControllerInstanceID = createdGlooEdgeWorkloadInstance.ID
	_, err = client.UpdateKubernetesRuntimeInstance(r.APIClient, r.APIServer, kubernetesRuntimeInstance)
	if err != nil {
		return fmt.Errorf("failed to update kubernetes runtime instance with gateway controller instance id: %w", err)
	}

	log.V(1).Info(
		"gloo edge deployed",
		"workloadInstanceID", glooEdgeWorkloadInstance.ID,
	)

	return nil
}

// confirmGatewayPortsExposed confirms whether the gateway controller has
// exposed the requested ports
func confirmGatewayPortsExposed(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	gatewayDefinition *v0.GatewayDefinition,
	log *logr.Logger,
) error {

	// get gateway controller workload resource instances
	if kubernetesRuntimeInstance.GatewayControllerInstanceID == nil {
		return fmt.Errorf("gateway controller instance ID is nil")
	}
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(r.APIClient, r.APIServer, *kubernetesRuntimeInstance.GatewayControllerInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get workload resource instances: %w", err)
	}

	// unmarshal gloo edge custom resource
	gateway, err := workloadutil.UnmarshalUniqueWorkloadResourceInstance(workloadResourceInstances, "GlooEdge")
	if err != nil {
		return fmt.Errorf("failed to unmarshal gloo edge workload resource instance: %w", err)
	}

	// get ports from gloo edge custom resource
	ports, found, err := unstructured.NestedSlice(gateway, "spec", "ports")
	if err != nil || !found {
		return fmt.Errorf("failed to get tcp ports from from gloo edge custom resource: %v", err)
	}

	// get gateway http and tcp ports
	gatewayHttpPorts, gatewayTcpPorts, err := client.GetGatewayHttpAndTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get gateway http ports: %w", err)
	}
	if len(*gatewayHttpPorts) == 0 && len(*gatewayTcpPorts) == 0 {
		return fmt.Errorf("no ports found")
	}

	// ensure http ports are exposed
	for _, httpPort := range *gatewayHttpPorts {
		ports, err = ensureGlooEdgePortExists("http", *httpPort.Port, *httpPort.TLSEnabled, ports, log)
		if err != nil {
			return fmt.Errorf("failed to ensure gloo edge port exists: %w", err)
		}
	}

	// ensure tcp ports are exposed
	for _, tcpPort := range *gatewayTcpPorts {
		ports, err = ensureGlooEdgePortExists("tcp", *tcpPort.Port, *tcpPort.TLSEnabled, ports, log)
		if err != nil {
			return fmt.Errorf("failed to ensure gloo edge port exists: %w", err)
		}
	}

	// set the ports slice on the gloo edge object
	err = unstructured.SetNestedSlice(gateway, ports, "spec", "ports")
	if err != nil {
		return fmt.Errorf("failed to set ports on gloo edge custom resource: %v", err)
	}

	jsonDefinition, err := util.MarshalJSON(gateway)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	// update the gloo edge workload resource object
	glooEdgeObject, err := workloadutil.GetUniqueWorkloadResourceInstance(workloadResourceInstances, "GlooEdge")
	if err != nil {
		return fmt.Errorf("failed to get gloo edge workload resource instance: %w", err)
	}
	glooEdgeObject.Reconciled = util.Ptr(false)
	glooEdgeObject.JSONDefinition = &jsonDefinition
	_, err = client.UpdateWorkloadResourceInstance(r.APIClient, r.APIServer, glooEdgeObject)
	if err != nil {
		return fmt.Errorf("failed to update gloo edge workload resource instance: %w", err)
	}

	// trigger a reconciliation of the gateway controller workload instance
	updatedGatewayControllerWorkloadInstance := v1.WorkloadInstance{
		Common:         v0.Common{ID: kubernetesRuntimeInstance.GatewayControllerInstanceID},
		Reconciliation: v0.Reconciliation{Reconciled: util.Ptr(false)},
	}
	_, err = client_v1.UpdateWorkloadInstance(r.APIClient, r.APIServer, &updatedGatewayControllerWorkloadInstance)
	if err != nil {
		return fmt.Errorf("failed to update gateway controller workload instance: %w", err)
	}

	// log the ports that are exposed
	gatewayPorts, err := client.GetGatewayPortsAsString(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get gateway ports as string: %w", err)
	}
	log.V(1).Info(
		"updated gateway controller instance to expose requested port",
		"ports", fmt.Sprintf("%s", gatewayPorts),
	)

	return nil

}

// ensureGlooEdgePortExists ensures a gloo edge port exists
// for a given protocol, port and tls state.
func ensureGlooEdgePortExists(protocol string, port int, tlsEnabled bool, ports []interface{}, log *logr.Logger) ([]interface{}, error) {

	// check existing gateways for requested ports
	for _, portSpec := range ports {
		spec := portSpec.(map[string]interface{})
		var portCurrent int64
		portCurrent, portNumberFound, err := util.NestedInt64OrFloat64(spec, "port")
		if err != nil {
			return nil, fmt.Errorf("failed to get port from from gloo edge custom resource: %v", err)
		}

		tlsEnabledCurrent, sslFound, err := unstructured.NestedBool(spec, "ssl")
		if err != nil {
			return nil, fmt.Errorf("failed to get ssl from from gloo edge custom resource: %v", err)
		}

		protocolCurrent, protocolFound, err := unstructured.NestedString(spec, "protocol")
		if err != nil {
			return nil, fmt.Errorf("failed to get protocol from from gloo edge custom resource: %v", err)
		}

		// check if current port matches requested port
		if portNumberFound && sslFound && protocolFound &&
			tlsEnabledCurrent == tlsEnabled &&
			int(portCurrent) == port &&
			protocolCurrent == protocol {
			log.V(1).Info(
				"port already exposed",
				"port", fmt.Sprintf("%d", port),
			)
			return ports, nil
		}
	}

	// otherwise, update gloo edge configuration

	// create a new gloo edge port object
	portNumber := int64(port)
	portString := strconv.Itoa(int(port))
	glooEdgePort := getGlooEdgePort(protocol, portString, portNumber, tlsEnabled)

	ports = append(ports, glooEdgePort.Object)

	return ports, nil
}

// configureGatewayManifests configures a VirtualService custom resource
// based on the configuration of the gateway workload definition
func configureGatewayManifests(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
) ([]*datatypes.JSON, error) {

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(r.APIClient, r.APIServer, *workloadInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances: %w", err)
	}

	// if a service name is provided, use it to get the service
	// otherwise, get the first service
	var service map[string]interface{}
	if gatewayDefinition.ServiceName != nil && *gatewayDefinition.ServiceName != "" {
		service, err = workloadutil.UnmarshalWorkloadResourceInstance(workloadResourceInstances, "Service", *gatewayDefinition.ServiceName)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal service workload resource instance: %w", err)
		}
	} else {
		service, err = workloadutil.UnmarshalUniqueWorkloadResourceInstance(workloadResourceInstances, "Service")
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal service workload resource instance: %w", err)
		}
	}

	// unmarshal service namespace
	namespace, found, err := unstructured.NestedString(service, "metadata", "namespace")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to unmarshal kubernetes service object's namespace field: %w", err)
	}

	// unmarshal service name
	name, found, err := unstructured.NestedString(service, "metadata", "name")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to unmarshal kubernetes service object's name field: %w", err)
	}

	// get gateway workload resource definitions
	gatewayWorkloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(r.APIClient, r.APIServer, *gatewayDefinition.WorkloadDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway workload resource definitions: %w", err)
	}

	// configure virtual service runtime parameters
	var jsonDefinitions []*datatypes.JSON
	virtualServiceManifests, err := configureVirtualServiceRuntimeParameters(
		r,
		gatewayDefinition,
		workloadInstance,
		kubernetesRuntimeInstance,
		gatewayWorkloadResourceDefinitions,
		namespace,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure virtual services: %w", err)
	}
	jsonDefinitions = append(jsonDefinitions, virtualServiceManifests...)

	// configure tcp gateway runtime parameters
	tcpGatewayManifests, err := configureTcpGatewayRuntimeParameters(
		r,
		gatewayDefinition,
		workloadInstance,
		kubernetesRuntimeInstance,
		gatewayWorkloadResourceDefinitions,
		namespace,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure virtual services: %w", err)
	}
	jsonDefinitions = append(jsonDefinitions, tcpGatewayManifests...)

	return jsonDefinitions, nil

}

// configureVirtualServiceRuntimeParameters configures runtime parameters
// for virtual services
func configureVirtualServiceRuntimeParameters(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	gatewayWorkloadResourceDefinitions *[]v0.WorkloadResourceDefinition,
	namespace,
	name string,
) ([]*datatypes.JSON, error) {

	// get domain info
	domain, _, err := getDomainInfo(r, gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain info: %w", err)
	}

	// get gateway http ports
	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}

	// configure virtual service runtime parameters
	var virtualServices []*datatypes.JSON
	for _, httpPort := range *gatewayHttpPorts {
		// unmarshal virtual service

		virtualService, err := workloadutil.UnmarshalUniqueWorkloadResourceDefinitionByName(
			gatewayWorkloadResourceDefinitions,
			"VirtualService",
			getVirtualServiceName(gatewayDefinition, domain, *httpPort.Port),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal virtual service workload resource definition: %w", err)
		}

		// if we're not redirecting HTTPS, set the upstream name
		// and namespace fields
		if !*httpPort.HTTPSRedirect {
			// get route array
			routes, found, err := unstructured.NestedSlice(virtualService, "spec", "virtualHost", "routes")
			if err != nil || !found {
				return nil, fmt.Errorf("failed to get virtualservice route: %w", err)
			}
			if len(routes) == 0 {
				return nil, fmt.Errorf("no routes found")
			}

			// set upstream port to 80 if tls is enabled
			// otherwise, use the port provided
			servicePort := *httpPort.Port
			if *httpPort.TLSEnabled {
				servicePort = 80
			}

			// set virtual service upstream name field
			err = unstructured.SetNestedField(
				routes[0].(map[string]interface{}),
				fmt.Sprintf("%s-%s-%d", namespace, name, servicePort), // $namespace-$name-$port is convention for gloo edge upstream names
				"routeAction",
				"single",
				"upstream",
				"name",
			)
			if err != nil {
				return nil, fmt.Errorf("failed to set upstream name on virtual service: %w", err)
			}

			// get gloo edge namespace
			glooEdgeNamespace, err := getGlooEdgeNamespace(r, kubernetesRuntimeInstance.GatewayControllerInstanceID)
			if err != nil {
				return nil, fmt.Errorf("failed to get gloo edge namespace: %w", err)
			}

			// set virtual service upstream namespace field
			err = unstructured.SetNestedField(
				routes[0].(map[string]interface{}),
				glooEdgeNamespace,
				"routeAction",
				"single",
				"upstream",
				"namespace",
			)
			if err != nil {
				return nil, fmt.Errorf("failed to set upstream name on virtual service: %w", err)
			}

			// set route field
			err = unstructured.SetNestedSlice(virtualService, routes, "spec", "virtualHost", "routes")
			if err != nil {
				return nil, fmt.Errorf("failed to set route on virtual service: %w", err)
			}
		}

		if *httpPort.TLSEnabled {

			// set secret ref namespace
			err = unstructured.SetNestedField(
				virtualService,
				namespace,
				"spec",
				"sslConfig",
				"secretRef",
				"namespace",
			)
			if err != nil {
				return nil, fmt.Errorf("failed to set secret ref name on virtual service: %w", err)
			}

		}

		// marshal virtual service
		virtualServiceMarshaled, err := util.MarshalJSON(virtualService)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal virtual service: %w", err)
		}
		virtualServices = append(virtualServices, &virtualServiceMarshaled)
	}

	return virtualServices, nil
}

// configureTcpGatewayRuntimeParameters configures runtime parameters
// for tcp gateways
func configureTcpGatewayRuntimeParameters(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	gatewayWorkloadResourceDefinitions *[]v0.WorkloadResourceDefinition,
	namespace,
	name string,
) ([]*datatypes.JSON, error) {

	// get tcp ports
	gatewayTcpPorts, err := client.GetGatewayTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway http ports: %w", err)
	}

	// configure tcp gateway runtime parameters
	var tcpGateways []*datatypes.JSON
	for _, tcpPort := range *gatewayTcpPorts {
		// unmarshal tcp gateway
		virtualService, err := workloadutil.UnmarshalUniqueWorkloadResourceDefinitionByName(
			gatewayWorkloadResourceDefinitions,
			"Gateway",
			fmt.Sprintf("%s-%d", *gatewayDefinition.Name, *tcpPort.Port),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal tcp gateway workload resource definition: %w", err)
		}

		// get tcp hosts array
		tcpHosts, found, err := unstructured.NestedSlice(virtualService, "spec", "tcpGateway", "tcpHosts")
		if err != nil || !found {
			return nil, fmt.Errorf("failed to get tcp gateway hosts: %w", err)
		}
		if len(tcpHosts) == 0 {
			return nil, fmt.Errorf("no tcp gateway hosts found")
		}

		// set tcp gateway upstream name field
		err = unstructured.SetNestedField(
			tcpHosts[0].(map[string]interface{}),
			fmt.Sprintf("%s-%s-%d", namespace, name, *tcpPort.Port), // $namespace-$name-$port is convention for gloo edge upstream names
			"destination",
			"single",
			"upstream",
			"name",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to set upstream name on tcp gateway: %w", err)
		}

		// get gloo edge namespace
		glooEdgeNamespace, err := getGlooEdgeNamespace(r, kubernetesRuntimeInstance.GatewayControllerInstanceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get gloo edge namespace: %w", err)
		}

		// set virtual service upstream namespace field
		err = unstructured.SetNestedField(
			tcpHosts[0].(map[string]interface{}),
			glooEdgeNamespace,
			"destination",
			"single",
			"upstream",
			"namespace",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to set upstream name on tcp gateway: %w", err)
		}

		// set tcp host field
		err = unstructured.SetNestedSlice(virtualService, tcpHosts, "spec", "tcpGateway", "tcpHosts")
		if err != nil {
			return nil, fmt.Errorf("failed to set host on tcp gateway: %w", err)
		}

		// TODO: configure ssl
		// if *tcpPort.TLSEnabled {
		// }

		// marshal virtual service
		tcpGatewayMarshaled, err := util.MarshalJSON(virtualService)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal virtual service: %w", err)
		}
		tcpGateways = append(tcpGateways, &tcpGatewayMarshaled)
	}

	return tcpGateways, nil
}

// getSubDomain returns the subdomain for a gateway definition and domain name definition.
func getSubDomain(gatewayDefinition *v0.GatewayDefinition, domainNameDefinition *v0.DomainNameDefinition) string {
	return fmt.Sprintf("%s.%s", *gatewayDefinition.SubDomain, *domainNameDefinition.Domain)
}

// configureIssuer configures an Issuer custom resource.
func configureIssuer(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	domainNameDefinition *v0.DomainNameDefinition,
) (*datatypes.JSON, error) {

	// get gateway workload resource definitions
	gatewayWorkloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(r.APIClient, r.APIServer, *gatewayDefinition.WorkloadDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway workload resource definitions: %w", err)
	}

	// unmarshal virtual service
	issuer, err := workloadutil.UnmarshalUniqueWorkloadResourceDefinition(gatewayWorkloadResourceDefinitions, "Issuer")
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal virtual service workload resource definition: %w", err)
	}

	// set issuer name
	kebabDomain := strcase.ToKebab(*domainNameDefinition.Name)
	err = unstructured.SetNestedField(issuer, kebabDomain, "metadata", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set name on issuer: %w", err)
	}

	// add domain to list of dns zones
	var dnsZones = []interface{}{
		*domainNameDefinition.Domain,
	}

	// if a subdomain is provided, append it to the list of dns zones
	if gatewayDefinition.SubDomain != nil && *gatewayDefinition.SubDomain != "" {
		dnsZones = append(dnsZones, getSubDomain(gatewayDefinition, domainNameDefinition))
	}

	// get kubernetes runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(r.APIClient, r.APIServer, *kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime definition by kubernetes runtime definition ID: %w", err)
	}

	// get infra provider region
	provider, err := runtime.GetCloudProviderForInfraProvider(*kubernetesRuntimeDefinition.InfraProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud provider for infra provider: %w", err)
	}
	infraProviderRegion, err := mapping.GetProviderRegionForLocation(provider, *kubernetesRuntimeInstance.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to get infra provider region for location: %w", err)
	}

	solver := map[string]interface{}{
		"selector": map[string]interface{}{
			"dnsZones": dnsZones,
		},
		"dns01": map[string]interface{}{
			"route53": map[string]interface{}{
				"region": infraProviderRegion,
			},
		},
	}

	// set solver
	solverList := []interface{}{solver}
	err = unstructured.SetNestedSlice(issuer, solverList, "spec", "acme", "solvers")
	if err != nil {
		return nil, fmt.Errorf("failed to set solvers on issuer: %w", err)
	}

	// marshal into json
	issuerMarshaled, err := util.MarshalJSON(issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal issuer: %w", err)
	}

	return &issuerMarshaled, nil
}

// configureCertificate configures a Certificate custom resource.
func configureCertificate(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	domainNameDefinition *v0.DomainNameDefinition,
) (*datatypes.JSON, error) {

	// get gateway workload resource definitions
	gatewayWorkloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(r.APIClient, r.APIServer, *gatewayDefinition.WorkloadDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway workload resource definitions: %w", err)
	}

	// unmarshal virtual service
	certificate, err := workloadutil.UnmarshalUniqueWorkloadResourceDefinition(gatewayWorkloadResourceDefinitions, "Certificate")
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal virtual service workload resource definition: %w", err)
	}

	// set certificate name
	kebabDomain := strcase.ToKebab(*domainNameDefinition.Name)
	err = unstructured.SetNestedField(certificate, kebabDomain, "metadata", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set name on issuer: %w", err)
	}

	dnsNames := []interface{}{}
	switch {
	case gatewayDefinition.SubDomain != nil && *gatewayDefinition.SubDomain != "":
		dnsNames = append(dnsNames, getSubDomain(gatewayDefinition, domainNameDefinition))
	case domainNameDefinition.Domain != nil:
		dnsNames = append(dnsNames, *domainNameDefinition.Domain)
	default:
		return nil, fmt.Errorf("failed to configure certificate, domain name definition domain is nil and no subdomain was provided")
	}

	// set dns names
	err = unstructured.SetNestedSlice(certificate, dnsNames, "spec", "dnsNames")
	if err != nil {
		return nil, fmt.Errorf("failed to set dns names on certificate: %w", err)
	}

	// set issuerRef name
	err = unstructured.SetNestedField(certificate, kebabDomain, "spec", "issuerRef", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set issuerRef on certificate: %w", err)
	}

	// marshal into json
	certificateMarshaled, err := util.MarshalJSON(certificate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal certificate: %w", err)
	}

	return &certificateMarshaled, nil
}

// getGatewayInstanceObjects returns the objects that should be created.
func getGatewayInstanceObjects(r *controller.Reconciler, gatewayInstance *v0.GatewayInstance) ([]string, error) {

	gatewayDefinition, err := client.GetGatewayDefinitionByID(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway definition: %w", err)
	}

	tlsEnabled, err := getTlsEnabled(r, gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get tls enabled: %w", err)
	}
	if tlsEnabled {
		return []string{"VirtualService", "Issuer", "Certificate"}, nil
	}

	return []string{"VirtualService"}, nil
}

// configureGatewayWorkloadResourceInstances configures the workload resource instances
// required for a gateway instance
func configureGatewayWorkloadResourceInstances(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v1.WorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
) (*[]v0.WorkloadResourceInstance, error) {

	var jsonManifests []*datatypes.JSON

	// get gloo edge virtual services and tcp gateways
	glooEdgeManifests, err := configureGatewayManifests(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to configure virtual service: %w", err)
	}
	jsonManifests = append(jsonManifests, glooEdgeManifests...)

	tlsEnabled, err := getTlsEnabled(r, gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get tls enabled: %w", err)
	}
	if tlsEnabled {

		if gatewayDefinition.DomainNameDefinitionID == nil {
			return nil, fmt.Errorf("failed to create certificate, domain name definition ID is nil")
		}

		// get domain name definition
		domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *gatewayDefinition.DomainNameDefinitionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get domain name definition: %w", err)
		}

		// configure issuerManifest manifest
		issuerManifest, err := configureIssuer(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance, domainNameDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to configure issuer: %w", err)
		}
		jsonManifests = append(jsonManifests, issuerManifest)

		// configure certificateManifest manifest
		certificateManifest, err := configureCertificate(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance, domainNameDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to configure certificate: %w", err)
		}
		jsonManifests = append(jsonManifests, certificateManifest)
	}

	var workloadResourceInstances []v0.WorkloadResourceInstance
	for _, jsonManifest := range jsonManifests {
		workloadResourceInstance := v0.WorkloadResourceInstance{
			JSONDefinition:     jsonManifest,
			WorkloadInstanceID: workloadInstance.ID,
			Reconciled:         util.Ptr(false),
		}
		workloadResourceInstances = append(workloadResourceInstances, workloadResourceInstance)
	}

	return &workloadResourceInstances, nil
}
