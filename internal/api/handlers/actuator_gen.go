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
// Profile
///////////////////////////////////////////////////////////////////////////////

// @Summary GetProfileVersions gets the supported versions for the profile API.
// @Description Get the supported API versions for profiles.
// @ID profile-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /profiles/versions [get]
func (h Handler) GetProfileVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeProfile)])
}

// @Summary adds a new profile.
// @Description Add a new profile to the Threeport database.
// @ID add-profile
// @Accept json
// @Produce json
// @Param profile body v0.Profile true "Profile object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/profiles [post]
func (h Handler) AddProfile(c echo.Context) error {
	objectType := v0.ObjectTypeProfile
	var profile v0.Profile

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&profile); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, profile, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingProfile v0.Profile
	nameUsed := true
	result := h.DB.Where("name = ?", profile.Name).First(&existingProfile)
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
	if result := h.DB.Create(&profile); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := profile.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ProfileCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, profile)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all profiles.
// @Description Get all profiles from the Threeport database.
// @ID get-profiles
// @Accept json
// @Produce json
// @Param name query string false "profile search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/profiles [get]
func (h Handler) GetProfiles(c echo.Context) error {
	objectType := v0.ObjectTypeProfile
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.Profile
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.Profile{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.Profile{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a profile.
// @Description Get a particular profile from the database.
// @ID get-profile
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/profiles/{id} [get]
func (h Handler) GetProfile(c echo.Context) error {
	objectType := v0.ObjectTypeProfile
	profileID := c.Param("id")
	var profile v0.Profile
	if result := h.DB.First(&profile, profileID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, profile)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing profile.
// @Description Update a profile in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating profile objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-profile
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param profile body v0.Profile true "Profile object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/profiles/{id} [patch]
func (h Handler) UpdateProfile(c echo.Context) error {
	objectType := v0.ObjectTypeProfile
	profileID := c.Param("id")
	var existingProfile v0.Profile
	if result := h.DB.First(&existingProfile, profileID); result.Error != nil {
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
	var updatedProfile v0.Profile
	if err := c.Bind(&updatedProfile); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	if result := h.DB.Model(&existingProfile).Updates(updatedProfile); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := updatedProfile.NotificationPayload(
		notifications.NotificationOperationUpdated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ProfileCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, existingProfile)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing profile by replacing the entire object.
// @Description Replace a profile in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating profile objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-profile
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param profile body v0.Profile true "Profile object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/profiles/{id} [put]
func (h Handler) ReplaceProfile(c echo.Context) error {
	objectType := v0.ObjectTypeProfile
	profileID := c.Param("id")
	var existingProfile v0.Profile
	if result := h.DB.First(&existingProfile, profileID); result.Error != nil {
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
	var updatedProfile v0.Profile
	if err := c.Bind(&updatedProfile); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedProfile, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedProfile.ID = existingProfile.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedProfile); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingProfile, profileID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingProfile)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a profile.
// @Description Delete a profile by ID from the database.
// @ID delete-profile
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/profiles/{id} [delete]
func (h Handler) DeleteProfile(c echo.Context) error {
	objectType := v0.ObjectTypeProfile
	profileID := c.Param("id")
	var profile v0.Profile
	if result := h.DB.First(&profile, profileID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&profile); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := profile.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.ProfileDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, profile)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

///////////////////////////////////////////////////////////////////////////////
// Tier
///////////////////////////////////////////////////////////////////////////////

// @Summary GetTierVersions gets the supported versions for the tier API.
// @Description Get the supported API versions for tiers.
// @ID tier-get-versions
// @Produce json
// @Success 200 {object} api.RESTAPIVersions "OK"
// @Router /tiers/versions [get]
func (h Handler) GetTierVersions(c echo.Context) error {
	return c.JSON(http.StatusOK, api.RestapiVersions[string(v0.ObjectTypeTier)])
}

// @Summary adds a new tier.
// @Description Add a new tier to the Threeport database.
// @ID add-tier
// @Accept json
// @Produce json
// @Param tier body v0.Tier true "Tier object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/tiers [post]
func (h Handler) AddTier(c echo.Context) error {
	objectType := v0.ObjectTypeTier
	var tier v0.Tier

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&tier); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, tier, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// check for duplicate names
	var existingTier v0.Tier
	nameUsed := true
	result := h.DB.Where("name = ?", tier.Name).First(&existingTier)
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
	if result := h.DB.Create(&tier); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := tier.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.TierCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, tier)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary gets all tiers.
// @Description Get all tiers from the Threeport database.
// @ID get-tiers
// @Accept json
// @Produce json
// @Param name query string false "tier search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/tiers [get]
func (h Handler) GetTiers(c echo.Context) error {
	objectType := v0.ObjectTypeTier
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.Tier
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	if result := h.DB.Model(&v0.Tier{}).Where(&filter).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	records := &[]v0.Tier{}
	if result := h.DB.Order("ID asc").Where(&filter).Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary gets a tier.
// @Description Get a particular tier from the database.
// @ID get-tier
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/tiers/{id} [get]
func (h Handler) GetTier(c echo.Context) error {
	objectType := v0.ObjectTypeTier
	tierID := c.Param("id")
	var tier v0.Tier
	if result := h.DB.First(&tier, tierID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, tier)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates specific fields for an existing tier.
// @Description Update a tier in the database.  Provide one or more fields to update.
// @Description Note: This API endpint is for updating tier objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID update-tier
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param tier body v0.Tier true "Tier object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/tiers/{id} [patch]
func (h Handler) UpdateTier(c echo.Context) error {
	objectType := v0.ObjectTypeTier
	tierID := c.Param("id")
	var existingTier v0.Tier
	if result := h.DB.First(&existingTier, tierID); result.Error != nil {
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
	var updatedTier v0.Tier
	if err := c.Bind(&updatedTier); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	if result := h.DB.Model(&existingTier).Updates(updatedTier); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := updatedTier.NotificationPayload(
		notifications.NotificationOperationUpdated,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.TierCreateSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, existingTier)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary updates an existing tier by replacing the entire object.
// @Description Replace a tier in the database.  All required fields must be provided.
// @Description If any optional fields are not provided, they will be null post-update.
// @Description Note: This API endpint is for updating tier objects only.
// @Description Request bodies that include related objects will be accepted, however
// @Description the related objects will not be changed.  Call the patch or put method for
// @Description each particular existing object to change them.
// @ID replace-tier
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Param tier body v0.Tier true "Tier object"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/tiers/{id} [put]
func (h Handler) ReplaceTier(c echo.Context) error {
	objectType := v0.ObjectTypeTier
	tierID := c.Param("id")
	var existingTier v0.Tier
	if result := h.DB.First(&existingTier, tierID); result.Error != nil {
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
	var updatedTier v0.Tier
	if err := c.Bind(&updatedTier); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, updatedTier, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// persist provided data
	updatedTier.ID = existingTier.ID
	if result := h.DB.Session(&gorm.Session{FullSaveAssociations: false}).Omit("CreatedAt", "DeletedAt").Save(&updatedTier); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// reload updated data from DB
	if result := h.DB.First(&existingTier, tierID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, existingTier)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a tier.
// @Description Delete a tier by ID from the database.
// @ID delete-tier
// @Accept json
// @Produce json
// @Param id path int true "ID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/tiers/{id} [delete]
func (h Handler) DeleteTier(c echo.Context) error {
	objectType := v0.ObjectTypeTier
	tierID := c.Param("id")
	var tier v0.Tier
	if result := h.DB.First(&tier, tierID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	if result := h.DB.Delete(&tier); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	// notify controller
	notifPayload, err := tier.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		0,
	)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}
	h.JS.Publish(v0.TierDeleteSubject, *notifPayload)

	response, err := v0.CreateResponse(nil, tier)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
