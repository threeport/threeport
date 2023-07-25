package util

import (
	"encoding/json"
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/datatypes"
	"sigs.k8s.io/yaml"
)

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

func UnmarshalWorkloadResourceInstance(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (map[string]interface{}, error) {

	// filter out service objects
	filteredObjects, err := FilterObjects(workloadResourceInstances, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get service objects from workload instance: %w", err)
	}
	if len(*filteredObjects) == 0 {
		return nil, fmt.Errorf("no service objects found")
	}
	if len(*filteredObjects) > 1 {
		return nil, fmt.Errorf("multiple service objects found")
	}

	// unmarshal service object
	service, err := UnmarshalJSON(*((*filteredObjects)[0]).JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal service object: %w", err)
	}

	return service, nil
}

// filterObjects returns a list of
// unstructured kubernetes objects from a list of workload resource instances.
func FilterObjects(workloadResourceInstances *[]v0.WorkloadResourceInstance, kind string) (*[]v0.WorkloadResourceInstance, error) {

	var objects []v0.WorkloadResourceInstance
	for _, wri := range *workloadResourceInstances {

		var mapDef map[string]interface{}
		err := json.Unmarshal([]byte(*wri.JSONDefinition), &mapDef)
		if err != nil {
			return &objects, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind {
			objects = append(objects, wri)
		}
	}

	return &objects, nil
}

