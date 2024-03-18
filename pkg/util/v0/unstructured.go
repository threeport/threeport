package v0

import (
	"fmt"

	ghodss_yaml "github.com/ghodss/yaml"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NestedInt64OrFloat64 returns the int64 value of the nested field in the unstructured
// object and gracefully handles the case where the value is a float64
func NestedInt64OrFloat64(input map[string]interface{}, fields ...string) (int64, bool, error) {
	var output int64
	var found bool
	output, found, err := unstructured.NestedInt64(input, fields...)
	if err != nil {
		floatOutput, found, errFloat := unstructured.NestedFloat64(input, fields...)
		output = int64(floatOutput)
		if errFloat != nil {
			return 0, false, fmt.Errorf("failed to get port from from gloo edge custom resource: %v", err)
		}
		return output, found, nil
	}

	return output, found, nil
}

// UnstructuredToYaml converts an unstructured object to a yaml string
func UnstructuredToYaml(input *unstructured.Unstructured) (string, error) {
	json, err := input.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal unstructured object to json: %v", err)
	}

	yaml, err := ghodss_yaml.JSONToYAML(json)
	if err != nil {
		return "", fmt.Errorf("failed to convert json to yaml: %v", err)
	}
	return string(yaml), nil
}

// UnstructuredToDatatypesJson converts an unstructured object to a datatypes.JSON object
func UnstructuredToDatatypesJson(input *unstructured.Unstructured) (datatypes.JSON, error) {
	json, err := input.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unstructured object to json: %v", err)
	}

	var output datatypes.JSON
	output = json
	return output, nil
}

// UnstructuredListToDatatypesJsonSlice converts a slice of unstructured objects to a
// slice of datatypes.JSON objects
func UnstructuredListToDatatypesJsonSlice(input []*unstructured.Unstructured) (
	datatypes.JSONSlice[datatypes.JSON],
	error,
) {
	var output datatypes.JSONSlice[datatypes.JSON]
	for _, item := range input {
		json, err := UnstructuredToDatatypesJson(item)
		if err != nil {
			return nil, fmt.Errorf("failed to convert unstructured object to datatypes.JSON: %v", err)
		}
		output = append(output, json)
	}
	return output, nil
}

// DatatypesJsonToUnstructured converts a datatypes.JSON object to an unstructured object
func DataTypesJsonToUnstructured(input *datatypes.JSON) (*unstructured.Unstructured, error) {
	var output unstructured.Unstructured
	err := output.UnmarshalJSON(*input)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal datatypes.JSON to unstructured object: %v", err)
	}
	return &output, nil
}

// DataTypesJsonSliceToUnstructuredList converts a slice of datatypes.JSON objects to a
// slice of unstructured objects
func DataTypesJsonSliceToUnstructuredList(
	input datatypes.JSONSlice[datatypes.JSON],
) ([]*unstructured.Unstructured, error) {
	var output []*unstructured.Unstructured
	for _, item := range input {
		unstructuredItem, err := DataTypesJsonToUnstructured(&item)
		if err != nil {
			return nil, fmt.Errorf("failed to convert datatypes.JSON to unstructured object: %v", err)
		}
		output = append(output, unstructuredItem)
	}
	return output, nil
}

// RemoveDataTypesJsonFromDataTypesJsonSlice removes a datatypes.JSON object from a slice
// of datatypes.JSON objects
func RemoveDataTypesJsonFromDataTypesJsonSlice(
	name,
	kind string,
	instances *datatypes.JSONSlice[datatypes.JSON],
) error {
	for i, instance := range *instances {
		unstructuredObject, err := DataTypesJsonToUnstructured(&instance)
		if err != nil {
			return fmt.Errorf("failed to convert datatypes.JSON to unstructured object: %v", err)
		}

		if name == unstructuredObject.GetName() &&
			kind == unstructuredObject.GetKind() {
			*instances = append((*instances)[:i], (*instances)[i+1:]...)
			break
		}
	}
	return nil
}
