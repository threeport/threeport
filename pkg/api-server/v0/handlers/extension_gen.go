// generated by 'threeport-sdk gen' - do not edit

package handlers

import (
	"errors"
	echo "github.com/labstack/echo/v4"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	gorm "gorm.io/gorm"
	"net/http"
)

///////////////////////////////////////////////////////////////////////////////
// ExtensionApi
///////////////////////////////////////////////////////////////////////////////

// @Summary GetExtensionApiVersions gets the supported versions for the extension api API.
// @Description Get the supported API versions for extension apis.
// @ID extensionApi-get-versions
// @Produce json
// @Success 200 {object} apiserver_lib.ApiObjectVersions "OK"
// @Router /extension-apis/versions [GET]
func (h Handler) GetExtensionApiVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, apiserver_lib.ObjectVersions[string(api_v0.ObjectTypeExtensionApi)])
}

// @Summary adds a new extension api.
// @Description Add a new extension api to the Threeport database.
// @ID add-v0-extensionApi
// @Accept json
// @Produce json
// @Param extensionApi body api_v0.ExtensionApi true "ExtensionApi object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-apis [POST]
func (h Handler) AddExtensionApi(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApi
	var extensionApi api_v0.ExtensionApi

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, objectType, extensionApi); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&extensionApi); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, extensionApi, objectType); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingExtensionApi api_v0.ExtensionApi
	nameUsed := true
	result := h.DB.Where("name = ?", extensionApi.Name).First(&existingExtensionApi)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			nameUsed = false
		} else {
			return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
		}
	}
	if nameUsed {
		return apiserver_lib.ResponseStatus409(c, nil, errors.New("object with provided name already exists"), objectType)
	}

	// persist to DB
	if result := h.DB.Create(&extensionApi); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, extensionApi, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

// @Summary gets all extension apis.
// @Description Get all extension apis from the Threeport database.
// @ID get-v0-extensionApis
// @Accept json
// @Produce json
// @Param name query string false "extension api search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-apis [GET]
func (h Handler) GetExtensionApis(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApi
	params, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	var filter api_v0.ExtensionApi
	if err := c.Bind(&filter); err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&api_v0.ExtensionApi{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]api_v0.ExtensionApi{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary gets a extension api.
// @Description Get a particular extension api from the database.
// @ID get-v0-extensionApi
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-apis/{id} [GET]
func (h Handler) GetExtensionApi(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApi
	extensionApiID := c.Param("id")
	var extensionApi api_v0.ExtensionApi
	if result := h.DB.First(&extensionApi, extensionApiID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, extensionApi, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing extension api.
// @Description Update a extension api in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating extension api objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-v0-extensionApi
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param extensionApi body api_v0.ExtensionApi true "ExtensionApi object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-apis/{id} [PATCH]
func (h Handler) UpdateExtensionApi(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApi
	extensionApiID := c.Param("id")
	var existingExtensionApi api_v0.ExtensionApi
	if result := h.DB.First(&existingExtensionApi, extensionApiID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, true, objectType, existingExtensionApi); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedExtensionApi api_v0.ExtensionApi
	if err := c.Bind(&updatedExtensionApi); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingExtensionApi).Updates(updatedExtensionApi); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, existingExtensionApi, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary updates an existing extension api by replacing the entire object.
// @Description Replace a extension api in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating extension api objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-v0-extensionApi
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param extensionApi body api_v0.ExtensionApi true "ExtensionApi object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-apis/{id} [PUT]
func (h Handler) ReplaceExtensionApi(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApi
	extensionApiID := c.Param("id")
	var existingExtensionApi api_v0.ExtensionApi
	if result := h.DB.First(&existingExtensionApi, extensionApiID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, true, objectType, existingExtensionApi); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedExtensionApi api_v0.ExtensionApi
	if err := c.Bind(&updatedExtensionApi); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, updatedExtensionApi, objectType); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedExtensionApi.ID = existingExtensionApi.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedExtensionApi); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingExtensionApi, extensionApiID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, existingExtensionApi, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary deletes a extension api.
// @Description Delete a extension api by ID from the database.
// @ID delete-v0-extensionApi
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-apis/{id} [DELETE]
func (h Handler) DeleteExtensionApi(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApi
	extensionApiID := c.Param("id")
	var extensionApi api_v0.ExtensionApi
	if result := h.DB.First(&extensionApi, extensionApiID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// delete object
	if result := h.DB.Delete(&extensionApi); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, extensionApi, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// ExtensionApiRoute
///////////////////////////////////////////////////////////////////////////////

// @Summary GetExtensionApiRouteVersions gets the supported versions for the extension api route API.
// @Description Get the supported API versions for extension api routes.
// @ID extensionApiRoute-get-versions
// @Produce json
// @Success 200 {object} apiserver_lib.ApiObjectVersions "OK"
// @Router /extension-api-routes/versions [GET]
func (h Handler) GetExtensionApiRouteVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, apiserver_lib.ObjectVersions[string(api_v0.ObjectTypeExtensionApiRoute)])
}

// @Summary adds a new extension api route.
// @Description Add a new extension api route to the Threeport database.
// @ID add-v0-extensionApiRoute
// @Accept json
// @Produce json
// @Param extensionApiRoute body api_v0.ExtensionApiRoute true "ExtensionApiRoute object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-api-routes [POST]
func (h Handler) AddExtensionApiRoute(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApiRoute
	var extensionApiRoute api_v0.ExtensionApiRoute

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, objectType, extensionApiRoute); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&extensionApiRoute); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, extensionApiRoute, objectType); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist to DB
	if result := h.DB.Create(&extensionApiRoute); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, extensionApiRoute, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

// @Summary gets all extension api routes.
// @Description Get all extension api routes from the Threeport database.
// @ID get-v0-extensionApiRoutes
// @Accept json
// @Produce json
// @Param name query string false "extension api route search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-api-routes [GET]
func (h Handler) GetExtensionApiRoutes(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApiRoute
	params, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	var filter api_v0.ExtensionApiRoute
	if err := c.Bind(&filter); err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&api_v0.ExtensionApiRoute{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]api_v0.ExtensionApiRoute{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary gets a extension api route.
// @Description Get a particular extension api route from the database.
// @ID get-v0-extensionApiRoute
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-api-routes/{id} [GET]
func (h Handler) GetExtensionApiRoute(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApiRoute
	extensionApiRouteID := c.Param("id")
	var extensionApiRoute api_v0.ExtensionApiRoute
	if result := h.DB.First(&extensionApiRoute, extensionApiRouteID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, extensionApiRoute, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing extension api route.
// @Description Update a extension api route in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating extension api route objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-v0-extensionApiRoute
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param extensionApiRoute body api_v0.ExtensionApiRoute true "ExtensionApiRoute object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-api-routes/{id} [PATCH]
func (h Handler) UpdateExtensionApiRoute(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApiRoute
	extensionApiRouteID := c.Param("id")
	var existingExtensionApiRoute api_v0.ExtensionApiRoute
	if result := h.DB.First(&existingExtensionApiRoute, extensionApiRouteID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, true, objectType, existingExtensionApiRoute); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedExtensionApiRoute api_v0.ExtensionApiRoute
	if err := c.Bind(&updatedExtensionApiRoute); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingExtensionApiRoute).Updates(updatedExtensionApiRoute); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, existingExtensionApiRoute, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary updates an existing extension api route by replacing the entire object.
// @Description Replace a extension api route in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating extension api route objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-v0-extensionApiRoute
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param extensionApiRoute body api_v0.ExtensionApiRoute true "ExtensionApiRoute object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-api-routes/{id} [PUT]
func (h Handler) ReplaceExtensionApiRoute(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApiRoute
	extensionApiRouteID := c.Param("id")
	var existingExtensionApiRoute api_v0.ExtensionApiRoute
	if result := h.DB.First(&existingExtensionApiRoute, extensionApiRouteID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, true, objectType, existingExtensionApiRoute); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedExtensionApiRoute api_v0.ExtensionApiRoute
	if err := c.Bind(&updatedExtensionApiRoute); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, updatedExtensionApiRoute, objectType); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedExtensionApiRoute.ID = existingExtensionApiRoute.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedExtensionApiRoute); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingExtensionApiRoute, extensionApiRouteID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, existingExtensionApiRoute, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary deletes a extension api route.
// @Description Delete a extension api route by ID from the database.
// @ID delete-v0-extensionApiRoute
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/extension-api-routes/{id} [DELETE]
func (h Handler) DeleteExtensionApiRoute(c echo.Context) error {
	objectType := api_v0.ObjectTypeExtensionApiRoute
	extensionApiRouteID := c.Param("id")
	var extensionApiRoute api_v0.ExtensionApiRoute
	if result := h.DB.First(&extensionApiRoute, extensionApiRouteID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// delete object
	if result := h.DB.Delete(&extensionApiRoute); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, extensionApiRoute, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
