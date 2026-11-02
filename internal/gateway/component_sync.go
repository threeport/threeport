package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// syncWorkloadObject adds or replaces one object on a component workload.
// The workload controller applies the object on its next update.
func syncWorkloadObject(
	r *controller.Reconciler,
	workloadID uint,
	manifest string,
) error {
	// read the object this caller needs
	documents, err := kube.GetJsonResourcesFromYamlDoc(manifest)
	if err != nil {
		return fmt.Errorf("failed to parse component manifest: %w", err)
	}
	if len(documents) != 1 {
		return fmt.Errorf("component manifest must hold one object")
	}
	kind, name, err := objectIdentity(documents[0])
	if err != nil {
		return err
	}
	desired := datatypes.JSON(documents[0])

	// load the workload and the objects it already stores
	workload, err := client.GetKubernetesWorkloadInstanceByID(r.APIClient, r.APIServer, workloadID)
	if err != nil {
		return fmt.Errorf("failed to get component workload: %w", err)
	}
	definition, err := client.GetKubernetesWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*workload.KubernetesWorkloadDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get component workload definition: %w", err)
	}
	resources, err := client.GetKubernetesWorkloadResourceInstancesByID(r.APIClient, r.APIServer, workloadID)
	if err != nil {
		return fmt.Errorf("failed to get component workload resources: %w", err)
	}

	// replace a stored object whose spec changed
	for _, resource := range *resources {
		if resource.JSONDefinition == nil {
			continue
		}
		storedKind, storedName, err := objectIdentity(*resource.JSONDefinition)
		if err != nil || storedKind != kind || storedName != name {
			continue
		}
		if sameJSON(*resource.JSONDefinition, desired) {
			return nil
		}
		resource.JSONDefinition = &desired
		reconciled := false
		resource.Reconciled = &reconciled
		_, err = client.UpdateKubernetesWorkloadResourceInstance(r.APIClient, r.APIServer, &resource)
		if err != nil {
			return fmt.Errorf("failed to update component workload resource: %w", err)
		}
		return markWorkloadUnreconciled(r, workload)
	}

	// add the object and record it on the definition document
	_, err = client.CreateKubernetesWorkloadResourceDefinition(
		r.APIClient,
		r.APIServer,
		&v0.KubernetesWorkloadResourceDefinition{
			JSONDefinition:                 &desired,
			KubernetesWorkloadDefinitionID: definition.ID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create component workload resource definition: %w", err)
	}
	reconciled := false
	_, err = client.CreateKubernetesWorkloadResourceInstance(
		r.APIClient,
		r.APIServer,
		&v0.KubernetesWorkloadResourceInstance{
			JSONDefinition:               &desired,
			KubernetesWorkloadInstanceID: workload.ID,
			Reconciled:                   &reconciled,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create component workload resource: %w", err)
	}
	document := manifest
	if definition.YAMLDocument != nil && *definition.YAMLDocument != "" {
		document = *definition.YAMLDocument + "\n---\n" + manifest
	}
	definition.YAMLDocument = &document
	if _, err := client.UpdateKubernetesWorkloadDefinition(r.APIClient, r.APIServer, definition); err != nil {
		return fmt.Errorf("failed to update component workload definition: %w", err)
	}
	return markWorkloadUnreconciled(r, workload)
}

// objectIdentity reads the kind and name from one stored object.
func objectIdentity(raw []byte) (string, string, error) {
	// decode the object
	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal component object: %w", err)
	}
	kind, _ := object["kind"].(string)
	metadata, _ := object["metadata"].(map[string]interface{})
	name, _ := metadata["name"].(string)
	if kind == "" || name == "" {
		return "", "", fmt.Errorf("component object is missing kind or name")
	}
	return kind, name, nil
}

// sameJSON reports whether two objects encode the same value.
func sameJSON(left, right []byte) bool {
	// compare the decoded values
	var decodedLeft any
	var decodedRight any
	if json.Unmarshal(left, &decodedLeft) != nil || json.Unmarshal(right, &decodedRight) != nil {
		return bytes.Equal(left, right)
	}
	encodedLeft, leftErr := json.Marshal(decodedLeft)
	encodedRight, rightErr := json.Marshal(decodedRight)
	if leftErr != nil || rightErr != nil {
		return bytes.Equal(left, right)
	}
	return bytes.Equal(encodedLeft, encodedRight)
}

// markWorkloadUnreconciled asks the workload controller to apply stored objects.
func markWorkloadUnreconciled(
	r *controller.Reconciler,
	workload *v0.KubernetesWorkloadInstance,
) error {
	// clear the reconciled flag so the update controller runs
	reconciled := false
	workload.Reconciled = &reconciled
	_, err := client.UpdateKubernetesWorkloadInstance(r.APIClient, r.APIServer, workload)
	if err != nil {
		return fmt.Errorf("failed to update component workload: %w", err)
	}
	return nil
}
