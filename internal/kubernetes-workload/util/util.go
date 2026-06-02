package util

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// UnmarshalUniqueKubernetesWorkloadResourceInstance gets a unique kubernetes workload
// resource instance and unmarshals it.
func UnmarshalUniqueKubernetesWorkloadResourceInstance(k8sWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance, kind string) (map[string]interface{}, error) {

	// filter out service objects
	k8sWorkloadResourceInstance, err := GetUniqueKubernetesWorkloadResourceInstance(k8sWorkloadResourceInstances, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes workload resource instances from kubernetes workload instance: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*k8sWorkloadResourceInstance.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes workload resource instance object: %w", err)
	}

	return service, nil
}

// UnmarshalUniqueKubernetesWorkloadResourceDefinition gets a unique kubernetes workload
// resource definition and unmarshals it.
func UnmarshalUniqueKubernetesWorkloadResourceDefinition(k8sWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition, kind string) (map[string]interface{}, error) {

	// filter out service objects
	k8sWorkloadResourceDefinition, err := GetUniqueKubernetesWorkloadResourceDefinition(k8sWorkloadResourceDefinitions, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes workload resource definitions from kubernetes workload definition: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*k8sWorkloadResourceDefinition.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes workload resource definition object: %w", err)
	}

	return service, nil
}

// UnmarshalUniqueKubernetesWorkloadResourceDefinitionByName gets a unique kubernetes
// workload resource definition by name and unmarshals it.
func UnmarshalUniqueKubernetesWorkloadResourceDefinitionByName(k8sWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition, kind, name string) (map[string]interface{}, error) {

	// filter out service objects
	k8sWorkloadResourceDefinition, err := GetUniqueKubernetesWorkloadResourceDefinitionByName(k8sWorkloadResourceDefinitions, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes workload resource definitions from kubernetes workload definition: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*k8sWorkloadResourceDefinition.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes workload resource definition object: %w", err)
	}

	return service, nil
}

// UnmarshalKubernetesWorkloadResourceDefinition gets a kubernetes workload resource
// definition by kind and name and unmarshals it.
func UnmarshalKubernetesWorkloadResourceDefinition(k8sWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition, kind, name string) (map[string]interface{}, error) {

	// filter out service objects
	k8sWorkloadResourceDefinition, err := GetKubernetesWorkloadResourceDefinition(k8sWorkloadResourceDefinitions, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes workload resource definitions from kubernetes workload definition: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*k8sWorkloadResourceDefinition.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes workload resource definition object: %w", err)
	}

	return service, nil
}

// UnmarshalKubernetesWorkloadResourceInstance gets a kubernetes workload resource
// instance by kind and name and unmarshals it.
func UnmarshalKubernetesWorkloadResourceInstance(k8sWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance, kind, name string) (map[string]interface{}, error) {

	// filter out service objects
	k8sWorkloadResourceInstance, err := GetKubernetesWorkloadResourceInstance(k8sWorkloadResourceInstances, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes workload resource instances from kubernetes workload instance: %w", err)
	}

	// unmarshal service object
	service, err := util.UnmarshalJSON(*k8sWorkloadResourceInstance.JSONDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes workload resource instance object: %w", err)
	}

	return service, nil
}

// GetUniqueKubernetesWorkloadResourceInstance gets a unique kubernetes workload resource
// instance by kind.
func GetUniqueKubernetesWorkloadResourceInstance(k8sWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance, kind string) (*v0.KubernetesWorkloadResourceInstance, error) {

	var objects []v0.KubernetesWorkloadResourceInstance
	for _, wri := range *k8sWorkloadResourceInstances {

		mapDef, err := util.UnmarshalJSON(*wri.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind {
			objects = append(objects, wri)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("kubernetes workload resource instance not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource instances found")
	}

	return &objects[0], nil
}

// GetUniqueKubernetesWorkloadResourceInstanceByName gets a unique kubernetes workload
// resource instance by kind and name.
func GetUniqueKubernetesWorkloadResourceInstanceByName(k8sWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance, kind, name string) (*v0.KubernetesWorkloadResourceInstance, error) {

	var objects []v0.KubernetesWorkloadResourceInstance
	for _, wri := range *k8sWorkloadResourceInstances {

		mapDef, err := util.UnmarshalJSON(*wri.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		// get object kind
		manifestKind, kindFound, err := unstructured.NestedString(mapDef, "kind")
		if err != nil {
			return nil, fmt.Errorf("failed to get kind: %w", err)
		}
		if !kindFound {
			return nil, fmt.Errorf("kind not found")
		}

		// get object name
		manifestName, nameFound, err := unstructured.NestedString(mapDef, "metadata", "name")
		if err != nil {
			return nil, fmt.Errorf("failed to get name: %w", err)
		}
		if !nameFound {
			return nil, fmt.Errorf("name not found")
		}

		if manifestKind == kind &&
			manifestName == name {
			objects = append(objects, wri)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("kubernetes workload resource instance not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource instances found")
	}

	return &objects[0], nil
}

// GetUniqueKubernetesWorkloadResourceDefinition gets a unique kubernetes workload resource
// definition by kind.
func GetUniqueKubernetesWorkloadResourceDefinition(k8sWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition, kind string) (*v0.KubernetesWorkloadResourceDefinition, error) {

	var objects []v0.KubernetesWorkloadResourceDefinition
	for _, wrd := range *k8sWorkloadResourceDefinitions {

		mapDef, err := util.UnmarshalJSON(*wrd.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind {
			objects = append(objects, wrd)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("kubernetes workload resource definition not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource definitions found")
	}

	return &objects[0], nil

}

// GetUniqueKubernetesWorkloadResourceDefinitionByName gets a unique kubernetes workload
// resource definition by kind and name.
func GetUniqueKubernetesWorkloadResourceDefinitionByName(k8sWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition, kind, name string) (*v0.KubernetesWorkloadResourceDefinition, error) {

	var objects []v0.KubernetesWorkloadResourceDefinition
	for _, wrd := range *k8sWorkloadResourceDefinitions {

		mapDef, err := util.UnmarshalJSON(*wrd.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		// get object kind
		manifestKind, kindFound, err := unstructured.NestedString(mapDef, "kind")
		if err != nil {
			return nil, fmt.Errorf("failed to get kind: %w", err)
		}
		if !kindFound {
			return nil, fmt.Errorf("kind not found")
		}

		// get object name
		manifestName, nameFound, err := unstructured.NestedString(mapDef, "metadata", "name")
		if err != nil {
			return nil, fmt.Errorf("failed to get name: %w", err)
		}
		if !nameFound {
			return nil, fmt.Errorf("name not found")
		}

		if manifestKind == kind &&
			manifestName == name {
			objects = append(objects, wrd)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("kubernetes workload resource definition not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource definitions found")
	}

	return &objects[0], nil

}

// GetKubernetesWorkloadResourceDefinition gets a kubernetes workload resource definition
// by kind and name.
func GetKubernetesWorkloadResourceDefinition(k8sWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition, kind, name string) (*v0.KubernetesWorkloadResourceDefinition, error) {

	var objects []v0.KubernetesWorkloadResourceDefinition
	for _, wrd := range *k8sWorkloadResourceDefinitions {

		mapDef, err := util.UnmarshalJSON(*wrd.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind &&
			mapDef["metadata"].(map[string]interface{})["name"] == name {
			objects = append(objects, wrd)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("kubernetes workload resource definition not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource definitions found")
	}

	return &objects[0], nil

}

// GetKubernetesWorkloadResourceInstance gets a kubernetes workload resource instance
// by kind and name.
func GetKubernetesWorkloadResourceInstance(k8sWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance, kind, name string) (*v0.KubernetesWorkloadResourceInstance, error) {

	var objects []v0.KubernetesWorkloadResourceInstance
	for _, wri := range *k8sWorkloadResourceInstances {

		mapDef, err := util.UnmarshalJSON(*wri.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}

		if mapDef["kind"] == kind &&
			mapDef["metadata"].(map[string]interface{})["name"] == name {
			objects = append(objects, wri)
		}
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("kubernetes workload resource instance not found")
	}
	if len(objects) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource instances found")
	}

	return &objects[0], nil

}
