package handlers

import (
	"errors"

	echo "github.com/labstack/echo/v4"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util_v0 "github.com/threeport/threeport/pkg/util/v0"
	zap "go.uber.org/zap"
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
