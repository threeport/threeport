package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v1/handlers"
)

func EventsCustomRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/v1/events-join-attached-object-references", h.GetEventsJoinAttachedObjectReferences)
}
