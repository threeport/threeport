package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// IrsaServiceAccountYaml returns a ServiceAccount annotated for an EKS role.
func IrsaServiceAccountYaml(name, namespace, roleArn string) (string, error) {
	// annotate the account the operator creates so the pod assumes the role
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"annotations": map[string]interface{}{
				"eks.amazonaws.com/role-arn": roleArn,
			},
		},
	}}
	body, err := yaml.Marshal(object.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal service account: %w", err)
	}
	return string(body), nil
}
