package util

import (
	"encoding/json"
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/datatypes"
	"sigs.k8s.io/yaml"
)

// MarshalJSON marshals a map[string]interface{} into a datatypes.JSON object
func MarshalJSON(mapDef map[string]interface{}) (datatypes.JSON, error) {

	jsonDef, err := json.Marshal(mapDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}

	var jsonDatatype datatypes.JSON
	err = jsonDatatype.UnmarshalJSON(jsonDef)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w", err)
	}

	return jsonDatatype, nil
}

// UnmarshalJSON unmarshals a datatypes.JSON object into a map[string]interface{}
func UnmarshalJSON(marshaledJson datatypes.JSON) (map[string]interface{}, error) {
	var mapDef map[string]interface{}
	err := json.Unmarshal([]byte(marshaledJson), &mapDef)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w", err)
	}
	return mapDef, nil
}

// UnmarshalYAML unmarshals a YAML string into a map[string]interface{}
func UnmarshalYAML(marshaledYaml string) (map[string]interface{}, error) {
	var mapDef map[string]interface{}
	err := yaml.Unmarshal([]byte(marshaledYaml), &mapDef)
	if err != nil {
		return nil, fmt.Errorf("error parsing YAML: %v", err)
	}

	return mapDef, nil
}

// UnmarshalUniqueWorkloadResourceInstance gets a unique workload resource instance
// and unmarshals it.
func UnmarshalUniqueWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (map[string]interface{}, error) {

	// filter out service objects
	workloadResourceInstance, err := GetUniqueWorkloadResourceInstance(workloadResourceInstances, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload resource instances from workload instance: %w", err)
	}

	// unmarshal service object
	service, err := UnmarshalJSON(*workloadResourceInstance.JSONDefinition)
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
	service, err := UnmarshalJSON(*workloadResourceDefinition.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload resource definition object: %w", err)
	}

	return service, nil
}

// GetUniqueWorkloadResourceInstance gets a unique workload resource instance.
func GetUniqueWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (*v0.WorkloadResourceInstance, error) {

	var objects []v0.WorkloadResourceInstance
	for _, wri := range *workloadResourceInstances {

		mapDef, err := UnmarshalJSON(*wri.JSONDefinition)
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

		mapDef, err := UnmarshalJSON(*wrd.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind {
			objects = append(objects, wrd)
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
