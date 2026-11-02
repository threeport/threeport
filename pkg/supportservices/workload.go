package supportservices

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"

	api "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	// IngressColumn is the runtime field that stores the ingress workload
	IngressColumn = "IngressComponentInstanceID"

	// CertificatesColumn is the runtime field that stores the certificates workload
	CertificatesColumn = "CertificatesComponentInstanceID"

	// SecretsColumn is the runtime field that stores the secrets workload
	SecretsColumn = "SecretsComponentInstanceID"

	// DatabaseColumn is the runtime field that stores the database workload
	DatabaseColumn = "DatabaseComponentInstanceID"
)

// IsComponentReconciled reports whether that workload instance has reconciled.
// A missing row returns reconciled false and missing true.
func IsComponentReconciled(r *controller.Reconciler, id uint) (bool, bool, error) {
	// read the workload the slot names
	workload, err := client.GetKubernetesWorkloadInstanceByID(
		r.APIClient,
		r.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	if workload.DeletionScheduled != nil {
		return false, false, fmt.Errorf("component workload %d is being deleted", id)
	}
	if workload.Reconciled != nil && *workload.Reconciled {
		return true, false, nil
	}
	return false, false, nil
}

// DeleteComponentWorkload removes that workload instance.
func DeleteComponentWorkload(r *controller.Reconciler, id uint) error {
	// treat a missing workload as already removed
	_, err := client.DeleteKubernetesWorkloadInstance(r.APIClient, r.APIServer, id)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil
	}
	return err
}

// ClaimComponentWorkload writes the runtime column when the compare accepts it.
func ClaimComponentWorkload(
	r *controller.Reconciler,
	runtimeID uint,
	workloadID uint,
	column string,
) error {
	// load the runtime and set the column
	runtime, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, runtimeID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	slot, err := componentColumn(runtime, column)
	if err != nil {
		return err
	}
	*slot = util.Ptr(workloadID)
	_, err = client.UpdateKubernetesRuntimeInstance(r.APIClient, r.APIServer, runtime)
	if err != nil {
		return fmt.Errorf("failed to claim component slot on runtime %d: %w", runtimeID, err)
	}
	return nil
}

// ComponentColumnHeld reports whether the runtime column stores this workload id.
func ComponentColumnHeld(
	r *controller.Reconciler,
	runtimeID uint,
	workloadID uint,
	column string,
) (bool, error) {
	// load the runtime and compare the stored id
	runtime, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, runtimeID)
	if err != nil {
		return false, fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	slot, err := componentColumn(runtime, column)
	if err != nil {
		return false, err
	}
	if *slot == nil || **slot != workloadID {
		return false, nil
	}
	return true, nil
}

// NullComponentColumn sets the runtime column to null when it stores this workload id.
// A remaining requires row returns cleared false.
func NullComponentColumn(
	r *controller.Reconciler,
	runtimeID uint,
	column string,
) (bool, error) {
	// load the runtime and encode the column as null
	runtime, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, runtimeID)
	if err != nil {
		return false, fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	body, err := componentSlotNull(runtime, column)
	if err != nil {
		return false, err
	}
	_, err = client_lib.GetResponse(
		r.APIClient,
		fmt.Sprintf("%s%s/%d", r.APIServer, api.PathKubernetesRuntimeInstances, runtimeID),
		http.MethodPut,
		bytes.NewBuffer(body),
		map[string]string{},
		http.StatusOK,
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, client_lib.ErrBadRequest) &&
		strings.Contains(err.Error(), api.ErrMsgSupportServiceSlotHeld) {
		return false, nil
	}
	return false, fmt.Errorf(
		"failed to clear component slot on runtime %d: %w",
		runtimeID,
		err,
	)
}

// ClearComponentWorkload nulls the runtime column when it stores this workload id.
// The cleared result is true when the column was set to null.
func ClearComponentWorkload(
	r *controller.Reconciler,
	runtimeID uint,
	workloadID uint,
	column string,
) (bool, error) {
	// leave a column this workload does not hold
	held, err := ComponentColumnHeld(r, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}
	return NullComponentColumn(r, runtimeID, column)
}

// componentColumn returns the runtime field named by column.
func componentColumn(runtime *api.KubernetesRuntimeInstance, column string) (**uint, error) {
	switch column {
	case IngressColumn:
		return &runtime.IngressComponentInstanceID, nil
	case CertificatesColumn:
		return &runtime.CertificatesComponentInstanceID, nil
	case SecretsColumn:
		return &runtime.SecretsComponentInstanceID, nil
	case DatabaseColumn:
		return &runtime.DatabaseComponentInstanceID, nil
	default:
		return nil, fmt.Errorf("unknown support service column %s", column)
	}
}

// componentSlotNull encodes the runtime with the named column set to null.
func componentSlotNull(runtime *api.KubernetesRuntimeInstance, column string) ([]byte, error) {
	// drop the identity fields and null the column
	encoded, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to encode runtime %d: %w", *runtime.ID, err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("failed to decode runtime %d: %w", *runtime.ID, err)
	}
	delete(fields, "ID")
	delete(fields, "CreatedAt")
	delete(fields, "UpdatedAt")
	fields[column] = nil
	return json.Marshal(fields)
}

// CreateComponentWorkload creates one component workload and returns its id.
func CreateComponentWorkload(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *api.KubernetesRuntimeInstance,
	workloadName string,
	manifest string,
	log *logr.Logger,
) (uint, error) {
	// reuse the workload definition a deleted component left behind
	var createdWorkloadDef *api.KubernetesWorkloadDefinition
	workloadDefName := workloadName
	existingDefinition, err := client.GetKubernetesWorkloadDefinitionByName(r.APIClient, r.APIServer, workloadDefName)
	if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to get component kubernetes workload definition: %w", err)
	}
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		workloadDefinition := api.KubernetesWorkloadDefinition{
			Definition:   api.Definition{Name: &workloadDefName},
			YAMLDocument: &manifest,
		}
		createdWorkloadDef, err = client.CreateKubernetesWorkloadDefinition(r.APIClient, r.APIServer, &workloadDefinition)
		if err != nil {
			return 0, fmt.Errorf("failed to create component kubernetes workload definition: %w", err)
		}
	} else {
		createdWorkloadDef = existingDefinition
	}

	// create the component workload instance
	workloadInstance := api.KubernetesWorkloadInstance{
		Instance:                       api.Instance{Name: &workloadDefName},
		KubernetesRuntimeInstanceID:    kubernetesRuntimeInstance.ID,
		KubernetesWorkloadDefinitionID: createdWorkloadDef.ID,
	}
	createdWorkloadInstance, err := client.CreateKubernetesWorkloadInstance(r.APIClient, r.APIServer, &workloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to create component kubernetes workload instance: %w", err)
	}

	log.V(1).Info(
		"support service workload created",
		"workloadInstanceID", *createdWorkloadInstance.ID,
	)

	return *createdWorkloadInstance.ID, nil
}

// SyncWorkloadObject stores each document on a component workload.
// The workload controller applies the objects on its next update.
func SyncWorkloadObject(
	r *controller.Reconciler,
	workloadID uint,
	manifest string,
) error {
	// store each document in the manifest
	documents, err := kube.GetYamlDocuments(manifest)
	if err != nil {
		return fmt.Errorf("failed to parse component manifest: %w", err)
	}
	if len(documents) == 0 {
		return fmt.Errorf("component manifest must hold an object")
	}
	for _, document := range documents {
		if err := syncWorkloadDocument(r, workloadID, document); err != nil {
			return err
		}
	}
	return nil
}

// syncWorkloadDocument stores one object on a component workload.
// The workload controller applies the object on its next update.
func syncWorkloadDocument(
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
		&api.KubernetesWorkloadResourceDefinition{
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
		&api.KubernetesWorkloadResourceInstance{
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
	workload *api.KubernetesWorkloadInstance,
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
