package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
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

		workloadDef := v0.WorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: &glooEdgeString,
		}

		// create gateway workload definition
		createdWorkloadDef, err := client.CreateWorkloadDefinition(
			r.APIClient,
			r.APIServer,
			&workloadDef,
		)
		if err != nil {
			return fmt.Errorf("failed to get gateway controller instance: %w", err)
		}

		// create gateway workload instance
		workloadInst := v0.WorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadDefName,
			},
			ClusterInstanceID:    gatewayInstance.ClusterInstanceID,
			WorkloadDefinitionID: createdWorkloadDef.ID,
		}
		createdWorkloadInst, err := client.CreateWorkloadInstance(
			r.APIClient,
			r.APIServer,
			&workloadInst,
		)

		// update cluster instance with gateway controller instance id
		clusterInstance.GatewayControllerInstanceID = createdWorkloadInst.ID
		_, err = client.UpdateClusterInstance(
			r.APIClient,
			r.APIServer,
			clusterInstance,
		)
		if err != nil {
			return fmt.Errorf("failed to get gateway controller workload definition: %w", err)
		}

	}

	// confirm gateway controller instance is reconciled

	// // get gateway workload instance
	// gatewayControllerInstance, err := client.GetWorkloadInstanceByID(
	// 	r.APIClient,
	// 	r.APIServer,
	// 	*clusterInstance.GatewayControllerInstanceID,
	// )
	// if err != nil {
	// 	return fmt.Errorf("failed to get gateway controller instance: %w", err)
	// }

	// // get gateway workload definition
	// gatewayControllerWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
	// 	r.APIClient,
	// 	r.APIServer,
	// 	*gatewayControllerInstance.WorkloadDefinitionID,
	// )
	// if err != nil {
	// 	return fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	// }

	// // get gateway definition
	// gatewayWorkloadDefinition, err := client.GetGatewayDefinitionByID(
	// 	r.APIClient,
	// 	r.APIServer,
	// 	*gatewayInstance.GatewayDefinitionID,
	// )
	// if err != nil {
	// 	return fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	// }

	// var portFound = false
	// // check existing gateways for requested port
	// for _, wri := range gatewayControllerWorkloadDefinition.WorkloadResourceDefinitions {

	// 	// marshal the resource definition json
	// 	jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
	// 	if err != nil {
	// 		return fmt.Errorf("failed to marshal json for workload resource instance: %w", err)
	// 	}

	// 	// build kube unstructured object from json
	// 	kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
	// 	if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
	// 		return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
	// 	}

	// 	bindPort, found, err := unstructured.NestedInt64(kubeObject.Object, "spec", "bindPort")
	// 	bindPortInt32 := int32(bindPort)
	// 	if err == nil && found && bindPortInt32 == *gatewayWorkloadDefinition.TCPPort {
	// 		portFound = true
	// 		break
	// 	}
	// }

	// // create a gateway with the requested port
	// if !portFound {
	// }

	// ensure gateway definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmGatewayDefReconciled(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if gateway definition is reconciled: %w", err)
	}
	if !reconciled {
		return errors.New("gateway definition not reconciled")
	}

	// get gateway definition for this instance
	gatewayDefinition, err := client.GetGatewayDefinitionByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.GatewayDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway definition for the instance being deployed: %w", err)
	}

	// get a kube discovery client for the cluster
	discoveryClient, err := kube.GetDiscoveryClient(clusterInstance, true)

	// manipulate namespace on kube resources as needed
	processedWRIs, err := kube.SetNamespaces(
		&gatewayResourceInstances,
		gatewayInstance,
		discoveryClient,
	)
	if err != nil {
		return fmt.Errorf("failed to set namespaces for gateway resource instances: %w", err)
	}

	// create a dynamic client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		return fmt.Errorf("failed to create dynamic kube API client: %w", err)
	}

	// create each resource in the target kube cluster
	for _, wri := range *processedWRIs {
		// marshal the resource definition json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for gateway resource instance: %w", err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}

		// set label metadata on kube object
		kubeObject, err = kube.AddLabels(
			kubeObject,
			*gatewayDefinition.Name,
			gatewayInstance,
		)
		if err != nil {
			return fmt.Errorf("failed to add label metadata to objects: %w", err)
		}

		// create kube resource
		_, err = kube.CreateResource(kubeObject, dynamicKubeClient, *mapper)
		if err != nil {
			// add a GatewayEvent to surface the problem
			eventRuntimeUID := r.ControllerID.String()
			eventType := "Failed"
			eventReason := "CreateResourceError"
			eventMessage := fmt.Sprintf("failed to create Kubernetes resource for gateway instance: %s", err)
			timestamp := time.Now()
			createEvent := v0.GatewayEvent{
				RuntimeEventUID:   &eventRuntimeUID,
				Type:              &eventType,
				Reason:            &eventReason,
				Message:           &eventMessage,
				Timestamp:         &timestamp,
				GatewayInstanceID: gatewayInstance.ID,
			}
			_, err := client.CreateGatewayEvent(
				r.APIClient,
				r.APIServer,
				&createEvent,
			)
			if err != nil {
				log.Error(err, "failed to create gateway event for Kubernetes resource creation error")
			}
			return fmt.Errorf("failed to create Kubernetes resource: %w", err)
		}

		// create object in threeport API
		createdWRI, err := client.CreateGatewayResourceInstance(
			r.APIClient,
			r.APIServer,
			&wri,
		)
		if err != nil {
			return fmt.Errorf("failed to create gateway resource instance in threeport: %w", err)
		}

		agentWRI := agentapi.GatewayResourceInstance{
			Name:        kubeObject.GetName(),
			Namespace:   kubeObject.GetNamespace(),
			Group:       kubeObject.GroupVersionKind().Group,
			Version:     kubeObject.GroupVersionKind().Version,
			Kind:        kubeObject.GetKind(),
			ThreeportID: *createdWRI.ID,
		}
		threeportGateway.Spec.GatewayResourceInstances = append(
			threeportGateway.Spec.GatewayResourceInstances,
			agentWRI,
		)

		log.V(1).Info(
			"gateway resource instance created",
			"gatewayResourceInstanceID", wri.ID,
		)
	}

	// create the ThreeportGateway resource to inform the threeport-agent of
	// the resources that need to be watched
	resourceClient := dynamicKubeClient.Resource(agentapi.ThreeportGatewayGVR)
	unstructured, err := agentapi.UnstructuredThreeportGateway(&threeportGateway)
	if err != nil {
		return fmt.Errorf("failed to generate unstructured object for ThreeportGateway resource for creation in run time cluster")
	}
	_, err = resourceClient.Create(context.Background(), unstructured, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create new ThreeportGateway resource: %w", err)
	}

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
	gatewayResourceInstances, err := client.GetGatewayResourceInstancesByGatewayInstanceID(
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