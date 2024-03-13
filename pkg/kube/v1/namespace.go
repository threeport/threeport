package v1

import (
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"
	"k8s.io/client-go/discovery"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube_v0 "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// SetNamespaces adds the namespace resource and namespace assignment as needed
// to an array of workload resource instances.
func SetNamespaces(
	workloadResourceInstances *[]v0.WorkloadResourceInstance,
	workloadInstanceName *string,
	workloadInstanceID *uint,
	discoveryClient *discovery.DiscoveryClient,
) (*[]v0.WorkloadResourceInstance, error) {
	// first check to see if any namespaces are included - if so assume
	// namespaces are managed by client and do nothing
	clientManagedNS := ""
	for _, wri := range *workloadResourceInstances {
		var mapDef map[string]interface{}
		err := json.Unmarshal([]byte(*wri.JSONDefinition), &mapDef)
		if err != nil {
			return workloadResourceInstances, fmt.Errorf("failed to unmarshal json: %w", err)
		}
		if mapDef["kind"] == "Namespace" {
			metadata := mapDef["metadata"].(map[string]interface{})
			clientManagedNS = metadata["name"].(string)
			break
		}
	}

	namespace := ""
	if clientManagedNS == "" {
		// we are managing namespaces for the client - create namespace and add to
		// array of processed workload resource instances
		namespace = fmt.Sprintf("%s-%s", *workloadInstanceName, util.RandomAlphaNumericString(10))
	} else {
		namespace = clientManagedNS
	}

	processedWRIs := []v0.WorkloadResourceInstance{}
	namespacedObjectCount := 0
	for _, wri := range *workloadResourceInstances {
		// check to see if this is a namespaced resource
		namespaced, err := kube_v0.IsNamespaced(
			string(*wri.JSONDefinition),
			discoveryClient,
		)
		if err != nil {
			return &processedWRIs, fmt.Errorf("failed to determine if workload resource instance is namespaced: %w", err)
		}
		if !namespaced {
			// skip non-namespaced resources
			processedWRIs = append(processedWRIs, wri)
			continue
		}
		namespacedObjectCount++

		// update the resource to set the namespace
		updatedJSONDef, err := util.UpdateNamespace(*wri.JSONDefinition, namespace)
		if err != nil {
			return &processedWRIs, fmt.Errorf("failed to update JSON definition to set namespace: %w", err)
		}

		// convert the resource back into a gorm.io/datatypes.JSON object
		var jsonObj datatypes.JSON
		if err := json.Unmarshal(updatedJSONDef, &jsonObj); err != nil {
			return &processedWRIs, fmt.Errorf("failed to convert resource definition back into gorm JSON object type: %w", err)
		}
		wri.JSONDefinition = &jsonObj
		processedWRIs = append(processedWRIs, wri)
	}

	// only prepend the namespace resource if there are namespaced resources that require it
	if namespacedObjectCount > 0 && clientManagedNS == "" {

		namespaceWRI, err := kube_v0.CreateNamespaceWorkloadResourceInstance(namespace, *workloadInstanceID)
		if err != nil {
			return nil, fmt.Errorf("failed to create new workload resource instance for namespace: %w", err)
		}

		// move first resource to the back of the array, then prepend the namespace
		processedWRIs = append(processedWRIs, processedWRIs[0])
		processedWRIs[0] = *namespaceWRI
	}
	return &processedWRIs, nil
}
