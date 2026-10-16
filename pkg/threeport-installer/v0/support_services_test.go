package v0

import (
	"testing"
)

func TestSupportServicesOperatorObjects(t *testing.T) {
	objects, err := supportServicesOperatorObjects()
	if err != nil {
		t.Fatalf("decode install manifest: %v", err)
	}

	var crds, deployments int
	for _, object := range objects {
		if object.GetKind() == "Namespace" {
			t.Fatalf("namespace object %s should be dropped", object.GetName())
		}
		if object.GetNamespace() == supportServicesOperatorNamespace {
			t.Fatalf("%s %s still uses the upstream namespace", object.GetKind(), object.GetName())
		}
		if object.GetKind() == "CustomResourceDefinition" {
			crds++
		}
		if object.GetKind() == "Deployment" && object.GetName() == SupportServicesOperatorDeployName {
			deployments++
			containers, _, _ := nestedContainers(object.Object)
			found := false
			for _, container := range containers {
				if container["image"] == SupportServicesOperatorImage {
					found = true
				}
				if container["image"] == supportServicesOperatorImage {
					t.Fatalf("deployment still has the placeholder image")
				}
			}
			if !found {
				t.Fatalf("deployment image is not %s", SupportServicesOperatorImage)
			}
			if object.GetNamespace() != ControlPlaneNamespace {
				t.Fatalf("deployment namespace is %s", object.GetNamespace())
			}
		}
	}
	if crds != 5 {
		t.Fatalf("got %d custom resource definitions", crds)
	}
	if deployments != 1 {
		t.Fatalf("got %d operator deployments", deployments)
	}
}

func nestedContainers(object map[string]interface{}) ([]map[string]interface{}, bool, error) {
	spec, _ := object["spec"].(map[string]interface{})
	template, _ := spec["template"].(map[string]interface{})
	podSpec, _ := template["spec"].(map[string]interface{})
	raw, _ := podSpec["containers"].([]interface{})
	var containers []map[string]interface{}
	for _, item := range raw {
		typed, ok := item.(map[string]interface{})
		if ok {
			containers = append(containers, typed)
		}
	}
	return containers, true, nil
}
