package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/internal/api/handlers"
)

func AddCustomRoutes(e *echo.Echo, h *handlers.Handler) {
	WorkloadResourceDefinitionSetRoutes(e, h)
}
