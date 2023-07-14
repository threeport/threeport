package gateway

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/threeport/threeport/internal/kube"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// gatewayInstanceCreated performs reconciliation when a gateway instance
// has been created.
func gatewayInstanceCreated(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	log *logr.Logger,
) error {

	// initialize threeport object references
	clusterInstance, gatewayDefinition, workloadInstance, err := getThreeportObjects(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// validate threeport state before deploying virtual service
	err = validateThreeportState(r, gatewayDefinition, gatewayInstance, workloadInstance, clusterInstance)
	if err != nil {
		return fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// configure virtual service
	jsonManifest, err := configureVirtualService(r, gatewayDefinition, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to configure virtual service: %w", err)
	}

	// build the workload resource instance
	workloadResourceInstanceReconciled := false
	workloadResourceInstance := &v0.WorkloadResourceInstance{
		JSONDefinition:     jsonManifest,
		WorkloadInstanceID: workloadInstance.ID,
		Reconciled:         &workloadResourceInstanceReconciled,
	}

	// update the workload instance with the new workload resource instance
	_, err = client.CreateWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		workloadResourceInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to create workload resource instance: %w", err)
	}

	// trigger a reconciliation of the workload instance by setting Reconciled to false
	workloadInstance.Reconciled = &workloadResourceInstanceReconciled
	_, err = client.UpdateWorkloadInstance(
		r.APIClient,
		r.APIServer,
		workloadInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update workload instance: %w", err)
	}

	// update gateway instance
	gatewayInstanceReconciled := true
	gatewayInstance.WorkloadResourceInstanceID = workloadResourceInstance.ID
	gatewayInstance.Reconciled = &gatewayInstanceReconciled
	_, err = client.UpdateGatewayInstance(
		r.APIClient,
		r.APIServer,
		gatewayInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update gateway instance: %w", err)
	}

	log.V(1).Info(
		"gateway instance created",
		"gatewayResourceInstanceID", gatewayInstance.ID,
	)

	return nil
}

// gatewayInstanceDeleted performs reconciliation when a gateway instance
// has been updated
func gatewayInstanceUpdated(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	log *logr.Logger,
) error {

	// only reconcile if gateway definition needs to be reconciled
	if gatewayInstance.Reconciled != nil && !*gatewayInstance.Reconciled {
		return nil
	}

	// initialize threeport object references
	clusterInstance, gatewayDefinition, workloadInstance, err := getThreeportObjects(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// validate threeport state before deploying virtual service
	err = validateThreeportState(r, gatewayDefinition, gatewayInstance, workloadInstance, clusterInstance)
	if err != nil {
		return fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// configure virtual service
	jsonManifest, err := configureVirtualService(r, gatewayDefinition, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to configure virtual service: %w", err)
	}

	// get workload resource instance
	workloadResourceInstance, err := client.GetWorkloadResourceInstanceByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.WorkloadResourceInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get workload resource instance: %w", err)
	}

	// update the workload resource instance
	workloadResourceInstanceReconciled := false
	workloadResourceInstance.Reconciled = &workloadResourceInstanceReconciled
	workloadResourceInstance.JSONDefinition = jsonManifest
	_, err = client.UpdateWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		workloadResourceInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update workload resource instance: %w", err)
	}

	// trigger a reconciliation of the workload instance by setting Reconciled to false
	workloadInstance.Reconciled = &workloadResourceInstanceReconciled
	_, err = client.UpdateWorkloadInstance(
		r.APIClient,
		r.APIServer,
		workloadInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update workload instance: %w", err)
	}

	// update gateway instance
	gatewayInstanceReconciled := true
	gatewayInstance.WorkloadResourceInstanceID = workloadResourceInstance.ID
	gatewayInstance.Reconciled = &gatewayInstanceReconciled
	_, err = client.UpdateGatewayInstance(
		r.APIClient,
		r.APIServer,
		gatewayInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to update gateway instance: %w", err)
	}

	log.V(1).Info(
		"gateway instance updated",
		"gatewayResourceInstanceID", gatewayInstance.ID,
	)

	return nil
}

// gatewayInstanceDeleted performs reconciliation when a gateway instance
// has been deleted
func gatewayInstanceDeleted(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	log *logr.Logger,
) error {
	// get gateway resource instances
	workloadResourceInstance, err := client.GetWorkloadResourceInstanceByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.WorkloadResourceInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource instance by workload resource instance ID: %w", err)
	}

	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.ClusterInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway cluster instance by ID: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		return fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// delete gateway resource instance and resource in the target kube cluster

	// marshal the resource instance json
	jsonDefinition, err := workloadResourceInstance.JSONDefinition.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal json for gateway resource instance with ID %d: %w", workloadResourceInstance.ID, err)
	}

	// build kube unstructured object from json
	kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
		return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object gateway resource instance with ID %d: %w", workloadResourceInstance.ID, err)
	}

	// delete kube resource
	if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to delete Kubernetes resource gateway resource instance with ID %d: %w", workloadResourceInstance.ID, err)
	}

	// delete gateway resource instance in threeport API
	_, err = client.DeleteWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		*workloadResourceInstance.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete gateway resource instance with ID %d: %w", workloadResourceInstance.ID, err)
	}
	log.V(1).Info(
		"workload resource instance deleted",
		"workloadResourceInstanceID", workloadResourceInstance.ID,
	)

	// get workload instance
	workloadInstance, err := client.GetWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadResourceInstance.WorkloadInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload instance with ID %d: %w", *workloadResourceInstance.WorkloadInstanceID, err)
	}

	// update workload instance
	workloadInstanceReconciled := false
	workloadInstance.Reconciled = &workloadInstanceReconciled
	_, err = client.UpdateWorkloadInstance(
		r.APIClient,
		r.APIServer,
		workloadInstance,
	)
	if err != nil {
		return fmt.Errorf("failed to delete workload resource instance with ID %d: %w", workloadResourceInstance.ID, err)
	}

	log.V(1).Info(
		"gateway instance deleted",
		"gatewayResourceInstanceID", gatewayInstance.ID,
	)

	return nil
}

// TODO: refactor this into generic reconcile check function for definitions
// confirmGatewayDefReconciled confirms the gateway definition related to a
// gateway instance is reconciled.
func confirmGatewayDefReconciled(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
) (bool, error) {
	gatewayDefinition, err := client.GetGatewayDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.GatewayDefinitionID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to get gateway definition by gateway definition ID: %w", err)
	}
	if gatewayDefinition.Reconciled != nil && !*gatewayDefinition.Reconciled {
		return false, nil
	}

	return true, nil
}

// TODO: refactor this into generic reconcile check function for instances
// confirmGatewayDefReconciled confirms the gateway definition related to a
// gateway instance is reconciled.
func confirmGatewayControllerInstanceReconciled(
	r *controller.Reconciler,
	workloadInstanceID uint,
) (bool, error) {
	gatewayControllerInstance, err := client.GetWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		workloadInstanceID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to get gateway definition by gateway definition ID: %w", err)
	}
	if gatewayControllerInstance.Reconciled != nil && !*gatewayControllerInstance.Reconciled {
		return false, nil
	}

	return true, nil
}

// getUnstructuredObjectsFromWorkloadResourceInstances returns a list of
// unstructured kubernetes objects from a list of workload resource instances.
func getUnstructuredObjectsFromWorkloadResourceInstances(workloadInstances []*v0.WorkloadResourceInstance, kind string) ([]unstructured.Unstructured, error) {

	var objects []unstructured.Unstructured
	for _, wri := range workloadInstances {
		// marshal the resource definition json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json for workload resource instance: %w", err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}

		// search for service resource
		manifestKind, found, err := unstructured.NestedString(kubeObject.Object, "kind")
		if err != nil || found && manifestKind == kind {
			objects = append(objects, *kubeObject)
		}
	}

	return objects, nil
}

// confirmGatewayControllerDeployed confirms the gateway controller is deployed,
// and if not, deploys it.
func confirmGatewayControllerDeployed(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	clusterInstance *v0.ClusterInstance,
) error {

	// if cluster instance has no gateway controller, deploy one
	if clusterInstance.GatewayControllerInstanceID == nil {
		workloadDefName := "gloo-edge"

		glooEdgeBytes, err := yaml.Marshal(CreateGlooEdge())
		if err != nil {
			return fmt.Errorf("error marshaling to YAML: %v", err)
		}

		glooEdgeString := string(glooEdgeBytes)

		glooEdgeWorkloadDefinition := v0.WorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: &glooEdgeString,
		}

		// create gateway controller workload definition
		createdWorkloadDef, err := client.CreateWorkloadDefinition(
			r.APIClient,
			r.APIServer,
			&glooEdgeWorkloadDefinition,
		)
		if err != nil {
			return fmt.Errorf("failed to create gateway controller workload definition: %w", err)
		}

		// create gateway workload instance
		glooEdgeWorkloadInstance := v0.WorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadDefName,
			},
			ClusterInstanceID:    gatewayInstance.ClusterInstanceID,
			WorkloadDefinitionID: createdWorkloadDef.ID,
		}
		createdGlooEdgeWorkloadInstance, err := client.CreateWorkloadInstance(
			r.APIClient,
			r.APIServer,
			&glooEdgeWorkloadInstance,
		)
		if err != nil {
			return fmt.Errorf("failed to create gateway controller workload instance: %w", err)
		}

		// update cluster instance with gateway controller instance id
		clusterInstance.GatewayControllerInstanceID = createdGlooEdgeWorkloadInstance.ID
		_, err = client.UpdateClusterInstance(
			r.APIClient,
			r.APIServer,
			clusterInstance,
		)
		if err != nil {
			return fmt.Errorf("failed to update gateway controller workload instance: %w", err)
		}

	}
	return nil
}

// confirmGatewayPortExposed confirms whether the gateway controller has
// exposed the requested port
func confirmGatewayPortExposed(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	clusterInstance *v0.ClusterInstance,
	gatewayDefinition *v0.GatewayDefinition,
) error {

	// ensure gateway controller has requested port exposed

	// get gateway controller workload instance
	gatewayControllerInstance, err := client.GetWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		*clusterInstance.GatewayControllerInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway controller instance: %w", err)
	}

	// check existing gateways for requested port
	var portFound = false

	gatewayObjects, err := getUnstructuredObjectsFromWorkloadResourceInstances(gatewayControllerInstance.WorkloadResourceInstances, "Gateway")
	if err != nil {
		return fmt.Errorf("failed to get service objects from workload instance: %v", err)
	}
	gatewayObject := gatewayObjects[0]

	bindPorts, found, err := unstructured.NestedSlice(gatewayObject.Object, "spec", "tcpPorts")
	if err != nil {
		return fmt.Errorf("failed to get tcp ports from from gloo edge custom resource: %v", err)
	}
	for _, bindPort := range bindPorts {
		bindPortInt32 := bindPort.(int32)
		if found && bindPortInt32 == *gatewayDefinition.TCPPort {
			portFound = true
			break
		}
	}

	// TODO: if port not found, update gateway controller workload definition with the requested port
	if !portFound {
		return errors.New("gateway controller instance does not have requested port exposed")
	}

	return nil
}

// configureVirtualService configures a VirtualService custom resource
// based on the configuration of the gateway workload definition
func configureVirtualService(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.WorkloadInstance,
) (*datatypes.JSON, error) {

	// get gateway workload definition
	gatewayWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.WorkloadDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway workload definition: %w", err)
	}

	var virtualService map[string]interface{}
	err = yaml.Unmarshal([]byte(*gatewayWorkloadDefinition.YAMLDocument), &virtualService)
	if err != nil {
		return nil, fmt.Errorf("error parsing YAML: %v", err)
	}

	serviceObjects, err := getUnstructuredObjectsFromWorkloadResourceInstances(workloadInstance.WorkloadResourceInstances, "Service")
	if err != nil {
		return nil, fmt.Errorf("failed to get service objects from workload instance: %v", err)
	}
	serviceObject := serviceObjects[0]

	// // TODO: handle multiple services
	// if len(serviceObjects) > 1 {
	// }

	// unmarshal service namespace
	namespace, found, err := unstructured.NestedString(serviceObject.Object, "metadata", "namespace")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to unmarshal kubernetes service object's namespace field: %w", err)
	}

	// unmarshal service name
	name, found, err := unstructured.NestedString(serviceObject.Object, "metadata", "name")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to unmarshal kubernetes service object's name field: %w", err)
	}

	// get route array object
	routes, found, err := unstructured.NestedSlice(virtualService, "spec", "virtualHost", "routes")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to get virtualservice route: %w", err)
	}

	// set virtual service upstream field
	err = unstructured.SetNestedField(routes[0].(map[string]interface{}), fmt.Sprintf("%s-%s", namespace, name), "routeAction", "single", "upstream", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set upstream name on virtual service: %w", err)
	}

	jsonBytes, err := json.Marshal(virtualService)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json to datatypes.JSON: %w", err)
	}

	jsonManifest := datatypes.JSON(jsonBytes)

	return &jsonManifest, nil

}

// getThreeportobjects returns all threeport objects required for
// gateway instance reconciliation
func getThreeportObjects(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
) (*v0.ClusterInstance, *v0.GatewayDefinition, *v0.WorkloadInstance, error) {
	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.ClusterInstanceID,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get gateway cluster instance by ID: %w", err)
	}

	// get gateway definition
	gatewayDefinition, err := client.GetGatewayDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.GatewayDefinitionID,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	}

	// get workload instance that we're configuring this gateway instance for
	workloadInstance, err := client.GetWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.WorkloadDefinitionID,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get workload instance: %w", err)
	}

	return clusterInstance, gatewayDefinition, workloadInstance, nil
}

// validateThreeportState validates the state of the threeport API
// prior to reconciling a gateway instance
func validateThreeportState(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	gatewayInstance *v0.GatewayInstance,
	workloadInstance *v0.WorkloadInstance,
	clusterInstance *v0.ClusterInstance,
) error {

	// ensure gateway definition is reconciled before working on an instance
	// for it
	gatewayDefinitionReconciled, err := confirmGatewayDefReconciled(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if gateway definition is reconciled: %w", err)
	}
	if !gatewayDefinitionReconciled {
		return errors.New("gateway definition not reconciled")
	}
	err = confirmGatewayControllerDeployed(r, gatewayInstance, clusterInstance)
	if err != nil {
		return fmt.Errorf("failed to reconcile gateway controller: %w", err)
	}

	// ensure gateway controller instance is reconciled before working on
	// a gateway instance for it
	gatewayControllerInstanceReconciled, err := confirmGatewayControllerInstanceReconciled(r, *clusterInstance.GatewayControllerInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if gateway controller instance is reconciled: %w", err)
	}
	if !gatewayControllerInstanceReconciled {
		return errors.New("gateway controller instance not reconciled")
	}

	// confirm requested port exposed
	err = confirmGatewayPortExposed(r, gatewayInstance, clusterInstance, gatewayDefinition)
	if err != nil {
		return fmt.Errorf("failed to confirm requested port is exposed: %w", err)
	}

	return nil
}
