package handlers

import (
	"errors"

	echo "github.com/labstack/echo/v4"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util_v0 "github.com/threeport/threeport/pkg/util/v0"
	zap "go.uber.org/zap"
	gorm "gorm.io/gorm"
)

// @Summary adds a new module api route with a module object reference.
// @Description Add a new module api route to the Threeport database with a module object reference.  This allows an API call to create a module api route that also populates the many-to-many relationship between the module api route and the module object.  This handler does not create the module object.  The module object must be created separately.
// @ID add-v0-moduleApiRouteWithModuleObjectReference
// @Accept json
// @Produce json
// @Param moduleApiRoute body api_v0.ModuleApiRoute true "ModuleApiRoute object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/module-api-route-with-module-object-reference [POST]
func (h Handler) AddModuleApiRouteWithModuleObjectReferences(c echo.Context) error {
	objectType := api_v0.ObjectTypeModuleApiRoute
	var moduleApiRoute api_v0.ModuleApiRoute

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, false, objectType, moduleApiRoute); err != nil {
		h.Logger.Error("handler error: error performing payload check", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&moduleApiRoute); err != nil {
		h.Logger.Error("handler error: error binding object", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, moduleApiRoute, objectType); err != nil {
		h.Logger.Error("handler error: error validating bound data", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist to DB
	if result := h.DB.Omit("ModuleObjects.*").Create(&moduleApiRoute); result.Error != nil {
		h.Logger.Error("handler error: error creating object", zap.Error(result.Error))
		// check if this is a custom HTTP error with specific status code
		var httpErr *util_v0.HttpError
		if errors.As(result.Error, &httpErr) {
			return apiserver_lib.ResponseStatusErr(
				httpErr.GetStatusCode(), c, nil, result.Error, objectType,
			)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, moduleApiRoute, objectType)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

// @Summary gets all module objects with associated module api routes.
// @Description Get all module objects from the Threeport database with associated module api routes.
// @ID get-v0-moduleObjectsModuleApiRoutes
// @Accept json
// @Produce json
// @Param name query string false "module object search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/module-objects-with-module-api-routes [GET]
func (h Handler) GetModuleObjectsWithModuleApiRoutes(c echo.Context) error {
	objectType := api_v0.ObjectTypeModuleObject
	params, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	var filter api_v0.ModuleObject
	if err := c.Bind(&filter); err != nil {
		h.Logger.Error("handler error: error binding filter", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&api_v0.ModuleObject{}).Where(&filter).Count(&totalCount); result.Error != nil {
		h.Logger.Error("handler error: error counting objects", zap.Error(result.Error))
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]api_v0.ModuleObject{}
	if result := h.DB.Order("ID asc").Where(&filter).Preload("ModuleApiRoutes").Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary gets a module object by ID with associated module api routes.
// @Description Get a module object from the Threeport database by ID with associated module api routes.
// @ID get-v0-moduleObjectWithModuleApiRoutesByID
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/module-objects-with-module-api-routes/{id} [GET]
func (h Handler) GetModuleObjectWithModuleApiRoutes(c echo.Context) error {
	objectType := api_v0.ObjectTypeModuleObject
	moduleObjectID := c.Param("id")
	var moduleObject api_v0.ModuleObject
	if result := h.DB.Preload("ModuleApiRoutes").First(&moduleObject, moduleObjectID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		h.Logger.Error("handler error: error finding object", zap.Error(result.Error))
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, moduleObject, objectType)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
