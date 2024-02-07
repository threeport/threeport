package workload

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/agent"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// workloadInstanceCreated performs reconciliation when a workload instance
// has been created.
func workloadInstanceCreated(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// ensure workload definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmWorkloadDefReconciled(r, workloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to determine if workload definition is reconciled: %w", err)
	}
	if !reconciled {
		return 0, errors.New("workload definition not reconciled")
	}

	// use workload definition ID to get workload resource definitions
	workloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.WorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload resource definitions by workload definition ID: %w", err)
	}
	if len(*workloadResourceDefinitions) == 0 {
		return 0, errors.New("zero workload resource definitions to deploy")
	}

	// get workload definition for this instance
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.WorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload definition for the instance being deployed: %w", err)
	}

	// construct ThreeportWorkload resource to inform the threeport-agent of
	// which resources it should watch
	threeportWorkloadName, err := agent.ThreeportWorkloadName(
		*workloadInstance.ID,
		agent.WorkloadInstanceType,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to generate threeport workload resource name: %w", err)
	}
	threeportWorkload := agentapi.ThreeportWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name: threeportWorkloadName,
		},
		Spec: agentapi.ThreeportWorkloadSpec{
			WorkloadType:       agent.WorkloadInstanceType,
			WorkloadInstanceID: *workloadInstance.ID,
		},
	}

	// construct workload resource instances
	var workloadResourceInstances []v0.WorkloadResourceInstance
	for _, wrd := range *workloadResourceDefinitions {
		wri := v0.WorkloadResourceInstance{
			JSONDefinition:     wrd.JSONDefinition,
			WorkloadInstanceID: workloadInstance.ID,
		}
		workloadResourceInstances = append(workloadResourceInstances, wri)
	}

	// get kubernetes runtime instance info
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload kubernetesRuntime instance by ID: %w", err)
	}

	// get a kube discovery client for the kubernetes runtime
	discoveryClient, err := kube.GetDiscoveryClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get kubernetes API discovery client for kubernetes runtime instance: %w", err)
	}

	// manipulate namespace on kube resources as needed
	processedWRIs, err := kube.SetNamespaces(
		&workloadResourceInstances,
		workloadInstance,
		discoveryClient,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set namespaces for workload resource instances: %w", err)
	}

	// create a dynamic client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create dynamic kube API client: %w", err)
	}

	// create each resource in the target kubernetes runtime instance
	for _, wri := range *processedWRIs {
		// marshal the resource definition json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return 0, fmt.Errorf("failed to marshal json for workload resource instance: %w", err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return 0, fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}

		// set label metadata on kube object to signal threeport agent
		kubeObject, err = kube.AddLabels(
			kubeObject,
			*workloadDefinition.Name,
			*workloadInstance.Name,
			*workloadInstance.ID,
			agent.WorkloadInstanceLabelKey,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to add label metadata to objects: %w", err)
		}

		// create kube resource
		_, err = kube.CreateResource(kubeObject, dynamicKubeClient, *mapper)
		if err != nil {
			// add a WorkloadEvent to surface the problem
			eventRuntimeUID := r.ControllerID.String()
			eventType := "Failed"
			eventReason := "CreateResourceError"
			eventMessage := fmt.Sprintf("failed to create Kubernetes resource for workload instance: %s", err)
			timestamp := time.Now()
			createEvent := v0.WorkloadEvent{
				RuntimeEventUID:    &eventRuntimeUID,
				Type:               &eventType,
				Reason:             &eventReason,
				Message:            &eventMessage,
				Timestamp:          &timestamp,
				WorkloadInstanceID: workloadInstance.ID,
			}
			_, eventErr := client.CreateWorkloadEvent(
				r.APIClient,
				r.APIServer,
				&createEvent,
			)
			if eventErr != nil {
				log.Error(err, "failed to create workload event for Kubernetes resource creation error")
			}
			return 0, fmt.Errorf("failed to create Kubernetes resource: %w", err)
		}

		// create object in threeport API
		reconciled := true
		wri.Reconciled = &reconciled
		createdWRI, err := client.CreateWorkloadResourceInstance(
			r.APIClient,
			r.APIServer,
			&wri,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to create workload resource instance in threeport: %w", err)
		}

		agentWRI := agentapi.WorkloadResourceInstance{
			Name:        kubeObject.GetName(),
			Namespace:   kubeObject.GetNamespace(),
			Group:       kubeObject.GroupVersionKind().Group,
			Version:     kubeObject.GroupVersionKind().Version,
			Kind:        kubeObject.GetKind(),
			ThreeportID: *createdWRI.ID,
		}
		threeportWorkload.Spec.WorkloadResourceInstances = append(
			threeportWorkload.Spec.WorkloadResourceInstances,
			agentWRI,
		)

		log.V(1).Info(
			"workload resource instance created",
			"workloadResourceInstanceID", wri.ID,
		)
	}

	// create the ThreeportWorkload resource to inform the threeport-agent of
	// the resources that need to be watched
	resourceClient := dynamicKubeClient.Resource(agentapi.ThreeportWorkloadGVR)
	unstructured, err := agentapi.UnstructuredThreeportWorkload(&threeportWorkload)
	if err != nil {
		return 0, fmt.Errorf("failed to generate unstructured object for ThreeportWorkload resource for creation in runtime kubernetes runtime")
	}
	_, err = resourceClient.Create(context.Background(), unstructured, metav1.CreateOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to create new ThreeportWorkload resource: %w", err)
	}

	return 0, nil
}

// workloadInstanceUpdated performs reconciliation when a workload instance
// has been updated
func workloadInstanceUpdated(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// get kubernetes runtime instance info
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload kubernetes runtime instance by ID: %w", err)
	}

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload resource instances by workload instance ID: %w", err)
	}
	if len(*workloadResourceInstances) == 0 {
		log.V(1).Info(
			"zero workload resource instances to update",
			"workloadInstanceID", workloadInstance.ID,
		)
		return 0, nil
	}

	// get a kube discovery client for the cluster
	discoveryClient, err := kube.GetDiscoveryClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get kube discovery client for cluster: %w", err)
	}

	// manipulate namespace on kube resources as needed
	processedWRIs, err := kube.SetNamespaces(
		workloadResourceInstances,
		workloadInstance,
		discoveryClient,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set namespaces for workload resource instances: %w", err)
	}

	// return if namespaceWRI hasn't been created yet
	namespaceWRI := (*processedWRIs)[0]
	if namespaceWRI.ID == nil {
		return 0, fmt.Errorf("namespace not created yet")
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// update each workload resource instance and resource in the target kube cluster
	for _, wri := range *processedWRIs {

		// only update resource instances that have not been reconciled
		if *wri.Reconciled {
			continue
		}

		// marshal the resource instance json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return 0, fmt.Errorf("failed to marshal json for workload resource instance with ID %d: %w", wri.ID, err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return 0, fmt.Errorf("failed to unmarshal json to kubernetes unstructured object workload resource instance with ID %d: %w", wri.ID, err)
		}

		// if the resource instance is scheduled for deletion, delete it
		if wri.ScheduledForDeletion != nil {

			// delete kube resource
			if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
				return 0, fmt.Errorf("failed to delete Kubernetes resource workload resource instance with ID %d: %w", wri.ID, err)
			}

			// delete threeport resource
			_, err = client.DeleteWorkloadResourceInstance(
				r.APIClient,
				r.APIServer,
				*wri.ID,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to delete workload resource instance with ID %d: %w", wri.ID, err)
			}
			continue

		} else {
			// otherwise, it needs to be created or updated
			if _, err := kube.CreateOrUpdateResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
				return 0, fmt.Errorf("failed to create or update Kubernetes resource workload resource instance with ID %d: %w", wri.ID, err)
			}
		}

		// update the workload resource instance
		reconciled := true
		wri.Reconciled = &reconciled
		_, err = client.UpdateWorkloadResourceInstance(
			r.APIClient,
			r.APIServer,
			&wri,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to update workload resource instance with ID %d: %w", wri.ID, err)
		}

		log.V(1).Info(
			"workload resource instance updated",
			"workloadResourceInstanceID", wri.ID,
		)
	}

	return 0, nil
}

// workloadInstanceDeleted performs reconciliation when a workload instance
// has been deleted
func workloadInstanceDeleted(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if workloadInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if workloadInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload resource instances by workload instance ID: %w", err)
	}
	if len(*workloadResourceInstances) == 0 {
		// no workload resource instances to clean up
		return 0, nil
	}

	// get kubernetes runtime instance info
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		log.Error(
			errors.New("failed to get kubernetes runtime instance by ID"),
			"kubernetesRuntimeInstance", *workloadInstance.KubernetesRuntimeInstanceID,
		)
		return 0, fmt.Errorf("failed to get kubernetes runtime instance by ID: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// We need to query the Threeport API for the attached object references
	// even though the WorkloadInstance table has a relation to AttachedObjectReferences.
	// This is because the AttachedObjectReferences relation is deleted when the
	// WorkloadInstance is deleted, so we can't use those references anymore in
	// the deletion handler.
	attachedObjectReferences, err := client.GetAttachedObjectReferencesByObjectID(r.APIClient, r.APIServer, *workloadInstance.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to get attached object references by workload instance ID: %w", err)
	}
	for _, object := range *attachedObjectReferences {
		err := client.DeleteObjectByTypeAndID(r.APIClient, r.APIServer, *object.AttachedObjectType, *object.AttachedObjectID)
		if err != nil {
			switch {
			case errors.Is(err, client.ErrObjectNotFound):
				log.Info("attached object has already been deleted", "objectID", *object.AttachedObjectID)
			case errors.Is(err, client.ErrConflict):
				log.Info("attached object is already being deleted", "objectID", *object.AttachedObjectID)
			default:
				return 0, fmt.Errorf("failed to delete object by type %s and ID %d: %w", *object.AttachedObjectType, *object.ID, err)
			}
		}
		_, err = client.DeleteAttachedObjectReference(r.APIClient, r.APIServer, *object.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to delete attached object reference with ID %d: %w", *object.ID, err)
		}
	}

	// delete each workload resource instance and resource in the target kubernetes runtime instance
	for _, wri := range *workloadResourceInstances {
		// marshal the resource instance json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return 0, fmt.Errorf("failed to marshal json for workload resource instance with ID %d: %w", wri.ID, err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return 0, fmt.Errorf("failed to unmarshal json to kubernetes unstructured object workload resource instance with ID %d: %w", wri.ID, err)
		}

		// delete kube resource
		if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
			return 0, fmt.Errorf("failed to delete Kubernetes resource workload resource instance with ID %d: %w", wri.ID, err)
		}

		// delete each workload resource instance in threeport API
		_, err = client.DeleteWorkloadResourceInstance(r.APIClient, r.APIServer, *wri.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to delete workload resource instance with ID %d: %w", wri.ID, err)
		}
		log.V(1).Info(
			"workload resource instance deleted",
			"workloadResourceInstanceID", wri.ID,
		)
	}

	// delete workload events related to workload instance
	_, err = client.DeleteWorkloadEventsByQueryString(
		r.APIClient,
		r.APIServer,
		fmt.Sprintf("workloadinstanceid=%d", *workloadInstance.ID),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete workload events for workload instance with ID %d: %w", workloadInstance.ID, err)
	}
	log.V(1).Info(
		"workload events deleted",
		"workloadInstanceID", workloadInstance.ID,
	)

	// delete the ThreeportWorkload resource to inform the threeport-agent the
	// resources are gone
	resourceClient := dynamicKubeClient.Resource(agentapi.ThreeportWorkloadGVR)
	threeportWorkloadName, err := agent.ThreeportWorkloadName(
		*workloadInstance.ID,
		agent.WorkloadInstanceType,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to determine threeport workload resource name: %w", err)
	}
	if err = resourceClient.Delete(
		context.Background(),
		threeportWorkloadName,
		metav1.DeleteOptions{},
	); err != nil && !kubeerr.IsNotFound(err) {
		return 0, fmt.Errorf("failed to delete ThreeportWorkload resource: %w", err)
	}

	return 0, nil
}

// confirmWorkloadDefReconciled confirms the workload definition related to a
// workload instance is reconciled.
func confirmWorkloadDefReconciled(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
) (bool, error) {
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.WorkloadDefinitionID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}
	if workloadDefinition.Reconciled != nil && !*workloadDefinition.Reconciled {
		return false, nil
	}

	return true, nil
}
