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
// DomainNameDefinition
///////////////////////////////////////////////////////////////////////////////

// @Summary GetDomainNameDefinitionVersions gets the supported versions for the domain name definition API.
// @Description Get the supported API versions for domain name definitions.
// @ID domainNameDefinition-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /domain-name-definitions/versions [get]
func (h Handler) GetDomainNameDefinitionVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeDomainNameDefinition)])
}

// @Summary adds a new domain name definition.
// @Description Add a new domain name definition to the Threeport database.
// @ID add-domainNameDefinition
// @Accept json
// @Produce json
// @Param domainNameDefinition body v0.DomainNameDefinition true "DomainNameDefinition object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-definitions [post]
func (h Handler) AddDomainNameDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameDefinition
	var domainNameDefinition v0.DomainNameDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&domainNameDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, domainNameDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingDomainNameDefinition v0.DomainNameDefinition
	nameUsed := true
	result := h.DB.Where("name = ?", domainNameDefinition.Name).First(&existingDomainNameDefinition)
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
	if result := h.DB.Create(&domainNameDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := domainNameDefinition.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.DomainNameDefinitionCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, domainNameDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all domain name definitions.
// @Description Get all domain name definitions from the Threeport database.
// @ID get-domainNameDefinitions
// @Accept json
// @Produce json
// @Param name query string false "domain name definition search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-definitions [get]
func (h Handler) GetDomainNameDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameDefinition
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.DomainNameDefinition
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.DomainNameDefinition{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.DomainNameDefinition{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a domain name definition.
// @Description Get a particular domain name definition from the database.
// @ID get-domainNameDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-definitions/{id} [get]
func (h Handler) GetDomainNameDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameDefinition
	domainNameDefinitionID := c.Param("id")
	var domainNameDefinition v0.DomainNameDefinition
	if result := h.DB.First(&domainNameDefinition, domainNameDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, domainNameDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing domain name definition.
// @Description Update a domain name definition in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating domain name definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-domainNameDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param domainNameDefinition body v0.DomainNameDefinition true "DomainNameDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-definitions/{id} [patch]
func (h Handler) UpdateDomainNameDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameDefinition
	domainNameDefinitionID := c.Param("id")
	var existingDomainNameDefinition v0.DomainNameDefinition
	if result := h.DB.First(&existingDomainNameDefinition, domainNameDefinitionID); result.Error != nil {
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
	var updatedDomainNameDefinition v0.DomainNameDefinition
	if err := c.Bind(&updatedDomainNameDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	if result := h.DB.Model(&existingDomainNameDefinition).Updates(updatedDomainNameDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := updatedDomainNameDefinition.NotificationPayload(
		notifications.NotificationOperationUpdated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.DomainNameDefinitionCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, existingDomainNameDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing domain name definition by replacing the entire object.
// @Description Replace a domain name definition in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating domain name definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-domainNameDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param domainNameDefinition body v0.DomainNameDefinition true "DomainNameDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-definitions/{id} [put]
func (h Handler) ReplaceDomainNameDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameDefinition
	domainNameDefinitionID := c.Param("id")
	var existingDomainNameDefinition v0.DomainNameDefinition
	if result := h.DB.First(&existingDomainNameDefinition, domainNameDefinitionID); result.Error != nil {
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
	var updatedDomainNameDefinition v0.DomainNameDefinition
	if err := c.Bind(&updatedDomainNameDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedDomainNameDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedDomainNameDefinition.ID = existingDomainNameDefinition.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedDomainNameDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingDomainNameDefinition, domainNameDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingDomainNameDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a domain name definition.
// @Description Delete a domain name definition by ID from the database.
// @ID delete-domainNameDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-definitions/{id} [delete]
func (h Handler) DeleteDomainNameDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameDefinition
	domainNameDefinitionID := c.Param("id")
	var domainNameDefinition v0.DomainNameDefinition
	if result := h.DB.First(&domainNameDefinition, domainNameDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&domainNameDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := domainNameDefinition.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.DomainNameDefinitionDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, domainNameDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// DomainNameInstance
///////////////////////////////////////////////////////////////////////////////

// @Summary GetDomainNameInstanceVersions gets the supported versions for the domain name instance API.
// @Description Get the supported API versions for domain name instances.
// @ID domainNameInstance-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /domain-name-instances/versions [get]
func (h Handler) GetDomainNameInstanceVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeDomainNameInstance)])
}

// @Summary adds a new domain name instance.
// @Description Add a new domain name instance to the Threeport database.
// @ID add-domainNameInstance
// @Accept json
// @Produce json
// @Param domainNameInstance body v0.DomainNameInstance true "DomainNameInstance object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-instances [post]
func (h Handler) AddDomainNameInstance(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameInstance
	var domainNameInstance v0.DomainNameInstance

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&domainNameInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, domainNameInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingDomainNameInstance v0.DomainNameInstance
	nameUsed := true
	result := h.DB.Where("name = ?", domainNameInstance.Name).First(&existingDomainNameInstance)
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
	if result := h.DB.Create(&domainNameInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := domainNameInstance.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.DomainNameInstanceCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, domainNameInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all domain name instances.
// @Description Get all domain name instances from the Threeport database.
// @ID get-domainNameInstances
// @Accept json
// @Produce json
// @Param name query string false "domain name instance search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-instances [get]
func (h Handler) GetDomainNameInstances(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameInstance
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.DomainNameInstance
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.DomainNameInstance{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.DomainNameInstance{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a domain name instance.
// @Description Get a particular domain name instance from the database.
// @ID get-domainNameInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-instances/{id} [get]
func (h Handler) GetDomainNameInstance(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameInstance
	domainNameInstanceID := c.Param("id")
	var domainNameInstance v0.DomainNameInstance
	if result := h.DB.First(&domainNameInstance, domainNameInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, domainNameInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing domain name instance.
// @Description Update a domain name instance in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating domain name instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-domainNameInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param domainNameInstance body v0.DomainNameInstance true "DomainNameInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-instances/{id} [patch]
func (h Handler) UpdateDomainNameInstance(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameInstance
	domainNameInstanceID := c.Param("id")
	var existingDomainNameInstance v0.DomainNameInstance
	if result := h.DB.First(&existingDomainNameInstance, domainNameInstanceID); result.Error != nil {
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
	var updatedDomainNameInstance v0.DomainNameInstance
	if err := c.Bind(&updatedDomainNameInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	if result := h.DB.Model(&existingDomainNameInstance).Updates(updatedDomainNameInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := updatedDomainNameInstance.NotificationPayload(
		notifications.NotificationOperationUpdated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.DomainNameInstanceCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, existingDomainNameInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing domain name instance by replacing the entire object.
// @Description Replace a domain name instance in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating domain name instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-domainNameInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param domainNameInstance body v0.DomainNameInstance true "DomainNameInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-instances/{id} [put]
func (h Handler) ReplaceDomainNameInstance(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameInstance
	domainNameInstanceID := c.Param("id")
	var existingDomainNameInstance v0.DomainNameInstance
	if result := h.DB.First(&existingDomainNameInstance, domainNameInstanceID); result.Error != nil {
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
	var updatedDomainNameInstance v0.DomainNameInstance
	if err := c.Bind(&updatedDomainNameInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedDomainNameInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedDomainNameInstance.ID = existingDomainNameInstance.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedDomainNameInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingDomainNameInstance, domainNameInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingDomainNameInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a domain name instance.
// @Description Delete a domain name instance by ID from the database.
// @ID delete-domainNameInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/domain-name-instances/{id} [delete]
func (h Handler) DeleteDomainNameInstance(c echo.Context) error {
	objectType := v0.ObjectTypeDomainNameInstance
	domainNameInstanceID := c.Param("id")
	var domainNameInstance v0.DomainNameInstance
	if result := h.DB.First(&domainNameInstance, domainNameInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&domainNameInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := domainNameInstance.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.DomainNameInstanceDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, domainNameInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
