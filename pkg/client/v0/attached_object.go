package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// GetObjectIdByAttachedObject gets an object ID that an attached object is
// attached to.  It is used when it is known that a single object of a
// particular type is has been attached to.  For example, if you have a workload
// instance and you need the ID of the kubernetes runtime instance it is
// attached to, the `objectType` will be
// `v0.ObjectTypeKubernetesRuntimeInstance` and the `attachedObjectType` will be
// `v0.ObjectTypeWorkloadInstance`.  With the workload instance's ID, this
// client function will return the ID of the kubernetes runtime instance.  This
// function will return an error if a single object is not found in the attached
// object reference table.
func GetObjectIdByAttachedObject(
	apiClient *http.Client,
	apiAddr string,
	objectType string,
	attachedObjectType string,
	attachedObjectId uint,
) (*uint, error) {
	objectIds, err := GetObjectIdsByAttachedObject(
		apiClient,
		apiAddr,
		objectType,
		attachedObjectType,
		attachedObjectId,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve object ID by attached object: %w", err)
	}

	if len(objectIds) != 1 {
		return nil, fmt.Errorf("expected 1 attached object reference, got %d", len(objectIds))
	}

	return objectIds[0], nil
}

// GetObjectIdsByAttachedObject will return all the objects of a particular type
// that an object is attached to.  It takes the object type that is being sought
// as well as the attached object type and ID.  It will return all the object
// IDs for the objects that are attached to.
func GetObjectIdsByAttachedObject(
	apiClient *http.Client,
	apiAddr string,
	objectType string,
	attachedObjectType string,
	attachedObjectId uint,
) ([]*uint, error) {
	queryString := fmt.Sprintf(
		"objecttype=%s&attachedobjecttype=%s&attachedobjectid=%d",
		objectType,
		attachedObjectType,
		attachedObjectId,
	)
	attachedObjRefs, err := GetAttachedObjectReferencesByQueryString(
		apiClient,
		apiAddr,
		queryString,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve object IDs by attached object: %w", err)
	}

	var objectIds []*uint
	for _, attachedObjRef := range *attachedObjRefs {
		objectIds = append(objectIds, attachedObjRef.ObjectID)
	}

	return objectIds, nil
}

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

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?attachedobjectid=%d", apiAddr, v0.PathAttachedObjectReferences, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?attachedobjectid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathAttachedObjectReferences, id, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &attachedObjectReferences, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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

// GetAttachedObjectReferencesByObjectID fetches attached object references
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

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s?objectid=%d", apiAddr, v0.PathAttachedObjectReferences, id)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?objectid=%d&queryid=%s&cursor=%d", apiAddr, v0.PathAttachedObjectReferences, id, queryId, nextCursor)
		}

		response, err := client_lib.GetResponse(
			apiClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return &attachedObjectReferences, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
		}

		allPageData = append(allPageData, response.Data...)

		if response.Meta.Pagination.HasMore {
			nextCursor = response.Meta.Pagination.NextCursor
			queryId = response.Meta.Pagination.QueryId
		} else {
			allPagesReceived = true
		}
	}

	jsonData, err := json.Marshal(allPageData)
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
	blocking bool,
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
	ref := &v0.AttachedObjectReference{
		ObjectID:           objectID,
		ObjectType:         &objectType,
		AttachedObjectType: &attachedObjectType,
		AttachedObjectID:   attachedObjectID,
		Blocking:           &blocking,
	}
	if _, err := CreateAttachedObjectReference(apiClient, apiAddr, ref); err != nil {
		return fmt.Errorf("failed to create attached object reference: %w", err)
	}

	return nil
}

// EnsureAttachedObjectReferenceRemoved ensures that an attached object reference
// is removed for the given object type and ID.
func EnsureAttachedObjectReferenceRemoved(
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

	// check to see if attachment already gone
	if len(*attachedObjectReferences) == 0 {
		return nil
	}

	// delete attached object reference
	_, err = DeleteAttachedObjectReference(apiClient, apiAddr, *(*attachedObjectReferences)[0].ID)
	if err != nil {
		return fmt.Errorf("failed to remove attached object reference: %w", err)
	}

	return nil
}
