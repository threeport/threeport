package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)


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

	response, err := GetResponse(
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

	response, err := GetResponse(
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
