// generated by 'threeport-codegen api-model' - do not edit

package handlers

import (
	"errors"
	echo "github.com/labstack/echo/v4"
	iapi "github.com/threeport/threeport/internal/api"
	api "github.com/threeport/threeport/pkg/api"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	gorm "gorm.io/gorm"
	"net/http"
)

///////////////////////////////////////////////////////////////////////////////
// GatewayDefinition
///////////////////////////////////////////////////////////////////////////////

// @Summary GetGatewayDefinitionVersions gets the supported versions for the gateway definition API.
// @Description Get the supported API versions for gateway definitions.
// @ID gatewayDefinition-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /gateway-definitions/versions [get]
func (h Handler) GetGatewayDefinitionVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeGatewayDefinition)])
}

// @Summary adds a new gateway definition.
// @Description Add a new gateway definition to the Threeport database.
// @ID add-gatewayDefinition
// @Accept json
// @Produce json
// @Param gatewayDefinition body v0.GatewayDefinition true "GatewayDefinition object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-definitions [post]
func (h Handler) AddGatewayDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayDefinition
	var gatewayDefinition v0.GatewayDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&gatewayDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, gatewayDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingGatewayDefinition v0.GatewayDefinition
	nameUsed := true
	result := h.DB.Where("name = ?", gatewayDefinition.Name).First(&existingGatewayDefinition)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			nameUsed = false
		} else {
			return iapi.ResponseStatus500(c, nil, result.Error, objectType)
		}
	}
	if nameUsed {
		return iapi.ResponseStatus409(c, nil, errors.New("object with provided name already exists"), objectType)
	}

	// persist to DB
	if result := h.DB.Create(&gatewayDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := gatewayDefinition.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.GatewayDefinitionCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, gatewayDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all gateway definitions.
// @Description Get all gateway definitions from the Threeport database.
// @ID get-gatewayDefinitions
// @Accept json
// @Produce json
// @Param name query string false "gateway definition search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-definitions [get]
func (h Handler) GetGatewayDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayDefinition
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.GatewayDefinition
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.GatewayDefinition{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.GatewayDefinition{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a gateway definition.
// @Description Get a particular gateway definition from the database.
// @ID get-gatewayDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-definitions/{id} [get]
func (h Handler) GetGatewayDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayDefinition
	gatewayDefinitionID := c.Param("id")
	var gatewayDefinition v0.GatewayDefinition
	if result := h.DB.First(&gatewayDefinition, gatewayDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, gatewayDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing gateway definition.
// @Description Update a gateway definition in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating gateway definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-gatewayDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param gatewayDefinition body v0.GatewayDefinition true "GatewayDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-definitions/{id} [patch]
func (h Handler) UpdateGatewayDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayDefinition
	gatewayDefinitionID := c.Param("id")
	var existingGatewayDefinition v0.GatewayDefinition
	if result := h.DB.First(&existingGatewayDefinition, gatewayDefinitionID); result.Error != nil {
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
	var updatedGatewayDefinition v0.GatewayDefinition
	if err := c.Bind(&updatedGatewayDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// if client doesn't specify reconciled, set it to false
	if updatedGatewayDefinition.Reconciled == nil {
		reconciled := false
		updatedGatewayDefinition.Reconciled = &reconciled
	}

	// update object in database
	if result := h.DB.Model(&existingGatewayDefinition).Updates(updatedGatewayDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controllers if reconciliation is required
	if !*existingGatewayDefinition.Reconciled {
		notifPayload, err := existingGatewayDefinition.NotificationPayload(
			notifications.NotificationOperationUpdated,
			false,
			0,
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.GatewayDefinitionUpdateSubject, *notifPayload)
	}

	response, err := v0.CreateResponse(nil, existingGatewayDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing gateway definition by replacing the entire object.
// @Description Replace a gateway definition in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating gateway definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-gatewayDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param gatewayDefinition body v0.GatewayDefinition true "GatewayDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-definitions/{id} [put]
func (h Handler) ReplaceGatewayDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayDefinition
	gatewayDefinitionID := c.Param("id")
	var existingGatewayDefinition v0.GatewayDefinition
	if result := h.DB.First(&existingGatewayDefinition, gatewayDefinitionID); result.Error != nil {
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
	var updatedGatewayDefinition v0.GatewayDefinition
	if err := c.Bind(&updatedGatewayDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedGatewayDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedGatewayDefinition.ID = existingGatewayDefinition.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedGatewayDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingGatewayDefinition, gatewayDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingGatewayDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a gateway definition.
// @Description Delete a gateway definition by ID from the database.
// @ID delete-gatewayDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-definitions/{id} [delete]
func (h Handler) DeleteGatewayDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayDefinition
	gatewayDefinitionID := c.Param("id")
	var gatewayDefinition v0.GatewayDefinition
	if result := h.DB.Preload("GatewayInstances").First(&gatewayDefinition, gatewayDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check to make sure no dependent instances exist for this definition
	if len(gatewayDefinition.GatewayInstances) != 0 {
		err := errors.New("gateway definition has related gateway instances - cannot be deleted")
		return iapi.ResponseStatus409(c, nil, err, objectType)
	}

	if result := h.DB.Delete(&gatewayDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := gatewayDefinition.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.GatewayDefinitionDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, gatewayDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// GatewayInstance
///////////////////////////////////////////////////////////////////////////////

// @Summary GetGatewayInstanceVersions gets the supported versions for the gateway instance API.
// @Description Get the supported API versions for gateway instances.
// @ID gatewayInstance-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /gateway-instances/versions [get]
func (h Handler) GetGatewayInstanceVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeGatewayInstance)])
}

// @Summary adds a new gateway instance.
// @Description Add a new gateway instance to the Threeport database.
// @ID add-gatewayInstance
// @Accept json
// @Produce json
// @Param gatewayInstance body v0.GatewayInstance true "GatewayInstance object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-instances [post]
func (h Handler) AddGatewayInstance(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayInstance
	var gatewayInstance v0.GatewayInstance

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&gatewayInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, gatewayInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingGatewayInstance v0.GatewayInstance
	nameUsed := true
	result := h.DB.Where("name = ?", gatewayInstance.Name).First(&existingGatewayInstance)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			nameUsed = false
		} else {
			return iapi.ResponseStatus500(c, nil, result.Error, objectType)
		}
	}
	if nameUsed {
		return iapi.ResponseStatus409(c, nil, errors.New("object with provided name already exists"), objectType)
	}

	// persist to DB
	if result := h.DB.Create(&gatewayInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := gatewayInstance.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.GatewayInstanceCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, gatewayInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all gateway instances.
// @Description Get all gateway instances from the Threeport database.
// @ID get-gatewayInstances
// @Accept json
// @Produce json
// @Param name query string false "gateway instance search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-instances [get]
func (h Handler) GetGatewayInstances(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayInstance
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.GatewayInstance
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.GatewayInstance{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.GatewayInstance{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a gateway instance.
// @Description Get a particular gateway instance from the database.
// @ID get-gatewayInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-instances/{id} [get]
func (h Handler) GetGatewayInstance(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayInstance
	gatewayInstanceID := c.Param("id")
	var gatewayInstance v0.GatewayInstance
	if result := h.DB.First(&gatewayInstance, gatewayInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, gatewayInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing gateway instance.
// @Description Update a gateway instance in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating gateway instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-gatewayInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param gatewayInstance body v0.GatewayInstance true "GatewayInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-instances/{id} [patch]
func (h Handler) UpdateGatewayInstance(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayInstance
	gatewayInstanceID := c.Param("id")
	var existingGatewayInstance v0.GatewayInstance
	if result := h.DB.First(&existingGatewayInstance, gatewayInstanceID); result.Error != nil {
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
	var updatedGatewayInstance v0.GatewayInstance
	if err := c.Bind(&updatedGatewayInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// if client doesn't specify reconciled, set it to false
	if updatedGatewayInstance.Reconciled == nil {
		reconciled := false
		updatedGatewayInstance.Reconciled = &reconciled
	}

	// update object in database
	if result := h.DB.Model(&existingGatewayInstance).Updates(updatedGatewayInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controllers if reconciliation is required
	if !*existingGatewayInstance.Reconciled {
		notifPayload, err := existingGatewayInstance.NotificationPayload(
			notifications.NotificationOperationUpdated,
			false,
			0,
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.GatewayInstanceUpdateSubject, *notifPayload)
	}

	response, err := v0.CreateResponse(nil, existingGatewayInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing gateway instance by replacing the entire object.
// @Description Replace a gateway instance in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating gateway instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-gatewayInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param gatewayInstance body v0.GatewayInstance true "GatewayInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-instances/{id} [put]
func (h Handler) ReplaceGatewayInstance(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayInstance
	gatewayInstanceID := c.Param("id")
	var existingGatewayInstance v0.GatewayInstance
	if result := h.DB.First(&existingGatewayInstance, gatewayInstanceID); result.Error != nil {
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
	var updatedGatewayInstance v0.GatewayInstance
	if err := c.Bind(&updatedGatewayInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedGatewayInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedGatewayInstance.ID = existingGatewayInstance.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedGatewayInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingGatewayInstance, gatewayInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingGatewayInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a gateway instance.
// @Description Delete a gateway instance by ID from the database.
// @ID delete-gatewayInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/gateway-instances/{id} [delete]
func (h Handler) DeleteGatewayInstance(c echo.Context) error {
	objectType := v0.ObjectTypeGatewayInstance
	gatewayInstanceID := c.Param("id")
	var gatewayInstance v0.GatewayInstance
	if result := h.DB.First(&gatewayInstance, gatewayInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&gatewayInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := gatewayInstance.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.GatewayInstanceDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, gatewayInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
