package v0

import (
	"fmt"

	ghodss_yaml "github.com/ghodss/yaml"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// This file is named unstructured.go because it uses the same object shape as k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.
// An unstructured object is a Kubernetes resource held as nested maps and lists, with no generated Go type.
// The client-go helpers address map keys.
// This file also addresses a list index, a JSON number decoded as float64, and conversion to YAML and datatypes.JSON.

// NestedInt64OrFloat64 returns the int64 value of the nested field in the unstructured
// object and gracefully handles the case where the value is a float64
func NestedInt64OrFloat64(input map[string]interface{}, fields ...string) (int64, bool, error) {
	// read the field as an int64
	var output int64
	var found bool
	output, found, err := unstructured.NestedInt64(input, fields...)
	if err != nil {
		// read a json number held as float64
		floatOutput, found, errFloat := unstructured.NestedFloat64(input, fields...)
		output = int64(floatOutput)
		if errFloat != nil {
			return 0, false, fmt.Errorf("failed to read nested number: %w", errFloat)
		}
		return output, found, nil
	}

	return output, found, nil
}

// UnstructuredToYaml converts an unstructured object to a yaml string
func UnstructuredToYaml(input *unstructured.Unstructured) (string, error) {
	// marshal the object to json
	json, err := input.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal unstructured object to json: %v", err)
	}

	// convert the json to yaml
	yaml, err := ghodss_yaml.JSONToYAML(json)
	if err != nil {
		return "", fmt.Errorf("failed to convert json to yaml: %v", err)
	}
	return string(yaml), nil
}

// UnstructuredFromYaml decodes one YAML document into a fresh object.
func UnstructuredFromYaml(asset []byte) (*unstructured.Unstructured, error) {
	// convert the document to json
	jsonBytes, err := yaml.YAMLToJSON(asset)
	if err != nil {
		return nil, fmt.Errorf("failed to convert yaml to json: %w", err)
	}
	// decode a new object
	object := &unstructured.Unstructured{}
	if err := object.UnmarshalJSON(jsonBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json into unstructured object: %w", err)
	}
	return object, nil
}

// FieldAt reads one field through maps and list indexes.
func FieldAt(object map[string]interface{}, path ...interface{}) (interface{}, error) {
	// reject an empty path
	if len(path) == 0 {
		return nil, fmt.Errorf("failed to read field: empty path")
	}
	// walk each path entry and return the last one
	var current interface{} = object
	for i, entry := range path {
		last := i == len(path)-1
		switch key := entry.(type) {
		case string:
			// read a map key
			fields, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("failed to read field %v: expected a map", path)
			}
			next, ok := fields[key]
			if !ok {
				return nil, fmt.Errorf("failed to read field %v: missing %s", path, key)
			}
			if last {
				return next, nil
			}
			current = next
		case int:
			// read a list index
			list, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("failed to read field %v: expected a list", path)
			}
			if key < 0 || key >= len(list) {
				return nil, fmt.Errorf("failed to read field %v: index %d out of range", path, key)
			}
			if last {
				return list[key], nil
			}
			current = list[key]
		default:
			// reject a path entry that is not a string or an int
			return nil, fmt.Errorf("failed to read field %v: path entry is not a string or an int", path)
		}
	}
	return nil, fmt.Errorf("failed to read field %v", path)
}

// SetIndexedField writes one field through maps and list indexes.
func SetIndexedField(object map[string]interface{}, value interface{}, path ...interface{}) error {
	return writeIndexedField(object, value, false, path...)
}

// RemoveIndexedField drops one field through maps and list indexes.
func RemoveIndexedField(object map[string]interface{}, path ...interface{}) error {
	return writeIndexedField(object, nil, true, path...)
}

// writeIndexedField walks a path and writes or deletes the last entry.
func writeIndexedField(object map[string]interface{}, value interface{}, remove bool, path ...interface{}) error {
	// reject an empty path
	if len(path) == 0 {
		return fmt.Errorf("failed to set field: empty path")
	}
	// walk each path entry, then write or delete the last one
	var current interface{} = object
	for i, entry := range path {
		last := i == len(path)-1
		switch key := entry.(type) {
		case string:
			// write or delete a map key
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
			// write a list index
			list, ok := current.([]interface{})
			if !ok {
				return fmt.Errorf("failed to set field %v: expected a list", path)
			}
			if key < 0 || key >= len(list) {
				return fmt.Errorf("failed to set field %v: index %d out of range", path, key)
			}
			if last {
				if remove {
					// reject a delete of a list entry
					return fmt.Errorf("failed to set field %v: cannot delete a list entry", path)
				}
				list[key] = value
				return nil
			}
			current = list[key]
		default:
			// reject a path entry that is not a string or an int
			return fmt.Errorf("failed to set field %v: path entry is not a string or an int", path)
		}
	}
	return nil
}

// MergeNamedEntries keeps entries already stored and adds or replaces by name.
func MergeNamedEntries(current, wanted []interface{}) []interface{} {
	// copy the current entries
	merged := append([]interface{}{}, current...)
	for _, want := range wanted {
		// append a value that is not a map
		wantMap, ok := want.(map[string]interface{})
		if !ok {
			merged = append(merged, want)
			continue
		}
		// match an entry by name
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
			// append an entry with a new name
			merged = append(merged, want)
		}
	}
	return merged
}

// UnstructuredToDatatypesJson converts an unstructured object to a datatypes.JSON object
func UnstructuredToDatatypesJson(input *unstructured.Unstructured) (datatypes.JSON, error) {
	// marshal the object to json
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
	// convert each object
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
	// unmarshal the json into an object
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
	// convert each value
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
	// find the object with this name and kind
	for i, instance := range *instances {
		unstructuredObject, err := DataTypesJsonToUnstructured(&instance)
		if err != nil {
			return fmt.Errorf("failed to convert datatypes.JSON to unstructured object: %v", err)
		}

		// drop the matching object
		if name == unstructuredObject.GetName() &&
			kind == unstructuredObject.GetKind() {
			*instances = append((*instances)[:i], (*instances)[i+1:]...)
			break
		}
	}
	return nil
}
