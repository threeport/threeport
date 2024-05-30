// generated by 'threeport-sdk gen' - do not edit

package handlers

import (
	"errors"
	echo "github.com/labstack/echo/v4"
	api "github.com/threeport/threeport/pkg/api"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v1 "github.com/threeport/threeport/pkg/api/v1"
	gorm "gorm.io/gorm"
	"net/http"
)

///////////////////////////////////////////////////////////////////////////////
// AttachedObjectReference
///////////////////////////////////////////////////////////////////////////////

// @Summary GetAttachedObjectReferenceVersions gets the supported versions for the attached object reference API.
// @Description Get the supported API versions for attached object references.
// @ID attachedObjectReference-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /attached-object-references/versions [GET]
func (h Handler) GetAttachedObjectReferenceVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(api_v1.ObjectTypeAttachedObjectReference)])
}

// @Summary adds a new attached object reference.
// @Description Add a new attached object reference to the Threeport database.
// @ID add-v1-attachedObjectReference
// @Accept json
// @Produce json
// @Param attachedObjectReference body v1.AttachedObjectReference true "AttachedObjectReference object"
// @Success 201 {object} v1.Response "Created"
// @Failure 400 {object} v1.Response "Bad Request"
// @Failure 500 {object} v1.Response "Internal Server Error"
// @Router /v1/attached-object-references [POST]
func (h Handler) AddAttachedObjectReference(c echo.Context) error {
	objectType := api_v1.ObjectTypeAttachedObjectReference
	var attachedObjectReference api_v1.AttachedObjectReference

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, objectType, attachedObjectReference); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&attachedObjectReference); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, attachedObjectReference, objectType); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist to DB
	if result := h.DB.Create(&attachedObjectReference); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, attachedObjectReference, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

// @Summary gets all attached object references.
// @Description Get all attached object references from the Threeport database.
// @ID get-v1-attachedObjectReferences
// @Accept json
// @Produce json
// @Param name query string false "attached object reference search by name"
// @Success 200 {object} v1.Response "OK"
// @Failure 400 {object} v1.Response "Bad Request"
// @Failure 500 {object} v1.Response "Internal Server Error"
// @Router /v1/attached-object-references [GET]
func (h Handler) GetAttachedObjectReferences(c echo.Context) error {
	objectType := api_v1.ObjectTypeAttachedObjectReference
	params, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	var filter api_v1.AttachedObjectReference
	if err := c.Bind(&filter); err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&api_v1.AttachedObjectReference{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]api_v1.AttachedObjectReference{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary gets a attached object reference.
// @Description Get a particular attached object reference from the database.
// @ID get-v1-attachedObjectReference
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v1.Response "OK"
// @Failure 404 {object} v1.Response "Not Found"
// @Failure 500 {object} v1.Response "Internal Server Error"
// @Router /v1/attached-object-references/{id} [GET]
func (h Handler) GetAttachedObjectReference(c echo.Context) error {
	objectType := api_v1.ObjectTypeAttachedObjectReference
	attachedObjectReferenceID := c.Param("id")
	var attachedObjectReference api_v1.AttachedObjectReference
	if result := h.DB.First(&attachedObjectReference, attachedObjectReferenceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, attachedObjectReference, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing attached object reference.
// @Description Update a attached object reference in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating attached object reference objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-v1-attachedObjectReference
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param attachedObjectReference body v1.AttachedObjectReference true "AttachedObjectReference object"
// @Success 200 {object} v1.Response "OK"
// @Failure 400 {object} v1.Response "Bad Request"
// @Failure 404 {object} v1.Response "Not Found"
// @Failure 500 {object} v1.Response "Internal Server Error"
// @Router /v1/attached-object-references/{id} [PATCH]
func (h Handler) UpdateAttachedObjectReference(c echo.Context) error {
	objectType := api_v1.ObjectTypeAttachedObjectReference
	attachedObjectReferenceID := c.Param("id")
	var existingAttachedObjectReference api_v1.AttachedObjectReference
	if result := h.DB.First(&existingAttachedObjectReference, attachedObjectReferenceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, true, objectType, existingAttachedObjectReference); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedAttachedObjectReference api_v1.AttachedObjectReference
	if err := c.Bind(&updatedAttachedObjectReference); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// update object in database
	if result := h.DB.Model(&existingAttachedObjectReference).Updates(updatedAttachedObjectReference); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, existingAttachedObjectReference, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary updates an existing attached object reference by replacing the entire object.
// @Description Replace a attached object reference in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating attached object reference objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-v1-attachedObjectReference
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param attachedObjectReference body v1.AttachedObjectReference true "AttachedObjectReference object"
// @Success 200 {object} v1.Response "OK"
// @Failure 400 {object} v1.Response "Bad Request"
// @Failure 404 {object} v1.Response "Not Found"
// @Failure 500 {object} v1.Response "Internal Server Error"
// @Router /v1/attached-object-references/{id} [PUT]
func (h Handler) ReplaceAttachedObjectReference(c echo.Context) error {
	objectType := api_v1.ObjectTypeAttachedObjectReference
	attachedObjectReferenceID := c.Param("id")
	var existingAttachedObjectReference api_v1.AttachedObjectReference
	if result := h.DB.First(&existingAttachedObjectReference, attachedObjectReferenceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// check for empty payload, invalid or unsupported fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, true, objectType, existingAttachedObjectReference); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// bind payload
	var updatedAttachedObjectReference api_v1.AttachedObjectReference
	if err := c.Bind(&updatedAttachedObjectReference); err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, updatedAttachedObjectReference, objectType); err != nil {
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedAttachedObjectReference.ID = existingAttachedObjectReference.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedAttachedObjectReference); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingAttachedObjectReference, attachedObjectReferenceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, existingAttachedObjectReference, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// @Summary deletes a attached object reference.
// @Description Delete a attached object reference by ID from the database.
// @ID delete-v1-attachedObjectReference
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v1.Response "OK"
// @Failure 404 {object} v1.Response "Not Found"
// @Failure 409 {object} v1.Response "Conflict"
// @Failure 500 {object} v1.Response "Internal Server Error"
// @Router /v1/attached-object-references/{id} [DELETE]
func (h Handler) DeleteAttachedObjectReference(c echo.Context) error {
	objectType := api_v1.ObjectTypeAttachedObjectReference
	attachedObjectReferenceID := c.Param("id")
	var attachedObjectReference api_v1.AttachedObjectReference
	if result := h.DB.First(&attachedObjectReference, attachedObjectReferenceID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return apiserver_lib.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// delete object
	if result := h.DB.Delete(&attachedObjectReference); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(nil, attachedObjectReference, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
