// generated by 'threeport-sdk gen' but will not be regenerated - intended for modification

package workload

import (
	"errors"
	"fmt"

	logr "github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	"gorm.io/datatypes"
)

// v0WorkloadDefinitionCreated performs reconciliation when a v0 workload definition
// has been created.
func v0WorkloadDefinitionCreated(
	r *controller.Reconciler,
	workloadDefinition *v0.WorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	// parse YAMLDocument and get kube objects in JSON
	jsonObjects, err := kube.GetJsonResourcesFromYamlDoc(*workloadDefinition.YAMLDocument)
	if err != nil {
		return 0, fmt.Errorf("failed to get JSON kube objects from YAML document: %w", err)
	}

	// create workload resource definition objects
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition
	for _, jsonContent := range jsonObjects {
		// unmarshal the json into the type used by API
		var jsonDefinition datatypes.JSON
		if err := jsonDefinition.UnmarshalJSON(jsonContent); err != nil {
			return 0, fmt.Errorf("failed to unmarshal json to datatypes.JSON: %w", err)
		}

		// build the workload resource definition and marshal to json
		workloadResourceDefinition := v0.WorkloadResourceDefinition{
			JSONDefinition:       &jsonDefinition,
			WorkloadDefinitionID: workloadDefinition.ID,
		}
		workloadResourceDefinitions = append(workloadResourceDefinitions, workloadResourceDefinition)
	}

	// create workload resource definitions in API
	createdWRDs, err := client.CreateWorkloadResourceDefinitions(
		r.APIClient,
		r.APIServer,
		&workloadResourceDefinitions,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create workload resource definitions in API: %w", err)
	}

	for _, wrd := range *createdWRDs {
		log.V(1).Info(
			"workload resource definition created",
			"workloadResourceDefinitionID", wrd.ID,
		)
	}

	return 0, nil
}

// v0WorkloadDefinitionUpdated performs reconciliation when a v0 workload definition
// has been updated.
func v0WorkloadDefinitionUpdated(
	r *controller.Reconciler,
	workloadDefinition *v0.WorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// v0WorkloadDefinitionDeleted performs reconciliation when a v0 workload definition
// has been deleted.
func v0WorkloadDefinitionDeleted(
	r *controller.Reconciler,
	workloadDefinition *v0.WorkloadDefinition,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if workloadDefinition.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if reconciled - it should not be, but if so we should do no
	// more
	if workloadDefinition.DeletionConfirmed != nil {
		return 0, nil
	}

	// get related workload resource definitions
	workloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
		r.APIClient,
		r.APIServer,
		*workloadDefinition.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload resource definitions by workload definition ID: %w", err)
	}

	// delete each related workload resource definition
	for _, wrd := range *workloadResourceDefinitions {
		_, err := client.DeleteWorkloadResourceDefinition(r.APIClient, r.APIServer, *wrd.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to delete workload resource definition with ID %d: %w", wrd.ID, err)
		}
		log.V(1).Info(
			"workload resource definition deleted",
			"workloadResourceDefinitionID", wrd.ID,
		)
	}

	return 0, nil
}
