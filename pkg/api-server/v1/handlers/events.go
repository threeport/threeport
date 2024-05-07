package handlers

import (
	echo "github.com/labstack/echo/v4"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
)

// @Summary gets all events.
// @Description Get all events from the Threeport database.
// @ID get-v1-events
// @Accept json
// @Produce json
// @Param name query string false "event search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v1/events [GET]
func (h Handler) GetEventsJoinAttachedObjectReferences(c echo.Context) error {
	objectType := v1.ObjectTypeEvent
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v1.Event
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	records := &[]v1.Event{}
	objectID := c.QueryParam("objectid")
	if result := h.DB.Joins(
		"INNER JOIN attached_object_references ON events.attached_object_reference_id = attached_object_references.id",
	).Where(
		"attached_object_references.object_id = ?", objectID,
	).Where(&filter).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
