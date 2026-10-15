package v0

import (
	"fmt"

	ghodss_yaml "github.com/ghodss/yaml"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
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

// UnstructuredFromYaml decodes one YAML document into a fresh object.
func UnstructuredFromYaml(asset []byte) (*unstructured.Unstructured, error) {
	// convert the document, then decode a new object
	jsonBytes, err := yaml.YAMLToJSON(asset)
	if err != nil {
		return nil, fmt.Errorf("failed to convert yaml to json: %w", err)
	}
	object := &unstructured.Unstructured{}
	if err := object.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json into unstructured object: %w", err)
	}
	return object, nil
}

// SetIndexedField writes one field through maps and list indexes.
// unstructured.SetNestedField only accepts map keys.
func SetIndexedField(object map[string]interface{}, value interface{}, path ...interface{}) error {
	return writeIndexedField(object, value, false, path...)
}

// RemoveIndexedField drops one field through maps and list indexes.
func RemoveIndexedField(object map[string]interface{}, path ...interface{}) error {
	return writeIndexedField(object, nil, true, path...)
}

// writeIndexedField walks a path and writes or deletes the last entry.
func writeIndexedField(object map[string]interface{}, value interface{}, remove bool, path ...interface{}) error {
	// walk each path entry, then write or delete the last one
	if len(path) == 0 {
		return fmt.Errorf("failed to set field: empty path")
	}
	var current interface{} = object
	for i, entry := range path {
		last := i == len(path)-1
		switch key := entry.(type) {
		case string:
			fields, ok := current.(map[string]interface{})
			if !ok {
				return fmt.Errorf("failed to set field %v: expected a map", path)
			}
			if last {
				if remove {
					delete(fields, key)
					return nil
				}
				fields[key] = value
				return nil
			}
			next, ok := fields[key]
			if !ok {
				return fmt.Errorf("failed to set field %v: missing %s", path, key)
			}
			current = next
		case int:
			list, ok := current.([]interface{})
			if !ok {
				return fmt.Errorf("failed to set field %v: expected a list", path)
			}
			if key < 0 || key >= len(list) {
				return fmt.Errorf("failed to set field %v: index %d out of range", path, key)
			}
			if last {
				if remove {
					return fmt.Errorf("failed to set field %v: cannot delete a list entry", path)
				}
				list[key] = value
				return nil
			}
			current = list[key]
		default:
			return fmt.Errorf("failed to set field %v: path entry is not a string or an int", path)
		}
	}
	return nil
}

// MergeNamedEntries keeps entries already stored and adds or replaces by name.
func MergeNamedEntries(current, wanted []interface{}) []interface{} {
	// keep the stored entries, then replace or append by name
	merged := append([]interface{}{}, current...)
	for _, want := range wanted {
		wantMap, ok := want.(map[string]interface{})
		if !ok {
			merged = append(merged, want)
			continue
		}
		wantName, _, _ := unstructured.NestedString(wantMap, "name")
		replaced := false
		for i, have := range merged {
			haveMap, ok := have.(map[string]interface{})
			if !ok {
				continue
			}
			haveName, _, _ := unstructured.NestedString(haveMap, "name")
			if wantName != "" && haveName == wantName {
				merged[i] = want
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, want)
		}
	}
	return merged
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
