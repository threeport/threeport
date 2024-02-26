// generated by 'threeport-sdk codegen api-model' - do not edit

package handlers

import (
	"errors"
	"fmt"
	echo "github.com/labstack/echo/v4"
	api "github.com/threeport/threeport/pkg/api"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	gorm "gorm.io/gorm"
	"net/http"
	"time"
)

///////////////////////////////////////////////////////////////////////////////
// HelmWorkloadDefinition
///////////////////////////////////////////////////////////////////////////////

// @Summary GetHelmWorkloadDefinitionVersions gets the supported versions for the helm workload definition API.
// @Description Get the supported API versions for helm workload definitions.
// @ID helmWorkloadDefinition-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /helm-workload-definitions/versions [GET]
func (h Handler) GetHelmWorkloadDefinitionVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeHelmWorkloadDefinition)])
}

// @Summary adds a new helm workload definition.
// @Description Add a new helm workload definition to the Threeport database.
// @ID add-helmWorkloadDefinition
// @Accept json
// @Produce json
// @Param helmWorkloadDefinition body v0.HelmWorkloadDefinition true "HelmWorkloadDefinition object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-definitions [POST]
func (h Handler) AddHelmWorkloadDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadDefinition
	var helmWorkloadDefinition v0.HelmWorkloadDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType, helmWorkloadDefinition); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&helmWorkloadDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, helmWorkloadDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingHelmWorkloadDefinition v0.HelmWorkloadDefinition
	nameUsed := true
	result := h.DB.Where("name = ?", helmWorkloadDefinition.Name).First(&existingHelmWorkloadDefinition)
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
	if result := h.DB.Create(&helmWorkloadDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller if reconciliation is required
	if !*helmWorkloadDefinition.Reconciled {
		notifPayload, err := helmWorkloadDefinition.NotificationPayload(
			notifications.NotificationOperationCreated,
			false,
			time.Now().Unix(),
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.HelmWorkloadDefinitionCreateSubject, *notifPayload)
	}

	response, err := v0.CreateResponse(nil, helmWorkloadDefinition, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all helm workload definitions.
// @Description Get all helm workload definitions from the Threeport database.
// @ID get-helmWorkloadDefinitions
// @Accept json
// @Produce json
// @Param name query string false "helm workload definition search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-definitions [GET]
func (h Handler) GetHelmWorkloadDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadDefinition
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.HelmWorkloadDefinition
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.HelmWorkloadDefinition{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.HelmWorkloadDefinition{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a helm workload definition.
// @Description Get a particular helm workload definition from the database.
// @ID get-helmWorkloadDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-definitions/{id} [GET]
func (h Handler) GetHelmWorkloadDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadDefinition
	helmWorkloadDefinitionID := c.Param("id")
	var helmWorkloadDefinition v0.HelmWorkloadDefinition
	if result := h.DB.First(&helmWorkloadDefinition, helmWorkloadDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, helmWorkloadDefinition, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing helm workload definition.
// @Description Update a helm workload definition in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating helm workload definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-helmWorkloadDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param helmWorkloadDefinition body v0.HelmWorkloadDefinition true "HelmWorkloadDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-definitions/{id} [PATCH]
func (h Handler) UpdateHelmWorkloadDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadDefinition
	helmWorkloadDefinitionID := c.Param("id")
	var existingHelmWorkloadDefinition v0.HelmWorkloadDefinition
	if result := h.DB.First(&existingHelmWorkloadDefinition, helmWorkloadDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType, existingHelmWorkloadDefinition); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedHelmWorkloadDefinition v0.HelmWorkloadDefinition
	if err := c.Bind(&updatedHelmWorkloadDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingHelmWorkloadDefinition).Updates(updatedHelmWorkloadDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller if reconciliation is required
	if !*existingHelmWorkloadDefinition.Reconciled {
		notifPayload, err := existingHelmWorkloadDefinition.NotificationPayload(
			notifications.NotificationOperationUpdated,
			false,
			time.Now().Unix(),
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.HelmWorkloadDefinitionUpdateSubject, *notifPayload)
	}

	response, err := v0.CreateResponse(nil, existingHelmWorkloadDefinition, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing helm workload definition by replacing the entire object.
// @Description Replace a helm workload definition in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating helm workload definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-helmWorkloadDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param helmWorkloadDefinition body v0.HelmWorkloadDefinition true "HelmWorkloadDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-definitions/{id} [PUT]
func (h Handler) ReplaceHelmWorkloadDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadDefinition
	helmWorkloadDefinitionID := c.Param("id")
	var existingHelmWorkloadDefinition v0.HelmWorkloadDefinition
	if result := h.DB.First(&existingHelmWorkloadDefinition, helmWorkloadDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType, existingHelmWorkloadDefinition); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedHelmWorkloadDefinition v0.HelmWorkloadDefinition
	if err := c.Bind(&updatedHelmWorkloadDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedHelmWorkloadDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedHelmWorkloadDefinition.ID = existingHelmWorkloadDefinition.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedHelmWorkloadDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingHelmWorkloadDefinition, helmWorkloadDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingHelmWorkloadDefinition, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a helm workload definition.
// @Description Delete a helm workload definition by ID from the database.
// @ID delete-helmWorkloadDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-definitions/{id} [DELETE]
func (h Handler) DeleteHelmWorkloadDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadDefinition
	helmWorkloadDefinitionID := c.Param("id")
	var helmWorkloadDefinition v0.HelmWorkloadDefinition
	if result := h.DB.Preload("HelmWorkloadInstances").First(&helmWorkloadDefinition, helmWorkloadDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check to make sure no dependent instances exist for this definition
	if len(helmWorkloadDefinition.HelmWorkloadInstances) != 0 {
		err := errors.New("helm workload definition has related helm workload instances - cannot be deleted")
		return iapi.ResponseStatus409(c, nil, err, objectType)
	}

	// schedule for deletion if not already scheduled
	// if scheduled and reconciled, delete object from DB
	// if scheduled but not reconciled, return 409 (controller is working on it)
	if helmWorkloadDefinition.DeletionScheduled == nil {
		// schedule for deletion
		reconciled := false
		timestamp := time.Now().UTC()
		scheduledHelmWorkloadDefinition := v0.HelmWorkloadDefinition{
			Reconciliation: v0.Reconciliation{
				DeletionScheduled: &timestamp,
				Reconciled:        &reconciled,
			}}
		if result := h.DB.Model(&helmWorkloadDefinition).Updates(scheduledHelmWorkloadDefinition); result.Error != nil {
			return iapi.ResponseStatus500(c, nil, result.Error, objectType)
		}
		// notify controller
		notifPayload, err := helmWorkloadDefinition.NotificationPayload(
			notifications.NotificationOperationDeleted,
			false,
			time.Now().Unix(),
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.HelmWorkloadDefinitionDeleteSubject, *notifPayload)
	} else {
		if helmWorkloadDefinition.DeletionConfirmed == nil {
			// if deletion scheduled but not reconciled, return 409 - deletion
			// already underway
			return iapi.ResponseStatus409(c, nil, errors.New(fmt.Sprintf(
				"object with ID %d already being deleted",
				*helmWorkloadDefinition.ID,
			)), objectType)
		} else {
			// object scheduled for deletion and confirmed - it can be deleted
			// from DB
			if result := h.DB.Delete(&helmWorkloadDefinition); result.Error != nil {
				return iapi.ResponseStatus500(c, nil, result.Error, objectType)
			}
		}
	}

	response, err := v0.CreateResponse(nil, helmWorkloadDefinition, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// HelmWorkloadInstance
///////////////////////////////////////////////////////////////////////////////

// @Summary GetHelmWorkloadInstanceVersions gets the supported versions for the helm workload instance API.
// @Description Get the supported API versions for helm workload instances.
// @ID helmWorkloadInstance-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /helm-workload-instances/versions [GET]
func (h Handler) GetHelmWorkloadInstanceVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeHelmWorkloadInstance)])
}

// @Summary adds a new helm workload instance.
// @Description Add a new helm workload instance to the Threeport database.
// @ID add-helmWorkloadInstance
// @Accept json
// @Produce json
// @Param helmWorkloadInstance body v0.HelmWorkloadInstance true "HelmWorkloadInstance object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-instances [POST]
func (h Handler) AddHelmWorkloadInstance(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadInstance
	var helmWorkloadInstance v0.HelmWorkloadInstance

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType, helmWorkloadInstance); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&helmWorkloadInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, helmWorkloadInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingHelmWorkloadInstance v0.HelmWorkloadInstance
	nameUsed := true
	result := h.DB.Where("name = ?", helmWorkloadInstance.Name).First(&existingHelmWorkloadInstance)
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
	if result := h.DB.Create(&helmWorkloadInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller if reconciliation is required
	if !*helmWorkloadInstance.Reconciled {
		notifPayload, err := helmWorkloadInstance.NotificationPayload(
			notifications.NotificationOperationCreated,
			false,
			time.Now().Unix(),
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.HelmWorkloadInstanceCreateSubject, *notifPayload)
	}

	response, err := v0.CreateResponse(nil, helmWorkloadInstance, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all helm workload instances.
// @Description Get all helm workload instances from the Threeport database.
// @ID get-helmWorkloadInstances
// @Accept json
// @Produce json
// @Param name query string false "helm workload instance search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-instances [GET]
func (h Handler) GetHelmWorkloadInstances(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadInstance
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.HelmWorkloadInstance
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.HelmWorkloadInstance{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.HelmWorkloadInstance{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a helm workload instance.
// @Description Get a particular helm workload instance from the database.
// @ID get-helmWorkloadInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-instances/{id} [GET]
func (h Handler) GetHelmWorkloadInstance(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadInstance
	helmWorkloadInstanceID := c.Param("id")
	var helmWorkloadInstance v0.HelmWorkloadInstance
	if result := h.DB.First(&helmWorkloadInstance, helmWorkloadInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, helmWorkloadInstance, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing helm workload instance.
// @Description Update a helm workload instance in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating helm workload instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-helmWorkloadInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param helmWorkloadInstance body v0.HelmWorkloadInstance true "HelmWorkloadInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-instances/{id} [PATCH]
func (h Handler) UpdateHelmWorkloadInstance(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadInstance
	helmWorkloadInstanceID := c.Param("id")
	var existingHelmWorkloadInstance v0.HelmWorkloadInstance
	if result := h.DB.First(&existingHelmWorkloadInstance, helmWorkloadInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType, existingHelmWorkloadInstance); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedHelmWorkloadInstance v0.HelmWorkloadInstance
	if err := c.Bind(&updatedHelmWorkloadInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingHelmWorkloadInstance).Updates(updatedHelmWorkloadInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller if reconciliation is required
	if !*existingHelmWorkloadInstance.Reconciled {
		notifPayload, err := existingHelmWorkloadInstance.NotificationPayload(
			notifications.NotificationOperationUpdated,
			false,
			time.Now().Unix(),
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.HelmWorkloadInstanceUpdateSubject, *notifPayload)
	}

	response, err := v0.CreateResponse(nil, existingHelmWorkloadInstance, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing helm workload instance by replacing the entire object.
// @Description Replace a helm workload instance in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating helm workload instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-helmWorkloadInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param helmWorkloadInstance body v0.HelmWorkloadInstance true "HelmWorkloadInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-instances/{id} [PUT]
func (h Handler) ReplaceHelmWorkloadInstance(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadInstance
	helmWorkloadInstanceID := c.Param("id")
	var existingHelmWorkloadInstance v0.HelmWorkloadInstance
	if result := h.DB.First(&existingHelmWorkloadInstance, helmWorkloadInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, true, objectType, existingHelmWorkloadInstance); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedHelmWorkloadInstance v0.HelmWorkloadInstance
	if err := c.Bind(&updatedHelmWorkloadInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedHelmWorkloadInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedHelmWorkloadInstance.ID = existingHelmWorkloadInstance.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedHelmWorkloadInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingHelmWorkloadInstance, helmWorkloadInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingHelmWorkloadInstance, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a helm workload instance.
// @Description Delete a helm workload instance by ID from the database.
// @ID delete-helmWorkloadInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/helm-workload-instances/{id} [DELETE]
func (h Handler) DeleteHelmWorkloadInstance(c echo.Context) error {
	objectType := v0.ObjectTypeHelmWorkloadInstance
	helmWorkloadInstanceID := c.Param("id")
	var helmWorkloadInstance v0.HelmWorkloadInstance
	if result := h.DB.First(&helmWorkloadInstance, helmWorkloadInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// schedule for deletion if not already scheduled
	// if scheduled and reconciled, delete object from DB
	// if scheduled but not reconciled, return 409 (controller is working on it)
	if helmWorkloadInstance.DeletionScheduled == nil {
		// schedule for deletion
		reconciled := false
		timestamp := time.Now().UTC()
		scheduledHelmWorkloadInstance := v0.HelmWorkloadInstance{
			Reconciliation: v0.Reconciliation{
				DeletionScheduled: &timestamp,
				Reconciled:        &reconciled,
			}}
		if result := h.DB.Model(&helmWorkloadInstance).Updates(scheduledHelmWorkloadInstance); result.Error != nil {
			return iapi.ResponseStatus500(c, nil, result.Error, objectType)
		}
		// notify controller
		notifPayload, err := helmWorkloadInstance.NotificationPayload(
			notifications.NotificationOperationDeleted,
			false,
			time.Now().Unix(),
		)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}
		h.JS.Publish(v0.HelmWorkloadInstanceDeleteSubject, *notifPayload)
	} else {
		if helmWorkloadInstance.DeletionConfirmed == nil {
			// if deletion scheduled but not reconciled, return 409 - deletion
			// already underway
			return iapi.ResponseStatus409(c, nil, errors.New(fmt.Sprintf(
				"object with ID %d already being deleted",
				*helmWorkloadInstance.ID,
			)), objectType)
		} else {
			// object scheduled for deletion and confirmed - it can be deleted
			// from DB
			if result := h.DB.Delete(&helmWorkloadInstance); result.Error != nil {
				return iapi.ResponseStatus500(c, nil, result.Error, objectType)
			}
		}
	}

	response, err := v0.CreateResponse(nil, helmWorkloadInstance, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
