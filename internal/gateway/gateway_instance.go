package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/kube"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
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

	// ensure gateway definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmGatewayDefReconciled(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if gateway definition is reconciled: %w", err)
	}
	if !reconciled {
		return errors.New("gateway definition not reconciled")
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

	// if cluster instance has no gateway controller, deploy one
	if clusterInstance.GatewayControllerInstanceID == nil {
		workloadDefName := "gloo-edge"

		glooEdgeBytes, err := yaml.Marshal(CreateGlooEdge())
		if err != nil {
			return fmt.Errorf("Error marshaling to YAML: %v", err)
		}

		glooEdgeString := string(glooEdgeBytes)

		glooEdgeWorkloadDefinition := v0.WorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: &glooEdgeString,
		}

		// create gateway workload definition
		createdWorkloadDef, err := client.CreateWorkloadDefinition(
			r.APIClient,
			r.APIServer,
			&glooEdgeWorkloadDefinition,
		)
		if err != nil {
			return fmt.Errorf("failed to get gateway controller instance: %w", err)
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

		return errors.New("failed to deploy gateway instance, no gateway controller instance found. Deploying gateway controller instance")

	} else {

		// ensure gateway controller instance is reconciled before working on
		// a gateway instance for it
		reconciled, err := confirmGatewayControllerInstanceReconciled(r, *clusterInstance.GatewayControllerInstanceID)
		if err != nil {
			return fmt.Errorf("failed to determine if gateway controller instance is reconciled: %w", err)
		}
		if !reconciled {
			return errors.New("gateway controller instance not reconciled")
		}

	}

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

	// get gateway controller workload definition
	gatewayControllerWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayControllerInstance.WorkloadDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	}

	// get gateway definition
	gatewayWorkloadDefinition, err := client.GetGatewayDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.GatewayDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	}

	var portFound = false
	// check existing gateways for requested port
	for _, wri := range gatewayControllerWorkloadDefinition.WorkloadResourceDefinitions {

		// marshal the resource definition json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for workload resource instance: %w", err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}

		bindPorts, found, err := unstructured.NestedSlice(kubeObject.Object, "spec", "tcpPorts")
		for _, bindPort := range bindPorts {
			bindPortInt32 := bindPort.(int32)
			if err == nil && found && bindPortInt32 == *gatewayWorkloadDefinition.TCPPort {
				portFound = true
				break
			}
		}
	}

	// create a gateway with the requested port
	if !portFound {
		//TODO: create gateway with requested port if not found
		return errors.New("gateway controller instance does not have requested port exposed")
	}

	// update parent workload definition with virtual service to expose requested port

	// get parent workload definition that we're configuring this gateway for
	parentWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayWorkloadDefinition.WorkloadDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get parent workload definition: %w", err)
	}

	for _, wri := range parentWorkloadDefinition.WorkloadResourceDefinitions {
		// marshal the resource definition json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for workload resource instance: %w", err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}

		bindPort, found, err := unstructured.NestedInt64(kubeObject.Object, "spec", "bindPort")
		bindPortInt32 := int32(bindPort)
		if err == nil && found && bindPortInt32 == *gatewayWorkloadDefinition.TCPPort {
			return errors.New("virtual service already exists for requested port")
		}
	}

	virtualServiceBytes, err := json.Marshal(CreateVirtualService())
	if err != nil {
		return fmt.Errorf("Error marshaling to JSON: %v", err)
	}

	// unmarshal the json into the type used by API
	var jsonDefinition datatypes.JSON
	if err := jsonDefinition.UnmarshalJSON(virtualServiceBytes); err != nil {
		return fmt.Errorf("failed to unmarshal json to datatypes.JSON: %w", err)
	}

	// build the workload resource definition and marshal to json
	workloadResourceDefinition := &v0.WorkloadResourceDefinition{
		JSONDefinition:       &jsonDefinition,
		WorkloadDefinitionID: gatewayControllerWorkloadDefinition.ID,
	}

	// update the parent workload definition with the new workload resource definition
	client.CreateWorkloadResourceDefinition(
		r.APIClient,
		r.APIServer,
		workloadResourceDefinition,
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
	gatewayResourceInstances, err := client.GetGatewayResourceInstancesByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway resource instances by gateway instance ID: %w", err)
	}
	if len(*gatewayResourceInstances) == 0 {
		return errors.New("zero gateway resource instances to delete")
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
		fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// delete each gateway resource instance and resource in the target kube cluster
	for _, wri := range *gatewayResourceInstances {
		// marshal the resource instance json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for gateway resource instance with ID %d: %w", wri.ID, err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object gateway resource instance with ID %d: %w", wri.ID, err)
		}

		// delete kube resource
		if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to delete Kubernetes resource gateway resource instance with ID %d: %w", wri.ID, err)
		}

		// delete each gateway resource instance in threeport API
		_, err = client.DeleteGatewayResourceInstance(r.APIClient, r.APIServer, *wri.ID)
		if err != nil {
			return fmt.Errorf("failed to delete gateway resource instance with ID %d: %w", wri.ID, err)
		}
		log.V(1).Info(
			"gateway resource instance deleted",
			"gatewayResourceInstanceID", wri.ID,
		)
	}

	// delete gateway events related to gateway instance
	_, err = client.DeleteGatewayEventsByGatewayInstanceID(r.APIClient, r.APIServer, *gatewayInstance.ID)
	if err != nil {
		return fmt.Errorf("failed to delete gateway events for gateway instance with ID %d: %w", gatewayInstance.ID, err)
	}
	log.V(1).Info(
		"gateway events deleted",
		"gatewayInstanceID", gatewayInstance.ID,
	)

	// delete the ThreeportGateway resource to inform the threeport-agent the
	// resources are gone
	resourceClient := dynamicKubeClient.Resource(agentapi.ThreeportGatewayGVR)
	if err = resourceClient.Delete(
		context.Background(),
		agent.ThreeportGatewayName(*gatewayInstance.ID),
		metav1.DeleteOptions{},
	); err != nil {
		return fmt.Errorf("failed to delete new ThreeportGateway resource: %w", err)
	}

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
	if gatewayDefinition.Reconciled != nil && *gatewayDefinition.Reconciled != true {
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
	if gatewayControllerInstance.Reconciled != nil && *gatewayControllerInstance.Reconciled != true {
		return false, nil
	}

	return true, nil
}
