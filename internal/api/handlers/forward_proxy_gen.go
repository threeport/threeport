// generated by 'threeport-codegen api-model' - do not edit

package handlers

import (
	"errors"
	echo "github.com/labstack/echo/v4"
	iapi "github.com/threeport/threeport/internal/api"
	api "github.com/threeport/threeport/pkg/api"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	gorm "gorm.io/gorm"
	"net/http"
)

///////////////////////////////////////////////////////////////////////////////
// ForwardProxyDefinition
///////////////////////////////////////////////////////////////////////////////

// @Summary GetForwardProxyDefinitionVersions gets the supported versions for the forward proxy definition API.
// @Description Get the supported API versions for forward proxy definitions.
// @ID forwardProxyDefinition-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /forward-proxy-definitions/versions [get]
func (h Handler) GetForwardProxyDefinitionVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeForwardProxyDefinition)])
}

// @Summary adds a new forward proxy definition.
// @Description Add a new forward proxy definition to the Threeport database.
// @ID add-forwardProxyDefinition
// @Accept json
// @Produce json
// @Param forwardProxyDefinition body v0.ForwardProxyDefinition true "ForwardProxyDefinition object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-definitions [post]
func (h Handler) AddForwardProxyDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyDefinition
	var forwardProxyDefinition v0.ForwardProxyDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&forwardProxyDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, forwardProxyDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if result := h.DB.Create(&forwardProxyDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifyPayload, err := forwardProxyDefinition.NotificationPayload(false, 0, "Created")

	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ForwardProxyDefinitionCreateSubject, *notifyPayload)

	response, err := v0.CreateResponse(nil, forwardProxyDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all forward proxy definitions.
// @Description Get all forward proxy definitions from the Threeport database.
// @ID get-forwardProxyDefinitions
// @Accept json
// @Produce json
// @Param name query string false "forward proxy definition search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-definitions [get]
func (h Handler) GetForwardProxyDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyDefinition
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.ForwardProxyDefinition
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.ForwardProxyDefinition{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.ForwardProxyDefinition{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a forward proxy definition.
// @Description Get a particular forward proxy definition from the database.
// @ID get-forwardProxyDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-definitions/{id} [get]
func (h Handler) GetForwardProxyDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyDefinition
	forwardProxyDefinitionID := c.Param("id")
	var forwardProxyDefinition v0.ForwardProxyDefinition
	if result := h.DB.First(&forwardProxyDefinition, forwardProxyDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, forwardProxyDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing forward proxy definition.
// @Description Update a forward proxy definition in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating forward proxy definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-forwardProxyDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param forwardProxyDefinition body v0.ForwardProxyDefinition true "ForwardProxyDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-definitions/{id} [patch]
func (h Handler) UpdateForwardProxyDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyDefinition
	forwardProxyDefinitionID := c.Param("id")
	var existingForwardProxyDefinition v0.ForwardProxyDefinition
	if result := h.DB.First(&existingForwardProxyDefinition, forwardProxyDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedForwardProxyDefinition v0.ForwardProxyDefinition
	if err := c.Bind(&updatedForwardProxyDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	if result := h.DB.Model(&existingForwardProxyDefinition).Updates(updatedForwardProxyDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifyPayload, err := updatedForwardProxyDefinition.NotificationPayload(false, 0, "Updated")

	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ForwardProxyDefinitionCreateSubject, *notifyPayload)

	response, err := v0.CreateResponse(nil, existingForwardProxyDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing forward proxy definition by replacing the entire object.
// @Description Replace a forward proxy definition in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating forward proxy definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-forwardProxyDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param forwardProxyDefinition body v0.ForwardProxyDefinition true "ForwardProxyDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-definitions/{id} [put]
func (h Handler) ReplaceForwardProxyDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyDefinition
	forwardProxyDefinitionID := c.Param("id")
	var existingForwardProxyDefinition v0.ForwardProxyDefinition
	if result := h.DB.First(&existingForwardProxyDefinition, forwardProxyDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedForwardProxyDefinition v0.ForwardProxyDefinition
	if err := c.Bind(&updatedForwardProxyDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedForwardProxyDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedForwardProxyDefinition.ID = existingForwardProxyDefinition.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedForwardProxyDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingForwardProxyDefinition, forwardProxyDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingForwardProxyDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a forward proxy definition.
// @Description Delete a forward proxy definition by from the database.
// @ID delete-forwardProxyDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-definitions/{id} [delete]
func (h Handler) DeleteForwardProxyDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyDefinition
	forwardProxyDefinitionID := c.Param("id")
	var forwardProxyDefinition v0.ForwardProxyDefinition
	if result := h.DB.First(&forwardProxyDefinition, forwardProxyDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&forwardProxyDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifyPayload, err := forwardProxyDefinition.NotificationPayload(false, 0, "Deleted")

	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ForwardProxyDefinitionCreateSubject, *notifyPayload)

	response, err := v0.CreateResponse(nil, forwardProxyDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// ForwardProxyInstance
///////////////////////////////////////////////////////////////////////////////

// @Summary GetForwardProxyInstanceVersions gets the supported versions for the forward proxy instance API.
// @Description Get the supported API versions for forward proxy instances.
// @ID forwardProxyInstance-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /forward-proxy-instances/versions [get]
func (h Handler) GetForwardProxyInstanceVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeForwardProxyInstance)])
}

// @Summary adds a new forward proxy instance.
// @Description Add a new forward proxy instance to the Threeport database.
// @ID add-forwardProxyInstance
// @Accept json
// @Produce json
// @Param forwardProxyInstance body v0.ForwardProxyInstance true "ForwardProxyInstance object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-instances [post]
func (h Handler) AddForwardProxyInstance(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyInstance
	var forwardProxyInstance v0.ForwardProxyInstance

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&forwardProxyInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, forwardProxyInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if result := h.DB.Create(&forwardProxyInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifyPayload, err := forwardProxyInstance.NotificationPayload(false, 0, "Created")

	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ForwardProxyInstanceCreateSubject, *notifyPayload)

	response, err := v0.CreateResponse(nil, forwardProxyInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all forward proxy instances.
// @Description Get all forward proxy instances from the Threeport database.
// @ID get-forwardProxyInstances
// @Accept json
// @Produce json
// @Param name query string false "forward proxy instance search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-instances [get]
func (h Handler) GetForwardProxyInstances(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyInstance
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.ForwardProxyInstance
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.ForwardProxyInstance{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.ForwardProxyInstance{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a forward proxy instance.
// @Description Get a particular forward proxy instance from the database.
// @ID get-forwardProxyInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-instances/{id} [get]
func (h Handler) GetForwardProxyInstance(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyInstance
	forwardProxyInstanceID := c.Param("id")
	var forwardProxyInstance v0.ForwardProxyInstance
	if result := h.DB.First(&forwardProxyInstance, forwardProxyInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, forwardProxyInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing forward proxy instance.
// @Description Update a forward proxy instance in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating forward proxy instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-forwardProxyInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param forwardProxyInstance body v0.ForwardProxyInstance true "ForwardProxyInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-instances/{id} [patch]
func (h Handler) UpdateForwardProxyInstance(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyInstance
	forwardProxyInstanceID := c.Param("id")
	var existingForwardProxyInstance v0.ForwardProxyInstance
	if result := h.DB.First(&existingForwardProxyInstance, forwardProxyInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedForwardProxyInstance v0.ForwardProxyInstance
	if err := c.Bind(&updatedForwardProxyInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	if result := h.DB.Model(&existingForwardProxyInstance).Updates(updatedForwardProxyInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifyPayload, err := updatedForwardProxyInstance.NotificationPayload(false, 0, "Updated")

	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ForwardProxyInstanceCreateSubject, *notifyPayload)

	response, err := v0.CreateResponse(nil, existingForwardProxyInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing forward proxy instance by replacing the entire object.
// @Description Replace a forward proxy instance in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating forward proxy instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-forwardProxyInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param forwardProxyInstance body v0.ForwardProxyInstance true "ForwardProxyInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-instances/{id} [put]
func (h Handler) ReplaceForwardProxyInstance(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyInstance
	forwardProxyInstanceID := c.Param("id")
	var existingForwardProxyInstance v0.ForwardProxyInstance
	if result := h.DB.First(&existingForwardProxyInstance, forwardProxyInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedForwardProxyInstance v0.ForwardProxyInstance
	if err := c.Bind(&updatedForwardProxyInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedForwardProxyInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedForwardProxyInstance.ID = existingForwardProxyInstance.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedForwardProxyInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingForwardProxyInstance, forwardProxyInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingForwardProxyInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a forward proxy instance.
// @Description Delete a forward proxy instance by from the database.
// @ID delete-forwardProxyInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/forward-proxy-instances/{id} [delete]
func (h Handler) DeleteForwardProxyInstance(c echo.Context) error {
	objectType := v0.ObjectTypeForwardProxyInstance
	forwardProxyInstanceID := c.Param("id")
	var forwardProxyInstance v0.ForwardProxyInstance
	if result := h.DB.First(&forwardProxyInstance, forwardProxyInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&forwardProxyInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifyPayload, err := forwardProxyInstance.NotificationPayload(false, 0, "Deleted")

	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ForwardProxyInstanceCreateSubject, *notifyPayload)

	response, err := v0.CreateResponse(nil, forwardProxyInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
