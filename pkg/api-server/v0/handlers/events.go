package handlers

import (
	"errors"

	echo "github.com/labstack/echo/v4"
	zap "go.uber.org/zap"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// @Summary gets all events joined with attached object references.
// @Description Get all events joined with attached object references
// from the Threeport database for a given objectId.
// @ID get-v0-events-join-attached-object-references
// @Accept json
// @Produce json
// @Param name query string false "events joined with attached object references search by objectId"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/events-join-attached-object-references [GET]
func (h Handler) GetEventsJoinAttachedObjectReferences(c echo.Context) error {
	objectType := v0.ObjectTypeEvent
	params, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, &params, err, objectType)
	}

	var filter v0.Event
	if err := c.Bind(&filter); err != nil {
		h.Logger.Error("handler error: error binding filter", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	records := &[]v0.Event{}
	objectId := c.QueryParam("objectid")
	if objectId == "" {
		return apiserver_lib.ResponseStatus400(c, &params, errors.New("must provide object ID"), objectType)
	}

	if result := h.DB.Joins(
		"INNER JOIN v0_attached_object_references ON v0_events.attached_object_reference_id = v0_attached_object_references.id",
	).Where(
		"v0_attached_object_references.object_id = ?", objectId,
	).Where(&filter).Find(records); result.Error != nil {
		h.Logger.Error("handler error: error getting events with attached object references", zap.Error(result.Error))
		return apiserver_lib.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.CreateMeta(params, totalCount), *records, objectType)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, &params, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}
