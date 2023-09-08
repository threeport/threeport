package routes

import (
	"github.com/labstack/echo/v4"

	"github.com/threeport/threeport/pkg/api-server/v0/handlers"
)

// VersionRoutes sets up all routes for the version.
func VersionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/version", h.GetApiVersion)
}
