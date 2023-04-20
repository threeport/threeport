package reconcile

import (
	"errors"
	"fmt"

	"github.com/threeport/threeport/internal/kube"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WorkloadInstanceDeleted performs reconciliation when a workload instance
// has been deleted
func WorkloadInstanceDeleted(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
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
		*workloadInstance.WorkloadDefinitionID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource definitions by workload definition ID: %w", err)
	}
	if len(*workloadResourceDefinitions) == 0 {
		return errors.New("zero workload resource definitions to deploy")
	}

	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		*workloadInstance.ClusterInstanceID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload cluster instance by ID: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// delete each resource in the target kube cluster
	for _, wrd := range *workloadResourceDefinitions {
		// marshal the resource definition json
		jsonDefinition, err := wrd.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for workload resource definition with ID %d: %w", wrd.ID, err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object workload resource definition with ID %d: %w", wrd.ID, err)
		}

		// delete kube resource
		if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to delete Kubernetes resource workload resource definition with ID %d: %w", wrd.ID, err)
		}
	}

	return nil
}
