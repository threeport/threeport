package v0

import (
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"
	"sigs.k8s.io/yaml"
)

// MarshalObject takes an object interface and returns its json byte array.
func MarshalObject(object interface{}) ([]byte, error) {
	objectJSON, err := json.Marshal(object)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	return objectJSON, nil
}

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
