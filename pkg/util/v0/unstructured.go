package v0

import (
	"fmt"

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
