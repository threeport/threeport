package helmworkload

import (
	"fmt"
	"os"

	util "github.com/threeport/threeport/pkg/util/v0"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/cli/values"
)

// MergeHelmValuesGo merges two helm values documents and
// returns the result as a map[string]interface{}.
func MergeHelmValuesGo(base, override string) (map[string]interface{}, error) {

	temporaryFiles := map[string]string{
		"/tmp/values.yaml":          base,
		"/tmp/override-values.yaml": override,
	}

	var valueFiles []string
	// create temporary files in /tmp and populate valueFiles
	for path, file := range temporaryFiles {
		err := os.WriteFile(path, []byte(file), 0644)
		if err != nil {
			return map[string]interface{}{}, fmt.Errorf("failed to write base helm values: %w", err)
		}
		valueFiles = append(valueFiles, path)
	}

	values := values.Options{
		ValueFiles: valueFiles,
	}
	grafanaGoValues, err := values.MergeValues(nil)
	if err != nil {
		return map[string]interface{}{}, fmt.Errorf("failed to merge grafana helm values: %w", err)
	}

	// clean up temporary files
	for filePath := range temporaryFiles {
		err := os.Remove(filePath)
		if err != nil {
			return map[string]interface{}{}, fmt.Errorf("failed to remove temporary file: %w", err)
		}
	}

	return grafanaGoValues, nil
}

// MergeHelmValuesString merges two helm values documents and
// returns the result as a string.
func MergeHelmValuesString(base, override string) (string, error) {

	// if one input is empty, return the other
	if base == "" {
		return override, nil
	} else if override == "" {
		return base, nil
	}

	// merge the helm values
	grafanaGoValues, err := MergeHelmValuesGo(base, override)
	if err != nil {
		return "", fmt.Errorf("failed to merge helm values: %w", err)
	}

	// marshal the merged helm values
	grafanaByteValues, err := yaml.Marshal(grafanaGoValues)
	if err != nil {
		return "", fmt.Errorf("failed to marshal grafana helm values: %w", err)
	}

	return string(grafanaByteValues), nil
}

// MergeHelmValuesPtrs merges two helm values documents
// that are referred to by string pointers.
func MergeHelmValuesPtrs(base, override *string) (string, error) {
	mergedHelmValues, err := MergeHelmValuesString(
		util.StringPtrToString(base),
		util.StringPtrToString(override),
	)
	if err != nil {
		return "", fmt.Errorf("failed to merge helm values: %w", err)
	}
	return mergedHelmValues, nil
}

// UnmarshalHelmValues unmarshals a helm values document.
func UnmarshalHelmValues(helmValues string) (map[string]interface{}, error) {
	var values map[string]interface{}
	err := yaml.Unmarshal([]byte(helmValues), &values)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal helm values: %w", err)
	}
	return values, nil
}
