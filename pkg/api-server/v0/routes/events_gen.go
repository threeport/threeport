// generated by 'threeport-sdk gen' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// EventRoutes sets up all routes for the Event handlers.
func EventRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/events/versions", h.GetEventVersions)

	e.POST(v0.PathEvents, h.AddEvent)
	e.GET(v0.PathEvents, h.GetEvents)
	e.GET(v0.PathEvents+"/:id", h.GetEvent)
	e.PATCH(v0.PathEvents+"/:id", h.UpdateEvent)
	e.PUT(v0.PathEvents+"/:id", h.ReplaceEvent)
	e.DELETE(v0.PathEvents+"/:id", h.DeleteEvent)
}
