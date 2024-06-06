package handlers

import (
	echo "github.com/labstack/echo/v4"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
)

// @Summary gets all events joined with attached object references.
// @Description Get all events joined with attached object references
// from the Threeport database for a given objectId.
// @ID get-v1-events-join-attached-object-references
// @Accept json
// @Produce json
// @Param name query string false "events joined with attached object references search by objectId"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v1/events-join-attached-object-references [GET]
func (h Handler) GetEventsJoinAttachedObjectReferences(c echo.Context) error {
	objectType := v1.ObjectTypeEvent
	params, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v1.Event
	if err := c.Bind(&filter); err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	records := &[]v1.Event{}
	objectId := c.QueryParam("objectid")
	if objectId == "" {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	if result := h.DB.Joins(
		"INNER JOIN attached_object_references ON events.attached_object_reference_id = attached_object_references.id",
	).Where(
		"attached_object_references.object_id = ?", objectId,
	).Where(&filter).Find(records); result.Error != nil {
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
