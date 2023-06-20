package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/internal/api/handlers"
)

// AddCustomRoutes adds non-code-generated routes for special use cases.
func AddCustomRoutes(e *echo.Echo, h *handlers.Handler) {
	WorkloadResourceDefinitionSetRoutes(e, h)
	WorkloadEventSetRoutes(e, h)
}
