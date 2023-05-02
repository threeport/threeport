package workload

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/kube"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
)

// workloadInstanceCreated performs reconciliation when a workload instance
// has been created.
func workloadInstanceCreated(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
	log *logr.Logger,
) error {
	// ensure workload definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmWorkloadDefReconciled(r, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if workload definition is reconciled: %w", err)
	}
	if !reconciled {
		return errors.New("workload definition not reconciled")
	}

	// use workload definition ID to get workload resource definitions
	workloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
		r.APIClient,
		*workloadInstance.WorkloadDefinitionID,
		r.APIServer,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource definitions by workload definition ID: %w", err)
	}
	if len(*workloadResourceDefinitions) == 0 {
		return errors.New("zero workload resource definitions to deploy")
	}

	// get workload definition for this instance
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.WorkloadDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload definition for the instance being deployed: %w", err)
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

	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.ClusterInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload cluster instance by ID: %w", err)
	}

	// get a kube discovery client for the cluster
	discoveryClient, err := kube.GetDiscoveryClient(clusterInstance, true)

	// manipulate namespace on kube resources as needed
	processedWRIs, err := kube.SetNamespaces(
		&workloadResourceInstances,
		workloadInstance,
		discoveryClient,
	)
	if err != nil {
		return fmt.Errorf("failed to set namespaces for workload resource instances: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// create each resource in the target kube cluster
	for _, wri := range *processedWRIs {
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

		// set label metadata on kube object
		kubeObject, err = kube.SetLabels(
			kubeObject,
			*workloadDefinition.Name,
			*workloadInstance.Name,
		)
		if err != nil {
			return fmt.Errorf("failed to add label metadata to objects: %w", err)
		}

		// create kube resource
		_, err = kube.CreateResource(kubeObject, dynamicKubeClient, *mapper)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes resource: %w", err)
		}

		// create object in threeport API
		_, err = client.CreateWorkloadResourceInstance(
			r.APIClient,
			r.APIServer,
			&wri,
		)
		if err != nil {
			return fmt.Errorf("failed to create workload resource instance in threeport: %w", err)
		}

		log.V(1).Info(
			"workload resource instance created",
			"workloadResourceInstanceID", wri.ID,
		)
	}

	return nil
}

// workloadInstanceDeleted performs reconciliation when a workload instance
// has been deleted
func workloadInstanceDeleted(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
	log *logr.Logger,
) error {
	// ensure workload definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmWorkloadDefReconciled(r, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if workload definition is reconciled: %w", err)
	}
	if !reconciled {
		return errors.New("workload definition not reconciled")
	}

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		r.APIClient,
		*workloadInstance.ID,
		r.APIServer,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource instances by workload instance ID: %w", err)
	}
	if len(*workloadResourceInstances) == 0 {
		return errors.New("zero workload resource instances to delete")
	}

	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		r.APIClient,
		r.APIServer,
		*workloadInstance.ClusterInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get workload cluster instance by ID: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// delete each workload resource instance and resource in the target kube cluster
	for _, wri := range *workloadResourceInstances {
		// marshal the resource instance json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for workload resource instance with ID %d: %w", wri.ID, err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object workload resource instance with ID %d: %w", wri.ID, err)
		}

		// delete kube resource
		if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to delete Kubernetes resource workload resource instance with ID %d: %w", wri.ID, err)
		}

		// delete each workload resource instance in threeport API
		_, err = client.DeleteWorkloadResourceInstance(r.APIClient, r.APIServer, *wri.ID)
		if err != nil {
			return fmt.Errorf("failed to delete workload resource instance with ID %d: %w", wri.ID, err)
		}
		log.V(1).Info(
			"workload resource instance deleted",
			"workloadResourceInstanceID", wri.ID,
		)
	}

	return nil
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
	if workloadDefinition.Reconciled != nil && *workloadDefinition.Reconciled != true {
		return false, nil
	}

	return true, nil
}
