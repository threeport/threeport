package v0

import (
	"fmt"

	ghodss_yaml "github.com/ghodss/yaml"
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
