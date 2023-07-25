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
	return datatypes.JSON(jsonDef), nil
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

// GetUniqueWorkloadResourceInstnace gets a unique workload resource instance.
func GetUniqueWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (*v0.WorkloadResourceInstance, error) {

	var objects []v0.WorkloadResourceInstance
	for _, wri := range *workloadResourceInstances {

		var mapDef map[string]interface{}
		err := json.Unmarshal([]byte(*wri.JSONDefinition), &mapDef)
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
