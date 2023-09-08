package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
)

// AddCustomRoutes adds non-code-generated routes for special use cases.
func AddCustomRoutes(e *echo.Echo, h *handlers.Handler) {
	WorkloadResourceDefinitionSetRoutes(e, h)
	WorkloadEventSetRoutes(e, h)
}
