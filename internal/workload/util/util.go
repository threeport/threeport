package util

import (
	"fmt"

	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// UnmarshalUniqueWorkloadResourceInstance gets a unique workload resource instance
// and unmarshals it.
func UnmarshalUniqueWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (map[string]interface{}, error) {

	// filter out service objects
	workloadResourceInstance, err := GetUniqueWorkloadResourceInstance(workloadResourceInstances, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances from workload instance: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*workloadResourceInstance.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload resource instance object: %w", err)
	}

	return service, nil
}

// UnmarshalUniqueWorkloadResourceDefinition gets a unique workload resource instance
// and unmarshals it.
func UnmarshalUniqueWorkloadResourceDefinition(workloadResourceDefinitions *[]v0.WorkloadResourceDefinition, kind string) (map[string]interface{}, error) {

	// filter out service objects
	workloadResourceDefinition, err := GetUniqueWorkloadResourceDefinition(workloadResourceDefinitions, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances from workload instance: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*workloadResourceDefinition.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload resource definition object: %w", err)
	}

	return service, nil
}

// UnmarshalWorkloadResourceDefinition gets a unique workload resource instance
// and unmarshals it.
func UnmarshalWorkloadResourceDefinition(workloadResourceDefinitions *[]v0.WorkloadResourceDefinition, kind, name string) (map[string]interface{}, error) {

	// filter out service objects
	workloadResourceDefinition, err := GetWorkloadResourceDefinition(workloadResourceDefinitions, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances from workload instance: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*workloadResourceDefinition.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload resource definition object: %w", err)
	}

	return service, nil
}

// UnmarshalWorkloadResourceInstance gets a unique workload resource instance
// and unmarshals it.
func UnmarshalWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind, name string) (map[string]interface{}, error) {

	// filter out service objects
	workloadResourceInstance, err := GetWorkloadResourceInstance(workloadResourceInstances, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances from workload instance: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*workloadResourceInstance.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload resource definition object: %w", err)
	}

	return service, nil
}

// GetUniqueWorkloadResourceInstance gets a unique workload resource instance.
func GetUniqueWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (*v0.WorkloadResourceInstance, error) {

	var objects []v0.WorkloadResourceInstance
	for _, wri := range *workloadResourceInstances {

		mapDef, err := util.UnmarshalJSON(*wri.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind {
			objects = append(objects, wri)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("workload resource instance not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple workload resource instances found")
	}

	return &objects[0], nil

}

// GetUniqueWorkloadResourceDefinition gets a unique workload resource instance.
func GetUniqueWorkloadResourceDefinition(workloadResourceDefinitions *[]v0.WorkloadResourceDefinition, kind string) (*v0.WorkloadResourceDefinition, error) {

	var objects []v0.WorkloadResourceDefinition
	for _, wrd := range *workloadResourceDefinitions {

		mapDef, err := util.UnmarshalJSON(*wrd.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind {
			objects = append(objects, wrd)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("workload resource definition not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple workload resource definitions found")
	}

	return &objects[0], nil

}

// GetUniqueWorkloadResourceDefinition gets a unique workload resource instance.
func GetWorkloadResourceDefinition(workloadResourceDefinitions *[]v0.WorkloadResourceDefinition, kind, name string) (*v0.WorkloadResourceDefinition, error) {

	var objects []v0.WorkloadResourceDefinition
	for _, wrd := range *workloadResourceDefinitions {

		mapDef, err := util.UnmarshalJSON(*wrd.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind &&
			mapDef["metadata"].(map[string]interface{})["name"] == name {
			objects = append(objects, wrd)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("workload resource definition not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple workload resource definitions found")
	}

	return &objects[0], nil

}

// GetWorkloadResourceInstance gets a unique workload resource instance.
func GetWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind, name string) (*v0.WorkloadResourceInstance, error) {

	var objects []v0.WorkloadResourceInstance
	for _, wri := range *workloadResourceInstances {

		mapDef, err := util.UnmarshalJSON(*wri.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind &&
			mapDef["metadata"].(map[string]interface{})["name"] == name {
			objects = append(objects, wri)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("workload resource instance not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple workload resource instances found")
	}

	return &objects[0], nil

}
