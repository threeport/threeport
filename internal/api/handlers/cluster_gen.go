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
// ClusterDefinition
///////////////////////////////////////////////////////////////////////////////

// @Summary GetClusterDefinitionVersions gets the supported versions for the cluster definition API.
// @Description Get the supported API versions for cluster definitions.
// @ID clusterDefinition-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /cluster-definitions/versions [get]
func (h Handler) GetClusterDefinitionVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeClusterDefinition)])
}

// @Summary adds a new cluster definition.
// @Description Add a new cluster definition to the Threeport database.
// @ID add-clusterDefinition
// @Accept json
// @Produce json
// @Param clusterDefinition body v0.ClusterDefinition true "ClusterDefinition object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-definitions [post]
func (h Handler) AddClusterDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeClusterDefinition
	var clusterDefinition v0.ClusterDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&clusterDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, clusterDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingClusterDefinition v0.ClusterDefinition
	nameUsed := true
	result := h.DB.Where("name = ?", clusterDefinition.Name).First(&existingClusterDefinition)
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
	if result := h.DB.Create(&clusterDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := clusterDefinition.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ClusterDefinitionCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, clusterDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all cluster definitions.
// @Description Get all cluster definitions from the Threeport database.
// @ID get-clusterDefinitions
// @Accept json
// @Produce json
// @Param name query string false "cluster definition search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-definitions [get]
func (h Handler) GetClusterDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeClusterDefinition
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.ClusterDefinition
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.ClusterDefinition{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.ClusterDefinition{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a cluster definition.
// @Description Get a particular cluster definition from the database.
// @ID get-clusterDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-definitions/{id} [get]
func (h Handler) GetClusterDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeClusterDefinition
	clusterDefinitionID := c.Param("id")
	var clusterDefinition v0.ClusterDefinition
	if result := h.DB.First(&clusterDefinition, clusterDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, clusterDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing cluster definition.
// @Description Update a cluster definition in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating cluster definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-clusterDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param clusterDefinition body v0.ClusterDefinition true "ClusterDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-definitions/{id} [patch]
func (h Handler) UpdateClusterDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeClusterDefinition
	clusterDefinitionID := c.Param("id")
	var existingClusterDefinition v0.ClusterDefinition
	if result := h.DB.First(&existingClusterDefinition, clusterDefinitionID); result.Error != nil {
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
	var updatedClusterDefinition v0.ClusterDefinition
	if err := c.Bind(&updatedClusterDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingClusterDefinition).Updates(updatedClusterDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingClusterDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing cluster definition by replacing the entire object.
// @Description Replace a cluster definition in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating cluster definition objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-clusterDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param clusterDefinition body v0.ClusterDefinition true "ClusterDefinition object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-definitions/{id} [put]
func (h Handler) ReplaceClusterDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeClusterDefinition
	clusterDefinitionID := c.Param("id")
	var existingClusterDefinition v0.ClusterDefinition
	if result := h.DB.First(&existingClusterDefinition, clusterDefinitionID); result.Error != nil {
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
	var updatedClusterDefinition v0.ClusterDefinition
	if err := c.Bind(&updatedClusterDefinition); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedClusterDefinition, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedClusterDefinition.ID = existingClusterDefinition.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedClusterDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingClusterDefinition, clusterDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingClusterDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a cluster definition.
// @Description Delete a cluster definition by ID from the database.
// @ID delete-clusterDefinition
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-definitions/{id} [delete]
func (h Handler) DeleteClusterDefinition(c echo.Context) error {
	objectType := v0.ObjectTypeClusterDefinition
	clusterDefinitionID := c.Param("id")
	var clusterDefinition v0.ClusterDefinition
	if result := h.DB.First(&clusterDefinition, clusterDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&clusterDefinition); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := clusterDefinition.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ClusterDefinitionDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, clusterDefinition)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// ClusterInstance
///////////////////////////////////////////////////////////////////////////////

// @Summary GetClusterInstanceVersions gets the supported versions for the cluster instance API.
// @Description Get the supported API versions for cluster instances.
// @ID clusterInstance-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /cluster-instances/versions [get]
func (h Handler) GetClusterInstanceVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeClusterInstance)])
}

// @Summary adds a new cluster instance.
// @Description Add a new cluster instance to the Threeport database.
// @ID add-clusterInstance
// @Accept json
// @Produce json
// @Param clusterInstance body v0.ClusterInstance true "ClusterInstance object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-instances [post]
func (h Handler) AddClusterInstance(c echo.Context) error {
	objectType := v0.ObjectTypeClusterInstance
	var clusterInstance v0.ClusterInstance

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&clusterInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, clusterInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingClusterInstance v0.ClusterInstance
	nameUsed := true
	result := h.DB.Where("name = ?", clusterInstance.Name).First(&existingClusterInstance)
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
	if result := h.DB.Create(&clusterInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := clusterInstance.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ClusterInstanceCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, clusterInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all cluster instances.
// @Description Get all cluster instances from the Threeport database.
// @ID get-clusterInstances
// @Accept json
// @Produce json
// @Param name query string false "cluster instance search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-instances [get]
func (h Handler) GetClusterInstances(c echo.Context) error {
	objectType := v0.ObjectTypeClusterInstance
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.ClusterInstance
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.ClusterInstance{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.ClusterInstance{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a cluster instance.
// @Description Get a particular cluster instance from the database.
// @ID get-clusterInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-instances/{id} [get]
func (h Handler) GetClusterInstance(c echo.Context) error {
	objectType := v0.ObjectTypeClusterInstance
	clusterInstanceID := c.Param("id")
	var clusterInstance v0.ClusterInstance
	if result := h.DB.First(&clusterInstance, clusterInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, clusterInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing cluster instance.
// @Description Update a cluster instance in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating cluster instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-clusterInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param clusterInstance body v0.ClusterInstance true "ClusterInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-instances/{id} [patch]
func (h Handler) UpdateClusterInstance(c echo.Context) error {
	objectType := v0.ObjectTypeClusterInstance
	clusterInstanceID := c.Param("id")
	var existingClusterInstance v0.ClusterInstance
	if result := h.DB.First(&existingClusterInstance, clusterInstanceID); result.Error != nil {
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
	var updatedClusterInstance v0.ClusterInstance
	if err := c.Bind(&updatedClusterInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingClusterInstance).Updates(updatedClusterInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingClusterInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing cluster instance by replacing the entire object.
// @Description Replace a cluster instance in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating cluster instance objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-clusterInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param clusterInstance body v0.ClusterInstance true "ClusterInstance object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-instances/{id} [put]
func (h Handler) ReplaceClusterInstance(c echo.Context) error {
	objectType := v0.ObjectTypeClusterInstance
	clusterInstanceID := c.Param("id")
	var existingClusterInstance v0.ClusterInstance
	if result := h.DB.First(&existingClusterInstance, clusterInstanceID); result.Error != nil {
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
	var updatedClusterInstance v0.ClusterInstance
	if err := c.Bind(&updatedClusterInstance); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedClusterInstance, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedClusterInstance.ID = existingClusterInstance.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedClusterInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingClusterInstance, clusterInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingClusterInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a cluster instance.
// @Description Delete a cluster instance by ID from the database.
// @ID delete-clusterInstance
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/cluster-instances/{id} [delete]
func (h Handler) DeleteClusterInstance(c echo.Context) error {
	objectType := v0.ObjectTypeClusterInstance
	clusterInstanceID := c.Param("id")
	var clusterInstance v0.ClusterInstance
	if result := h.DB.First(&clusterInstance, clusterInstanceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&clusterInstance); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := clusterInstance.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ClusterInstanceDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, clusterInstance)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
