package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// CreateModuleApiRouteWithModuleObjectReferences creates a new module api route with module
// object references.  This allows an API call to create a module api route that also populates
// the many-to-many relationships between the module api route and the module objects.
// This sends the definition of the module API route with the definition of a module object embedded.
// It will not create any module objects.  The module objects must be created separately.
func CreateModuleApiRouteWithModuleObjectReferences(
	apiClient *http.Client,
	apiAddr string,
	moduleApiRoute *v0.ModuleApiRoute,
) (*v0.ModuleApiRoute, error) {
	jsonModuleApiRoute, err := util.MarshalObject(moduleApiRoute)
	if err != nil {
		return moduleApiRoute, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathModuleApiRouteWithModuleObjectReferences),
		http.MethodPost,
		bytes.NewBuffer(jsonModuleApiRoute),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return moduleApiRoute, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return moduleApiRoute, fmt.Errorf("threeport API returned no object for the created module api route")
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return moduleApiRoute, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&moduleApiRoute); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return moduleApiRoute, nil
}

// ReclaimModuleController asks the Threeport API to rebind the module controller
// with the provided name to the module api in moduleController.ModuleApiID. The
// API server checks that the controller's current owner is gone and rebinds the
// row in a single transaction, so the check cannot go stale before the write. A
// controller still owned by an active module api returns an error that wraps
// client_lib.ErrConflict. A controller that is already bound to the provided
// module api is returned as is.
func ReclaimModuleController(
	apiClient *http.Client,
	apiAddr string,
	moduleController *v0.ModuleController,
) (*v0.ModuleController, error) {
	payload := *moduleController
	payload.ID = nil
	payload.CreatedAt = nil
	payload.UpdatedAt = nil
	jsonModuleController, err := util.MarshalObject(payload)
	if err != nil {
		return moduleController, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathModuleControllerReclaim),
		http.MethodPost,
		bytes.NewBuffer(jsonModuleController),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return moduleController, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return moduleController, fmt.Errorf("threeport API returned no object for the reclaimed module controller")
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return moduleController, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	var reclaimed v0.ModuleController
	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&reclaimed); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &reclaimed, nil
}

// GetModuleObjectsWithModuleApiRoutes fetches all module objects with associated module api routes.
func GetModuleObjectsWithModuleApiRoutes(apiClient *http.Client, apiAddr string) (*[]v0.ModuleObject, error) {
	var moduleObjects []v0.ModuleObject

	allPagesReceived := false
	var allPageData []apiserver_lib.Object
	nextCursor := uint(0)
	queryId := ""
	for !allPagesReceived {
		url := fmt.Sprintf("%s%s", apiAddr, v0.PathModuleObjectsWithModuleApiRoutes)
		if queryId != "" {
			url = fmt.Sprintf("%s%s?queryid=%s&cursor=%d", apiAddr, v0.PathModuleObjectsWithModuleApiRoutes, queryId, nextCursor)
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
			return &moduleObjects, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
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
		return &moduleObjects, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&moduleObjects); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &moduleObjects, nil
}

// GetModuleObjectWithModuleApiRoutesByID fetches a module object with associated module api routes by module object ID.
func GetModuleObjectWithModuleApiRoutesByID(apiClient *http.Client, apiAddr string, moduleObjectID uint) (*v0.ModuleObject, error) {
	var moduleObject v0.ModuleObject

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s/%d", apiAddr, v0.PathModuleObjectsWithModuleApiRoutes, moduleObjectID),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &moduleObject, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return &moduleObject, fmt.Errorf("no module object found with ID %d", moduleObjectID)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &moduleObject, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&moduleObject); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &moduleObject, nil
}
