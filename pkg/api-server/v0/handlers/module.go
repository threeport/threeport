package handlers

import (
	"errors"
	"fmt"

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

	response, err := apiserver_lib.CreateResponse(
		apiserver_lib.SingleObjectMeta(),
		moduleApiRoute,
		objectType,
	)
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

	// get pagination parameters
	pageParams, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, pageParams, err, objectType)
	}

	// bind filter
	var filter api_v0.ModuleObject
	if err := c.Bind(&filter); err != nil {
		h.Logger.Error("handler error: error binding filter", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
	}

	pagination := new(apiserver_lib.Pagination)
	pagination.Limit = pageParams.Limit

	records := &[]api_v0.ModuleObject{}
	var returnedCount int64

	switch {
	case pageParams.QueryId == "":
		// no query ID provided, so the client is not requesting a specific page of results
		// count total number of objects
		var totalCount int64
		if result := h.DB.Model(&api_v0.ModuleObject{}).Where(&filter).Count(&totalCount); result.Error != nil {
			h.Logger.Error("handler error: error counting objects", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
		}

		// see if total count is greater than the limit
		pagination.HasMore = totalCount > pagination.Limit

		switch pagination.HasMore {
		case false:
			// if we don't have to paginate, return all records
			if result := h.DB.Order("ID asc").Where(&filter).Preload("ModuleApiRoutes").Find(records); result.Error != nil {
				h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
			}
			returnedCount = int64(len(*records))

		case true:
			// if we have to paginate, create the materialized view and use it to fetch the first page of records
			viewName, queryId, err := h.CreateMaterializedView(objectType)
			if err != nil {
				h.Logger.Error("handler error: error creating materialized view", zap.Error(err))
				return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
			}
			pagination.QueryId = queryId

			query := fmt.Sprintf("SELECT * FROM %s ORDER BY ID ASC LIMIT %d", viewName, pageParams.Limit)
			if result := h.DB.Raw(query).Find(records); result.Error != nil {
				h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
			}

			// load associations for the records retrieved from materialized view
			if len(*records) > 0 {
				var ids []uint
				for _, record := range *records {
					if record.ID != nil {
						ids = append(ids, *record.ID)
					}
				}
				if len(ids) > 0 {
					recordsWithPreload := &[]api_v0.ModuleObject{}
					if result := h.DB.Where("id IN ?", ids).Preload("ModuleApiRoutes").Find(recordsWithPreload); result.Error != nil {
						h.Logger.Error("handler error: error preloading associations", zap.Error(result.Error))
						return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
					}
					records = recordsWithPreload
				}
			}

			returnedCount = int64(len(*records))
			if len(*records) > 0 {
				pagination.NextCursor = *(*records)[len(*records)-1].ID
			} else {
				pagination.NextCursor = 0
			}
		}

	case pageParams.QueryId != "" && pageParams.Cursor == 0:
		// client provided a query ID but no cursor, so we cannot fetch the next page of results
		return apiserver_lib.ResponseStatus400(c, pageParams, errors.New("cursor is required when query ID is provided"), objectType)

	case pageParams.QueryId != "" && pageParams.Cursor != 0:
		// use query ID to find the materialized view name
		viewName, err := h.GetMaterializedViewName(pageParams.QueryId)
		if err != nil {
			h.Logger.Error("handler error: error finding materialized view", zap.Error(err))
			return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
		}

		pagination.QueryId = pageParams.QueryId

		// fetch records from the materialized view based on cursor
		returnedCount, err = h.GetMaterializedViewRecords(records, viewName, pageParams)
		if err != nil {
			h.Logger.Error("handler error: error finding records", zap.Error(err))
			return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
		}

		// load associations for the records retrieved from materialized view
		if len(*records) > 0 {
			var ids []uint
			for _, record := range *records {
				if record.ID != nil {
					ids = append(ids, *record.ID)
				}
			}
			if len(ids) > 0 {
				recordsWithPreload := &[]api_v0.ModuleObject{}
				if result := h.DB.Where("id IN ?", ids).Preload("ModuleApiRoutes").Find(recordsWithPreload); result.Error != nil {
					h.Logger.Error("handler error: error preloading associations", zap.Error(result.Error))
					return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
				}
				records = recordsWithPreload
			}
		}

		// set the next cursor
		if len(*records) > 0 {
			pagination.NextCursor = *(*records)[len(*records)-1].ID
		} else {
			pagination.NextCursor = 0
		}

		// see if we fetched the last of the records
		pagination.HasMore = returnedCount >= pagination.Limit
	}

	// construct response
	response, err := apiserver_lib.CreateResponse(
		&apiserver_lib.Meta{
			Pagination:  *pagination,
			ObjectCount: returnedCount,
		},
		*records,
		objectType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
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

	response, err := apiserver_lib.CreateResponse(
		apiserver_lib.SingleObjectMeta(),
		moduleObject,
		objectType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
