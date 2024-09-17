package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// GetAttachedObjectReferenceByAttachedObjectID fetches an attached object reference
// by object ID. Returns an error if more than one object references the attached object,
// or if no object references the attached object.
func GetAttachedObjectReferenceByAttachedObjectID(
	apiClient *http.Client,
	apiAddr string,
	id uint,
) (
	*v0.AttachedObjectReference,
	error,
) {
	attachedObjectReferences, err := GetAttachedObjectReferencesByAttachedObjectID(apiClient, apiAddr, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get attached object references by object ID: %w", err)
	}
	if len(*attachedObjectReferences) == 0 || len(*attachedObjectReferences) > 1 {
		return nil, fmt.Errorf("expected 1 attached object reference, got %d", len(*attachedObjectReferences))
	}

	return &(*attachedObjectReferences)[0], nil
}

// GetAttachedObjectReferencesByAttachedObjectID fetches attached object references
// by attached object ID.
func GetAttachedObjectReferencesByAttachedObjectID(
	apiClient *http.Client,
	apiAddr string,
	id uint,
) (
	*[]v0.AttachedObjectReference,
	error,
) {
	var attachedObjectReferences []v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?attachedobjectid=%d", apiAddr, v0.PathAttachedObjectReferences, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &attachedObjectReferences, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &attachedObjectReferences, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&attachedObjectReferences); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &attachedObjectReferences, nil
}

// GetAttachedObjectReferencesByObjectID fetches an attached object reference
// by object ID.
func GetAttachedObjectReferencesByObjectID(
	apiClient *http.Client,
	apiAddr string,
	id uint,
) (
	*[]v0.AttachedObjectReference,
	error,
) {
	var attachedObjectReferences []v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?objectid=%d", apiAddr, v0.PathAttachedObjectReferences, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &attachedObjectReferences, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &attachedObjectReferences, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&attachedObjectReferences); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &attachedObjectReferences, nil
}

// EnsureAttachedObjectReferenceExists ensures that an attached object reference
// exists for the given object type and ID.
func EnsureAttachedObjectReferenceExists(
	apiClient *http.Client,
	apiAddr string,
	objectType string,
	objectID *uint,
	attachedObjectType string,
	attachedObjectID *uint,
) error {
	attachedObjectReferences, err := GetAttachedObjectReferencesByObjectID(apiClient, apiAddr, *objectID)
	if err != nil {
		return fmt.Errorf("failed to get attached object references by object ID: %w", err)
	}

	// check if attached object reference already exists
	for _, attachedObjectReference := range *attachedObjectReferences {
		if *attachedObjectReference.AttachedObjectType == attachedObjectType &&
			*attachedObjectReference.AttachedObjectID == *attachedObjectID {
			return nil
		}
	}

	// create attached object reference
	workloadInstanceAttachedObjectReference := &v0.AttachedObjectReference{
		ObjectID:           objectID,
		ObjectType:         &objectType,
		AttachedObjectType: &attachedObjectType,
		AttachedObjectID:   attachedObjectID,
	}
	_, err = CreateAttachedObjectReference(apiClient, apiAddr, workloadInstanceAttachedObjectReference)
	if err != nil {
		return fmt.Errorf("failed to create attached object reference: %w", err)
	}

	return nil
}
