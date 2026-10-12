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
	fullyQualifiedType := new(api_v0.ModuleApiRoute).GetFullyQualifiedType()
	var moduleApiRoute api_v0.ModuleApiRoute

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, false, objectType, moduleApiRoute); err != nil {
		h.Logger.Error("handler error: error performing payload check", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), fullyQualifiedType)
	}

	if err := c.Bind(&moduleApiRoute); err != nil {
		h.Logger.Error("handler error: error binding object", zap.Error(err))
		return apiserver_lib.ResponseStatusBindErr(c, nil, err, fullyQualifiedType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, moduleApiRoute, objectType); err != nil {
		h.Logger.Error("handler error: error validating bound data", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), fullyQualifiedType)
	}

	// persist to DB
	if result := h.Write(c, func(db *gorm.DB) *gorm.DB {
		// clear id so a retried create does not reuse a rolled-back key
		moduleApiRoute.ID = nil
		return db.Omit("ModuleObjects.*").Create(&moduleApiRoute)
	}); result.Error != nil {
		h.Logger.Error("handler error: error creating object", zap.Error(result.Error))
		// check if this is a custom HTTP error with specific status code
		var httpErr *util_v0.HttpError
		if errors.As(result.Error, &httpErr) {
			return apiserver_lib.ResponseStatusErr(
				httpErr.GetStatusCode(), c, nil, result.Error, fullyQualifiedType,
			)
		}
		return apiserver_lib.RespondWriteError(
			c,
			h.Logger,
			result.Error,
			new(api_v0.ModuleApiRoute),
			fullyQualifiedType,
		)
	}

	// the write has committed; register the route on the module router before
	// answering, so a module that gets a 201 can rely on its path being served.
	// This is the path modules actually register through, so it needs the same
	// post-commit reconciliation the generated handlers get.
	if err := apiserver_lib.AfterCommitCreate(h.DB, &moduleApiRoute); err != nil {
		h.Logger.Error("handler error: error reconciling process state after commit", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	response, err := apiserver_lib.CreateResponse(
		apiserver_lib.SingleObjectMeta(),
		moduleApiRoute,
		fullyQualifiedType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, fullyQualifiedType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

// moduleControllerConflictError reports that a module controller cannot be
// reclaimed because another module api holds it. It maps to a 409.
type moduleControllerConflictError struct {
	message string
}

func (e *moduleControllerConflictError) Error() string {
	return e.message
}

// @Summary reclaims a module controller whose owning module api is gone.
// @Description Rebinds the module controller with the provided name to the provided module api, but only when the module api that currently owns it no longer exists. The ownership check and the rebind run in one database transaction, so an owner that re-registers between the two cannot lose its controller. If the current owner still exists the request is rejected with a 409. If the controller is already bound to the provided module api the existing object is returned unchanged. If no controller with that name exists it is created.
// @ID reclaim-v0-moduleController
// @Accept json
// @Produce json
// @Param moduleController body api_v0.ModuleController true "ModuleController object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/module-controller-reclaim [POST]
func (h Handler) ReclaimModuleController(c echo.Context) error {
	objectType := api_v0.ObjectTypeModuleController
	fullyQualifiedType := new(api_v0.ModuleController).GetFullyQualifiedType()
	var moduleController api_v0.ModuleController

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, false, objectType, moduleController); err != nil {
		h.Logger.Error("handler error: error performing payload check", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), fullyQualifiedType)
	}

	if err := c.Bind(&moduleController); err != nil {
		h.Logger.Error("handler error: error binding object", zap.Error(err))
		return apiserver_lib.ResponseStatusBindErr(c, nil, err, fullyQualifiedType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, moduleController, objectType); err != nil {
		h.Logger.Error("handler error: error validating bound data", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), fullyQualifiedType)
	}

	// The owner check and the rebind share one transaction. Under serializable
	// isolation the database aborts one side of a concurrent conflict with
	// SQLSTATE 40001 and RetryWrite reruns it against the committed state. This
	// does not use Write: request query scopes such as includedeleted must not
	// change which owners count as live.
	ctx := c.Request().Context()
	var reclaimed api_v0.ModuleController
	result := apiserver_lib.RetryWrite(ctx, func() *gorm.DB {
		db := h.DB.WithContext(ctx)
		// drop the result of an aborted attempt if the write retries
		reclaimed = api_v0.ModuleController{}
		err := db.Transaction(func(tx *gorm.DB) error {
			return reclaimModuleController(tx, &moduleController, &reclaimed)
		})
		if err != nil {
			_ = db.AddError(err)
		}
		return db
	})
	if result.Error != nil {
		var conflictErr *moduleControllerConflictError
		if errors.As(result.Error, &conflictErr) {
			h.Logger.Info("handler error: module controller reclaim rejected", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus409(c, nil, result.Error, fullyQualifiedType)
		}
		h.Logger.Error("handler error: error reclaiming module controller", zap.Error(result.Error))
		return apiserver_lib.RespondWriteError(
			c,
			h.Logger,
			result.Error,
			new(api_v0.ModuleController),
			fullyQualifiedType,
		)
	}

	response, err := apiserver_lib.CreateResponse(
		apiserver_lib.SingleObjectMeta(),
		reclaimed,
		fullyQualifiedType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, fullyQualifiedType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// reclaimModuleController binds the module controller named in requested to
// requested.ModuleApiID, if its current owner is gone, and stores the resulting
// row in out. It must run inside one transaction. The final update names the
// owner it observed, so a rebind that lands between the read and the write
// changes no rows and the call fails with a conflict even where the database
// does not abort the transaction itself.
//
// ModuleApiID is a `requires` foreign key, which the model's update hook treats
// as immutable once set, so an ordinary update cannot move it. Reclaiming a row
// from a vanished owner is the one sanctioned change, so the rebind skips the
// hooks and swaps the attached object reference itself, in the same
// transaction as the owner check.
func reclaimModuleController(
	tx *gorm.DB,
	requested *api_v0.ModuleController,
	out *api_v0.ModuleController,
) error {
	var existing api_v0.ModuleController
	err := tx.Where("name = ?", *requested.Name).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// the row that conflicted with the caller's create is gone, so there is
		// nothing to reclaim; create it for the caller
		created := *requested
		created.ID = nil
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		*out = created
		return nil
	case err != nil:
		return err
	}

	// the registering module already holds the controller
	if existing.ModuleApiID != nil && *existing.ModuleApiID == *requested.ModuleApiID {
		*out = existing
		return nil
	}

	// a different module api holds it; only an owner that is gone can be replaced
	if existing.ModuleApiID != nil {
		var owners int64
		if err := tx.Model(&api_v0.ModuleApi{}).
			Where("id = ?", *existing.ModuleApiID).
			Count(&owners).Error; err != nil {
			return err
		}
		if owners > 0 {
			return &moduleControllerConflictError{message: fmt.Sprintf(
				"module controller %q is already owned by a different, active module api %d",
				*requested.Name, *existing.ModuleApiID,
			)}
		}
	}

	// rebind only if the row still has the owner we just checked
	update := tx.Session(&gorm.Session{SkipHooks: true}).Model(&existing).Where("id = ?", *existing.ID)
	if existing.ModuleApiID != nil {
		update = update.Where("module_api_id = ?", *existing.ModuleApiID)
	} else {
		update = update.Where("module_api_id IS NULL")
	}
	update = update.Updates(map[string]interface{}{
		"module_api_id":   *requested.ModuleApiID,
		"deployment_name": *requested.DeploymentName,
	})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected == 0 {
		return &moduleControllerConflictError{message: fmt.Sprintf(
			"module controller %q was changed by another request during the reclaim",
			*requested.Name,
		)}
	}

	// point the `requires` reference at the new owner and drop the one that
	// named the owner that is gone
	moduleApiType := new(api_v0.ModuleApi).GetFullyQualifiedType()
	controllerType := new(api_v0.ModuleController).GetFullyQualifiedType()
	requires := api_v0.RelationshipRequires
	if err := tx.Where(
		"object_type = ? AND attached_object_type = ? AND attached_object_id = ? AND relationship = ?",
		moduleApiType, controllerType, *existing.ID, requires,
	).Delete(&api_v0.AttachedObjectReference{}).Error; err != nil {
		return fmt.Errorf("failed to drop the reference to the prior module api: %w", err)
	}
	if err := tx.Create(&api_v0.AttachedObjectReference{
		ObjectType:         &moduleApiType,
		ObjectID:           requested.ModuleApiID,
		AttachedObjectType: &controllerType,
		AttachedObjectID:   existing.ID,
		Relationship:       &requires,
	}).Error; err != nil {
		return fmt.Errorf("failed to reference the new module api: %w", err)
	}

	return tx.First(out, *existing.ID).Error
}

// fetchModuleObjectPage fetches one page of module objects through the
// configured pagination strategy, then reloads that page's rows together with
// their api-route associations. It stamps the queryId and next cursor onto
// pagination, and returns the page's records alongside the row count the fetch
// produced before the association reload, which is the count that says whether
// more pages remain.
//
// The association reload reads current state rather than the pagination
// snapshot. The api-route list per object is small and stable, so drift within
// one page is acceptable. It answers to the same request scoping as the fetch,
// and the next cursor is taken from the page as fetched, so a row the reload
// misses cannot pull the cursor backward and serve the same rows again.
func (h Handler) fetchModuleObjectPage(
	c echo.Context,
	filter *api_v0.ModuleObject,
	pageParams *apiserver_lib.PageRequestParams,
	pagination *apiserver_lib.Pagination,
) (*[]api_v0.ModuleObject, int64, error) {
	records := &[]api_v0.ModuleObject{}

	queryId, fetchedCount, err := h.DispatchGetPaginatedRecords(
		h.RequestDB(c).Model(&api_v0.ModuleObject{}).Where(filter),
		records,
		filter.TableName(),
		pageParams,
	)
	if err != nil {
		return nil, 0, err
	}
	pagination.QueryId = queryId

	if len(*records) > 0 {
		pagination.NextCursor = *(*records)[len(*records)-1].ID
	} else {
		pagination.NextCursor = 0
	}

	if len(*records) > 0 {
		var ids []uint
		for _, record := range *records {
			if record.ID != nil {
				ids = append(ids, *record.ID)
			}
		}
		if len(ids) > 0 {
			recordsWithPreload := &[]api_v0.ModuleObject{}
			if result := h.RequestDB(c).Where("id IN ?", ids).Preload("ModuleApiRoutes").Find(recordsWithPreload); result.Error != nil {
				return nil, 0, fmt.Errorf("handler error: error preloading associations: %w", result.Error)
			}
			records = recordsWithPreload
		}
	}

	return records, fetchedCount, nil
}

// @Summary gets all module objects with associated module api routes.
// @Description Get all module objects from the Threeport database with associated module api routes.
// @ID get-v0-moduleObjectsModuleApiRoutes
// @Accept json
// @Produce json
// @Param name query string false "filter by exact module object name (case sensitive)"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/module-objects-with-module-api-routes [GET]
func (h Handler) GetModuleObjectsWithModuleApiRoutes(c echo.Context) error {
	fullyQualifiedType := new(api_v0.ModuleObject).GetFullyQualifiedType()

	// get pagination parameters
	pageParams, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, pageParams, err, fullyQualifiedType)
	}

	// bind filter
	var filter api_v0.ModuleObject
	if err := c.Bind(&filter); err != nil {
		h.Logger.Error("handler error: error binding filter", zap.Error(err))
		return apiserver_lib.ResponseStatus400(c, pageParams, err, fullyQualifiedType)
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
		if result := h.RequestDB(c).Model(&api_v0.ModuleObject{}).Where(&filter).Count(&totalCount); result.Error != nil {
			h.Logger.Error("handler error: error counting objects", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, fullyQualifiedType)
		}

		// see if total count is greater than the limit
		pagination.HasMore = totalCount > pagination.Limit

		switch pagination.HasMore {
		case false:
			// if we don't have to paginate, return all records
			if result := h.RequestDB(c).Order("ID asc").Where(&filter).Preload("ModuleApiRoutes").Find(records); result.Error != nil {
				h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, fullyQualifiedType)
			}
			returnedCount = int64(len(*records))

		case true:
			// paginate: fetch the first page. HasMore is already known
			// from the total count above, so the fetched count is unused
			page, _, err := h.fetchModuleObjectPage(c, &filter, pageParams, pagination)
			if err != nil {
				if errors.Is(err, apiserver_lib.ErrInvalidPaginationQueryId) || errors.Is(err, apiserver_lib.ErrPaginationSessionExpired) {
					return apiserver_lib.ResponseStatus400(c, pageParams, err, fullyQualifiedType)
				}
				h.Logger.Error("handler error: error fetching paginated records", zap.Error(err))
				return apiserver_lib.ResponseStatus500(c, pageParams, err, fullyQualifiedType)
			}
			records = page

			// count what ships, not what the paginated fetch returned
			returnedCount = int64(len(*records))
		}

	case pageParams.QueryId != "" && pageParams.Cursor == 0:
		// client provided a query ID but no cursor, so we cannot fetch the next page of results
		return apiserver_lib.ResponseStatus400(c, pageParams, errors.New("cursor is required when query ID is provided"), fullyQualifiedType)

	case pageParams.QueryId != "" && pageParams.Cursor != 0:
		// continuation: fetch the next page. the queryId round-trips
		// opaquely, so both strategies resume from the snapshot they
		// anchored on the first page
		page, fetchedCount, err := h.fetchModuleObjectPage(c, &filter, pageParams, pagination)
		if err != nil {
			if errors.Is(err, apiserver_lib.ErrInvalidPaginationQueryId) || errors.Is(err, apiserver_lib.ErrPaginationSessionExpired) {
				return apiserver_lib.ResponseStatus400(c, pageParams, err, fullyQualifiedType)
			}
			h.Logger.Error("handler error: error fetching paginated records", zap.Error(err))
			return apiserver_lib.ResponseStatus500(c, pageParams, err, fullyQualifiedType)
		}
		records = page

		// count what ships, not what the paginated fetch returned
		returnedCount = int64(len(*records))

		// a full page from the paginated fetch means rows remain, so test
		// the fetched count rather than the shipped count
		pagination.HasMore = fetchedCount >= pagination.Limit
	}

	// construct response
	response, err := apiserver_lib.CreateResponse(
		&apiserver_lib.Meta{
			Pagination:  *pagination,
			ObjectCount: returnedCount,
		},
		*records,
		fullyQualifiedType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, pageParams, err, fullyQualifiedType)
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
	fullyQualifiedType := new(api_v0.ModuleObject).GetFullyQualifiedType()
	moduleObjectID := c.Param("id")
	var moduleObject api_v0.ModuleObject
	if result := h.DB.Preload("ModuleApiRoutes").First(&moduleObject, moduleObjectID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, fullyQualifiedType)
		}
		h.Logger.Error("handler error: error finding object", zap.Error(result.Error))
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, fullyQualifiedType)
	}

	response, err := apiserver_lib.CreateResponse(
		apiserver_lib.SingleObjectMeta(),
		moduleObject,
		fullyQualifiedType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, fullyQualifiedType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
