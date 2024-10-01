package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// AppendObjectToYamlDoc takes an unstructured Kubernetes object and YAML
// document as a string and appends the K8s object to the YAML doc.
func AppendObjectToYamlDoc(
	object *unstructured.Unstructured,
	yamlDoc string,
) (string, error) {
	yamlObj, err := UnstructuredToYaml(object)
	if err != nil {
		return yamlDoc, fmt.Errorf("failed to convert object to YAML: %w", err)
	}

	yamlDoc += "\n---\n"
	yamlDoc += yamlObj

	return yamlDoc, nil
}

// UnstructuredToYaml converts an unstructured Kubernetes object to YAML.
func UnstructuredToYaml(object *unstructured.Unstructured) (string, error) {
	yamlData, err := yaml.Marshal(object.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object to YAML: %w", err)
	}

	return string(yamlData), nil
}
