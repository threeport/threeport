// generated by 'threeport-sdk gen' - do not edit

package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"net/http"
)

// GetAttachedObjectReferences fetches all attached object references.
// TODO: implement pagination
func GetAttachedObjectReferences(apiClient *http.Client, apiAddr string) (*[]v0.AttachedObjectReference, error) {
	var attachedObjectReferences []v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathAttachedObjectReferences),
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

// GetAttachedObjectReferenceByID fetches a attached object reference by ID.
func GetAttachedObjectReferenceByID(apiClient *http.Client, apiAddr string, id uint) (*v0.AttachedObjectReference, error) {
	var attachedObjectReference v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathAttachedObjectReferences, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &attachedObjectReference, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &attachedObjectReference, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&attachedObjectReference); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &attachedObjectReference, nil
}

// GetAttachedObjectReferencesByQueryString fetches attached object references by provided query string.
func GetAttachedObjectReferencesByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.AttachedObjectReference, error) {
	var attachedObjectReferences []v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?%s", apiAddr, v0.PathAttachedObjectReferences, queryString),
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

// GetAttachedObjectReferenceByName fetches a attached object reference by name.
func GetAttachedObjectReferenceByName(apiClient *http.Client, apiAddr, name string) (*v0.AttachedObjectReference, error) {
	var attachedObjectReferences []v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s?name=%s", apiAddr, v0.PathAttachedObjectReferences, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.AttachedObjectReference{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.AttachedObjectReference{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&attachedObjectReferences); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(attachedObjectReferences) < 1:
		return &v0.AttachedObjectReference{}, errors.New(fmt.Sprintf("no attached object reference with name %s", name))
	case len(attachedObjectReferences) > 1:
		return &v0.AttachedObjectReference{}, errors.New(fmt.Sprintf("more than one attached object reference with name %s returned", name))
	}

	return &attachedObjectReferences[0], nil
}

// CreateAttachedObjectReference creates a new attached object reference.
func CreateAttachedObjectReference(apiClient *http.Client, apiAddr string, attachedObjectReference *v0.AttachedObjectReference) (*v0.AttachedObjectReference, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(attachedObjectReference)
	jsonAttachedObjectReference, err := util.MarshalObject(attachedObjectReference)
	if err != nil {
		return attachedObjectReference, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathAttachedObjectReferences),
		http.MethodPost,
		bytes.NewBuffer(jsonAttachedObjectReference),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return attachedObjectReference, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return attachedObjectReference, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&attachedObjectReference); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return attachedObjectReference, nil
}

// UpdateAttachedObjectReference updates a attached object reference.
func UpdateAttachedObjectReference(apiClient *http.Client, apiAddr string, attachedObjectReference *v0.AttachedObjectReference) (*v0.AttachedObjectReference, error) {
	client_lib.ReplaceAssociatedObjectsWithNil(attachedObjectReference)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	attachedObjectReferenceID := *attachedObjectReference.ID
	payloadAttachedObjectReference := *attachedObjectReference
	payloadAttachedObjectReference.ID = nil
	payloadAttachedObjectReference.CreatedAt = nil
	payloadAttachedObjectReference.UpdatedAt = nil

	jsonAttachedObjectReference, err := util.MarshalObject(payloadAttachedObjectReference)
	if err != nil {
		return attachedObjectReference, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathAttachedObjectReferences, attachedObjectReferenceID),
		http.MethodPatch,
		bytes.NewBuffer(jsonAttachedObjectReference),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return attachedObjectReference, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return attachedObjectReference, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadAttachedObjectReference); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadAttachedObjectReference.ID = &attachedObjectReferenceID
	return &payloadAttachedObjectReference, nil
}

// DeleteAttachedObjectReference deletes a attached object reference by ID.
func DeleteAttachedObjectReference(apiClient *http.Client, apiAddr string, id uint) (*v0.AttachedObjectReference, error) {
	var attachedObjectReference v0.AttachedObjectReference

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathAttachedObjectReferences, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &attachedObjectReference, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &attachedObjectReference, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&attachedObjectReference); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &attachedObjectReference, nil
}